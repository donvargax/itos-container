//go:build unix

package disk

import (
	"golang.org/x/sys/unix"
	"testing"
)

func TestPrivatePermissionsIgnoreRestrictiveUmask(t *testing.T) {
	store, a, _ := diskFixture(t)
	previous := unix.Umask(0777)
	defer unix.Umask(previous)
	if _, failure := store.Commit(a, ""); failure != "" {
		t.Fatal(failure)
	}
	if _, failure := store.Read(a); failure != "" {
		t.Fatal(failure)
	}
}
