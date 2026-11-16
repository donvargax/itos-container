//go:build unix

package disk

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/sys/unix"
)

func TestStagedLogPublication(t *testing.T) {
	over := append(bytes.Repeat([]byte("0123456789abcde\n"), 65536), "tail\n"...)
	if len(over) <= 1024*1024 {
		t.Fatal("the large body must exceed what Publish accepts for a record", len(over))
	}
	for _, tc := range []struct {
		name    string
		umask   int
		parts   [][]byte
		publish bool
	}{
		{"over 1 MiB, umask 0", 0, [][]byte{over[:300000], over[300000:1048576], over[1048576:]}, true},
		{"over 1 MiB, umask 0777", 0777, [][]byte{over[:300000], over[300000:1048576], over[1048576:]}, true},
		{"empty, umask 0", 0, nil, true},
		{"empty, umask 0777", 0777, nil, true},
		{"closed unpublished, umask 0777", 0777, [][]byte{[]byte("discarded\n")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, _ := diskFixture(t)
			id, dir := stagedSession(t, Registry{Store: s}, c)
			// The pinned chain is the filesystem root, then each directory down to the session's.
			chain := []string{string(filepath.Separator)}
			for at := dir; at != string(filepath.Separator); at = filepath.Dir(at) {
				chain = slices.Insert(chain, 1, at)
			}
			same := func(file *os.File, path string) bool {
				a, errA := file.Stat()
				b, errB := os.Stat(path)
				return errA == nil && errB == nil && os.SameFile(a, b)
			}
			inSession := func(root *os.Root) bool {
				file, err := root.Open(".")
				if err != nil {
					return false
				}
				defer func() { _ = file.Close() }()
				return same(file, dir)
			}
			mode := func(file *os.File) os.FileMode {
				info, err := file.Stat()
				if err != nil {
					t.Fatal(err)
				}
				return info.Mode()
			}
			var journal []string
			var temporary string
			ops := s.operations()
			real := ops
			ops.syncDir = func(file *os.File) error {
				synced := len(journal)
				if synced >= len(chain) || !same(file, chain[synced]) {
					t.Errorf("directory sync %d is not of %v", synced, chain)
				}
				journal = append(journal, "directory sync")
				return real.syncDir(file)
			}
			ops.create = func(root *os.Root, name string) (*os.File, error) {
				journal, temporary = append(journal, "create"), name
				file, err := real.create(root, name)
				if err == nil && mode(file) != os.FileMode(0600&^tc.umask) {
					t.Errorf("created with mode %v under umask %o", mode(file), tc.umask)
				}
				return file, err
			}
			ops.chmod = func(file *os.File, perm os.FileMode) error {
				journal = append(journal, fmt.Sprintf("chmod %o", perm))
				err := real.chmod(file, perm)
				if mode(file) != 0600 {
					t.Errorf("mode after chmod is %v", mode(file))
				}
				return err
			}
			ops.write = func(file *os.File, data []byte) (int, error) {
				journal = append(journal, fmt.Sprintf("write %d", len(data)))
				return real.write(file, data)
			}
			ops.syncFile = func(file *os.File) error { journal = append(journal, "file fsync"); return real.syncFile(file) }
			ops.closeFile = func(file *os.File) error { journal = append(journal, "close"); return real.closeFile(file) }
			ops.rename = func(root *os.Root, from, to string) error {
				journal = append(journal, "rename "+to)
				if from != temporary || !inSession(root) {
					t.Errorf("rename of %s, staged as %s, in %s", from, temporary, root.Name())
				}
				return real.rename(root, from, to)
			}
			ops.syncParent = func(file *os.File) error {
				journal = append(journal, "parent fsync")
				if !same(file, dir) {
					t.Errorf("parent fsync of %s", file.Name())
				}
				return real.syncParent(file)
			}
			s.ops = &ops
			previous := unix.Umask(tc.umask)
			defer unix.Umask(previous)
			staged, failure := (Registry{Store: s}).Stage(context.Background(), c, id)
			if failure != "" || staged == nil {
				t.Fatalf("stage = %v, %s", staged, failure)
			}
			want := slices.Repeat([]string{"directory sync"}, len(chain))
			want = append(want, "create", "chmod 600")
			if !slices.Equal(journal, want) || !stagedTemporary(temporary) || !slices.Equal(stagedTemporaries(t, dir), []string{temporary}) {
				t.Fatalf("staging journal %v with temporary %q, want %v", journal, temporary, want)
			}
			var written []byte
			for _, part := range tc.parts {
				if failure = staged.Write(part); failure != "" {
					t.Fatal(failure)
				}
				written = append(written, part...)
				want = append(want, fmt.Sprintf("write %d", len(part)))
				// The bytes are the file's before Write returns, not a buffer's.
				if data, err := os.ReadFile(filepath.Join(dir, temporary)); err != nil || !bytes.Equal(data, written) {
					t.Fatalf("temporary holds %d bytes after writing %d: %v", len(data), len(written), err)
				}
			}
			if storedLog(t, dir) != nil {
				t.Fatal("a log is visible before Publish")
			}
			names := []string{"controller.lock", "operation.lock", "record.json"}
			if tc.publish {
				if failure = staged.Publish(); failure != "" {
					t.Fatal(failure)
				}
				want = append(want, "file fsync", "close", "rename "+stagedLogName, "parent fsync")
				names = slices.Insert(names, 1, stagedLogName)
				info, err := os.Lstat(filepath.Join(dir, stagedLogName))
				if err != nil {
					t.Fatal(err)
				}
				owner, links, ok := ownership(info)
				if !ok || !info.Mode().IsRegular() || info.Mode() != 0600 || links != 1 || owner != uint64(os.Geteuid()) {
					t.Fatalf("published log has mode %v, %d links and owner %d", info.Mode(), links, owner)
				}
				if data := storedLog(t, dir); data == nil || !bytes.Equal(data, written) {
					t.Fatalf("published log holds %d bytes, want %d", len(data), len(written))
				}
			}
			staged.Close()
			if !slices.Equal(journal, want) {
				t.Fatalf("journal %v, want %v", journal, want)
			}
			if got := sessionNames(t, dir); !slices.Equal(got, names) {
				t.Fatalf("session directory holds %v, want %v", got, names)
			}
		})
	}
}
