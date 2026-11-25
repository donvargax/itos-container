//go:build unix

package disk

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// A directory is swept only while no live publisher can own a temporary in it.
func TestSweepKeepsADirectoryWhoseLockIsNotOurs(t *testing.T) {
	hold := func(t *testing.T, path string) {
		t.Helper()
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = unlock(file); _ = file.Close() })
		if err := tryLock(file); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name string
		kind sweepFixtureKind
		set  func(t *testing.T, s *swept)
	}{
		{"admission collection held", fixtureAdmission, func(t *testing.T, s *swept) { hold(t, s.dirs[fixtureAdmission]) }},
		{"intent directory held", fixtureIntents, func(t *testing.T, s *swept) { hold(t, s.dirs[fixtureIntents]) }},
		{"operation lock held", fixtureSession, func(t *testing.T, s *swept) { hold(t, filepath.Join(s.dirs[fixtureSession], "operation.lock")) }},
		{"operation lock missing", fixtureSession, func(t *testing.T, s *swept) {
			if err := os.Remove(filepath.Join(s.dirs[fixtureSession], "operation.lock")); err != nil {
				t.Fatal(err)
			}
		}},
		{"operation lock is a symlink", fixtureSession, func(t *testing.T, s *swept) {
			path := filepath.Join(s.dirs[fixtureSession], "operation.lock")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(s.dirs[fixtureSession], "record.json"), path); err != nil {
				t.Fatal(err)
			}
		}},
		{"operation lock is not private", fixtureSession, func(t *testing.T, s *swept) {
			if err := os.Chmod(filepath.Join(s.dirs[fixtureSession], "operation.lock"), 0644); err != nil {
				t.Fatal(err)
			}
		}},
		{"operation lock has a second link", fixtureSession, func(t *testing.T, s *swept) {
			mustLink(t, filepath.Join(s.dirs[fixtureSession], "operation.lock"), filepath.Join(s.dirs[fixtureSession], "again"))
		}},
		{"operation lock is not empty", fixtureSession, func(t *testing.T, s *swept) {
			if err := os.WriteFile(filepath.Join(s.dirs[fixtureSession], "operation.lock"), []byte("x"), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"operation lock is a directory", fixtureSession, func(t *testing.T, s *swept) {
			path := filepath.Join(s.dirs[fixtureSession], "operation.lock")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := sweptTree(t)
			kept := s.lone(t, tc.kind, "1")
			linked := s.link(t, tc.kind, "2")
			// The other directories are not held back.
			var others []string
			for kind := range s.dirs {
				if kind != tc.kind {
					others = append(others, s.lone(t, kind, "3"))
				}
			}
			tc.set(t, s)
			s.begin(t, true)
			present(t, kept)
			present(t, linked)
			for _, path := range others {
				gone(t, path)
			}
		})
	}
}

func TestSweepSkipsFIFOOperationLockWithoutOpeningIt(t *testing.T) {
	s := sweptTree(t)
	kept := s.lone(t, fixtureSession, "1")
	orphan := s.lone(t, fixtureAdmission, "2")
	dir := s.dirs[fixtureSession]
	lock := filepath.Join(dir, "operation.lock")
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(lock, 0600); err != nil {
		t.Fatal(err)
	}
	ops := s.control.Store.operations()
	openRecord := ops.openRecord
	ops.openRecord = func(root *os.Root, name string, flags int, mode os.FileMode) (*os.File, error) {
		if root.Name() == dir && name == "operation.lock" {
			t.Error("sweep tried to open a FIFO operation lock")
			return nil, os.ErrInvalid
		}
		return openRecord(root, name, flags, mode)
	}
	s.control.Store.ops = &ops
	s.begin(t, true)
	gone(t, orphan)
	present(t, kept)
	present(t, lock)
}

func TestSweepRejectsFIFOSwappedInBeforeOperationLockOpen(t *testing.T) {
	s := sweptTree(t)
	kept := s.lone(t, fixtureSession, "1")
	orphan := s.lone(t, fixtureAdmission, "2")
	dir := s.dirs[fixtureSession]
	lock := filepath.Join(dir, "operation.lock")
	fifo := filepath.Join(dir, "replacement-fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}

	ops := s.control.Store.operations()
	openRecord := ops.openRecord
	opened := false
	ops.openRecord = func(root *os.Root, name string, flags int, mode os.FileMode) (*os.File, error) {
		if root.Name() == dir && name == "operation.lock" {
			opened = true
			// Check the flags before the real open so a regression cannot hang the test.
			if flags&syscall.O_NONBLOCK == 0 {
				t.Error("operation lock open is blocking")
				return nil, os.ErrInvalid
			}
			if err := os.Rename(fifo, lock); err != nil {
				t.Fatal(err)
			}
		}
		return openRecord(root, name, flags, mode)
	}
	s.control.Store.ops = &ops
	s.begin(t, true)
	if !opened {
		t.Fatal("sweep did not reach the operation lock open")
	}
	gone(t, orphan)
	present(t, kept)
}

func TestSweepKeepsTemporaryWhenOperationLockDescriptorBecomesUnsafe(t *testing.T) {
	s := sweptTree(t)
	kept := s.lone(t, fixtureSession, "1")
	lock := filepath.Join(s.dirs[fixtureSession], "operation.lock")
	ops := s.control.Store.operations()
	statFile := ops.statFile
	checks := 0
	ops.statFile = func(file *os.File) (os.FileInfo, error) {
		info, err := statFile(file)
		if err == nil && file.Name() == lock {
			checks++
			if checks == 2 {
				// The descriptor recheck sees unsafe ownership while the
				// subsequent pathname lookup would still accept the inode.
				info.Sys().(*syscall.Stat_t).Uid = uint32(os.Geteuid()) + 1
			}
		}
		return info, err
	}
	s.control.Store.ops = &ops
	s.begin(t, true)
	if checks != 2 {
		t.Fatalf("operation lock descriptor checks = %d, want 2", checks)
	}
	present(t, kept)
}

func TestSweepKeepsDirectoryWhenOperationLockIsReplaced(t *testing.T) {
	s := sweptTree(t)
	kept := s.lone(t, fixtureSession, "1")
	orphan := s.lone(t, fixtureAdmission, "2")
	dir := s.dirs[fixtureSession]
	lock := filepath.Join(dir, "operation.lock")
	newLock := filepath.Join(dir, "replacement")
	if err := os.WriteFile(newLock, nil, 0600); err != nil {
		t.Fatal(err)
	}
	ops := s.control.Store.operations()
	lstat := ops.lstat
	checks := 0
	ops.lstat = func(root *os.Root, name string) (os.FileInfo, error) {
		// Swap the lock after the sweep acquired it, during the temporary's recheck.
		if root.Name() == dir && name == filepath.Base(kept) {
			checks++
			if checks == 2 {
				if err := os.Rename(newLock, lock); err != nil {
					t.Fatal(err)
				}
			}
		}
		return lstat(root, name)
	}
	s.control.Store.ops = &ops
	s.begin(t, true)
	gone(t, orphan)
	present(t, kept)
	if checks != 2 {
		t.Fatalf("temporary checks = %d, want 2", checks)
	}
}
