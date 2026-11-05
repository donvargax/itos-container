package disk

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateChmodKeepsPinnedInode(t *testing.T) {
	s, _, _ := diskFixture(t)
	path := filepath.Join(s.StateHome, "new")
	moved := filepath.Join(s.StateHome, "moved")
	target := filepath.Join(s.StateHome, "target")
	if err := os.Mkdir(path, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(moved, 0700) })
	if err := os.Mkdir(target, 0500); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(s.StateHome)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = parent.Close() }()
	fd, err := openPrivate(parent.Fd(), "new")
	if err != nil {
		t.Fatal(err)
	}
	pinned := os.NewFile(uintptr(fd), path)
	defer func() { _ = pinned.Close() }()
	if err := os.Rename(path, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := chmodPrivate(pinned.Fd()); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{moved: 0700, target: 0500} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != want {
			t.Fatalf("%s mode = %o, want %o", path, info.Mode().Perm(), want)
		}
	}
}

func TestPrivateDirectoryRejectsReplacement(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		t.Run(map[bool]string{false: "directory", true: "symlink"}[symlink], func(t *testing.T) {
			s, _, _ := diskFixture(t)
			root, err := os.OpenRoot(s.StateHome)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = root.Close() }()
			if err := root.Mkdir("new", 0700); err != nil {
				t.Fatal(err)
			}
			expected, err := root.Lstat("new")
			if err != nil {
				t.Fatal(err)
			}
			if err := root.Rename("new", "old"); err != nil {
				t.Fatal(err)
			}
			if symlink {
				if err := root.Mkdir("target", 0500); err != nil {
					t.Fatal(err)
				}
				err = root.Symlink("target", "new")
			} else {
				err = root.Mkdir("new", 0500)
			}
			if err != nil {
				t.Fatal(err)
			}
			if info, failure := s.operations().privateDirectory(root, "new", expected); failure != "state.path" || info != nil {
				t.Fatalf("replacement accepted: %v, %s", info, failure)
			}
			actual, err := root.Stat("new")
			if err != nil || actual.Mode().Perm() != 0500 {
				t.Fatalf("replacement changed: %v, %v", actual, err)
			}
		})
	}
}

func TestPrivateDirectoryRejectsNonDirectory(t *testing.T) {
	s, _, _ := diskFixture(t)
	path := filepath.Join(s.StateHome, "file")
	if err := os.WriteFile(path, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(s.StateHome)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	expected, err := root.Lstat("file")
	if err != nil {
		t.Fatal(err)
	}
	if info, failure := s.operations().privateDirectory(root, "file", expected); failure != "state.path" || info != nil {
		t.Fatalf("non-directory accepted: %v, %s", info, failure)
	}
	actual, err := root.Lstat("file")
	if err != nil || !os.SameFile(expected, actual) || actual.Mode() != expected.Mode() {
		t.Fatalf("refusal changed file identity or permissions: %v, %v", actual, err)
	}
}
