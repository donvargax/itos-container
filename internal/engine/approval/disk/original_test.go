package disk

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Home retirement reads claim evidence while it holds the home scope lock, and
// a session operation holds the intents entry lock for its whole lifetime while
// it prepares or reconciles a home under that scope. If ReadOriginal waited for
// the entry lock, each side would hold exactly what the other waits for, and
// retirement would report the session operation's lock as a temporary failure
// instead of judging the claim. Reading claim evidence must take no session
// operation lock, as unlocked Locations discovery does; the pinned directory
// and record identity revalidation before and after the read keep it safe.
func TestOriginalReadWaitsForNoSessionOperationLock(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("protected storage target is Linux")
	}
	control, c, _, _, _ := controlFixture(t)
	storage := Storage{Store: control.Store}
	path := filepath.Join(control.Store.StateHome, "itos-container", "intents", c.Project)
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "1.json"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	entry, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = entry.Close() }()
	if err := tryLock(entry); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unlock(entry) }()
	data, failure := storage.ReadOriginal(t.Context(), c, "intents", "1.json")
	if failure != "" || string(data) != "original" {
		t.Fatalf("read under a peer session operation lock: %s %q", failure, string(data))
	}
}
