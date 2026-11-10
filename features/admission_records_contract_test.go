package features

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func seedAdmissionRecord(t *testing.T, w *world, id, intent string, complete bool) {
	t.Helper()
	path := w.admissionPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	claim := map[string]any{"schema": 1, "root": w.doctor.checkout, "id": id, "intent": intent, "complete": complete}
	if err := os.WriteFile(path, approvalJSON(claim), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestAdmissionPreservationRequiresExactProtectedBinding(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native ownership assertions use Linux metadata")
	}
	for _, fault := range []string{"none", "missing", "directory mode", "record mode", "symlink", "hardlink", "extra file", "schema", "root", "id", "intent", "complete", "extra field"} {
		t.Run(fault, func(t *testing.T) {
			w, dir, _ := offlineRegistryFixture(t)
			id := filepath.Base(dir)
			path := w.admissionPath(id)
			var err error
			switch fault {
			case "missing":
				err = os.Remove(path)
			case "directory mode":
				err = os.Chmod(filepath.Dir(path), 0755)
			case "record mode":
				err = os.Chmod(path, 0644)
			case "symlink":
				if err = os.Rename(path, path+"-old"); err == nil {
					err = os.Symlink(path+"-old", path)
				}
			case "hardlink":
				err = os.Link(path, path+"-link")
			case "extra file":
				err = os.WriteFile(filepath.Join(filepath.Dir(path), "unrelated"), nil, 0600)
			case "schema", "root", "id", "intent", "complete", "extra field":
				claim := map[string]any{"schema": 1, "root": w.doctor.checkout, "id": id, "intent": w.session.reference, "complete": true}
				claim[fault] = "wrong"
				err = os.WriteFile(path, approvalJSON(claim), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			w.offline.after, err = w.offlineProtected()
			if err != nil {
				t.Fatal(err)
			}
			if err = w.offlineUnrelated(); (err == nil) != (fault == "none") {
				t.Fatalf("admission preservation %s: %v", fault, err)
			}
		})
	}
}
