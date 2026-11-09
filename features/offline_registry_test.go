package features

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// These controls exercise the native preservation assertion with disposable
// files only. They never set up, connect to or claim evidence from an engine.
func TestOfflineRegistryPreservation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native protected-registry assertions target Linux")
	}
	for _, fault := range []string{
		"none", "missing registry", "missing lock", "lock contents", "symlink lock", "hardlinked lock",
		"unsafe directory", "unsafe record", "wrong owner", "extra file", "extra session", "other project",
		"changed checkout", "changed host", "changed preexisting record", "changed resources", "operation token reused",
		"schema", "id", "project", "intent", "state", "workload_exit", "last_problem", "mount_detector", "extra field",
	} {
		t.Run(fault, func(t *testing.T) {
			w, dir, record := offlineRegistryFixture(t)
			root := filepath.Dir(filepath.Dir(dir))
			lock := filepath.Join(dir, "controller.lock")
			var err error
			switch fault {
			case "missing registry":
				err = os.RemoveAll(root)
			case "missing lock":
				err = os.Remove(lock)
			case "lock contents":
				err = os.WriteFile(lock, []byte("unexpected"), 0600)
			case "symlink lock", "hardlinked lock":
				if err = os.Remove(lock); err == nil {
					other := filepath.Join(dir, "operation.lock")
					if fault == "symlink lock" {
						err = os.Symlink(other, lock)
					} else {
						err = os.Link(other, lock)
					}
				}
			case "unsafe directory":
				err = os.Chmod(dir, 0755)
			case "unsafe record":
				err = os.Chmod(filepath.Join(dir, "record.json"), 0644)
			case "wrong owner":
				w.approval.uid += "1"
			case "extra file":
				err = os.WriteFile(filepath.Join(dir, "unexpected"), nil, 0600)
			case "extra session":
				err = os.Mkdir(filepath.Join(filepath.Dir(dir), strings.Repeat("d", 64)), 0700)
			case "other project":
				err = os.Mkdir(filepath.Join(root, strings.Repeat("d", 64)), 0700)
			case "changed checkout":
				err = os.WriteFile(filepath.Join(w.doctor.checkout, "sentinel"), []byte("changed"), 0600)
			case "changed host":
				err = os.WriteFile(filepath.Join(w.doctor.host, "sentinel"), []byte("changed"), 0600)
			case "changed preexisting record":
				record["state"] = "failed"
				path := filepath.Join(dir, "record.json")
				if err = os.WriteFile(path, approvalJSON(record), 0600); err != nil {
					break
				}
				w.offline.before, err = w.offlineProtected()
				if err == nil {
					record["state"] = "stopped"
					err = os.WriteFile(path, approvalJSON(record), 0600)
				}
			case "changed resources":
				w.offline.resourcesAfter["unrelated"] = "changed"
			case "operation token reused":
				w.session.token = filepath.Base(dir)
			case "schema", "id", "project", "intent", "state", "workload_exit", "last_problem", "mount_detector", "extra field":
				record[fault] = "wrong"
				err = os.WriteFile(filepath.Join(dir, "record.json"), approvalJSON(record), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			w.offline.after, err = w.offlineProtected()
			if err != nil {
				t.Fatal(err)
			}
			err = w.offlineUnrelated()
			if fault == "none" && err != nil {
				t.Fatal("required registration counted as an unrelated change:", err)
			}
			if fault != "none" && err == nil {
				t.Fatal("preservation accepted", fault)
			}
		})
	}
}

func offlineRegistryFixture(t *testing.T) (*world, string, map[string]any) {
	t.Helper()
	root := t.TempDir()
	checkout, host := filepath.Join(root, "checkout"), filepath.Join(root, "host")
	state := filepath.Join(host, "state")
	project, id, token := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	w := &world{
		doctor:   &doctorWorld{checkout: checkout, host: host, env: map[string]string{"XDG_STATE_HOME": state}},
		approval: &approvalWorld{project: project, uid: strconv.Itoa(os.Geteuid())},
		session:  &sessionWorld{token: token, reference: "intents/" + project + "/" + token + ".json"},
		offline:  &offlineRuntime{resourcesBefore: map[string]string{}, resourcesAfter: map[string]string{}},
	}
	for _, dir := range []string{checkout, filepath.Join(state, "itos-container")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{checkout, host} {
		if err := os.WriteFile(filepath.Join(dir, "sentinel"), []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var err error
	w.offline.before, err = w.offlineProtected()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(state, "itos-container", "sessions", w.recoveryIndexKey(), id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	record := map[string]any{"schema": 1, "id": id, "project": project, "intent": w.session.reference,
		"mount_detector": "verified", "state": "stopped", "workload_exit": 0, "last_problem": nil}
	for name, data := range map[string][]byte{"record.json": approvalJSON(record), "controller.lock": nil, "operation.lock": nil} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	seedAdmissionRecord(t, w, id, w.session.reference, true)
	return w, dir, record
}
