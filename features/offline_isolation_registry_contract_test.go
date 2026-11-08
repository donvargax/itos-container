package features

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Disposable files exercise the preservation and retry oracles, including a
// successful run followed by failed removal. No native engine is involved.
func TestIsolationRegistryPreservesOnlyValidatedAdditions(t *testing.T) {
	for _, fault := range []string{
		"none", "missing registration", "extra session", "extra project", "extra file", "unsafe lock",
		"changed earlier registration", "changed checkout", "changed host", "preexisting record",
		"id", "intent", "project", "schema", "state", "workload_exit", "last_problem", "mount_detector", "extra field",
	} {
		t.Run(fault, func(t *testing.T) {
			s := isolationRegistryFixture(t)
			r := s.runs[1]
			dir := filepath.Join(s.registrationRoot(), r.sessionID)
			path := filepath.Join(dir, "record.json")
			var err error
			switch fault {
			case "missing registration":
				err = os.Remove(path)
			case "extra session":
				err = os.Mkdir(filepath.Join(s.registrationRoot(), strings.Repeat("f", 64)), 0700)
			case "extra project":
				err = os.Mkdir(filepath.Join(filepath.Dir(s.registrationRoot()), strings.Repeat("f", 64)), 0700)
			case "extra file":
				err = os.WriteFile(filepath.Join(dir, "unexpected"), nil, 0600)
			case "unsafe lock":
				err = os.Chmod(filepath.Join(dir, "controller.lock"), 0644)
			case "changed earlier registration":
				err = os.WriteFile(filepath.Join(s.registrationRoot(), s.runs[0].sessionID, "record.json"), []byte("{}"), 0600)
			case "changed checkout":
				err = os.WriteFile(filepath.Join(s.w.doctor.checkout, "sentinel"), []byte("changed"), 0600)
			case "changed host":
				err = os.WriteFile(filepath.Join(s.w.doctor.host, "sentinel"), []byte("changed"), 0600)
			case "preexisting record":
				s.before[path] = "original preexisting record"
			case "id", "intent", "project", "schema", "state", "workload_exit", "last_problem", "mount_detector", "extra field":
				record, decodeErr := sessionDecodeRecord(r.registration.Bytes)
				if decodeErr != nil {
					t.Fatal(decodeErr)
				}
				record[fault] = "wrong"
				err = os.WriteFile(path, approvalJSON(record), 0600)
				if err == nil {
					r.registration, err = sessionReadRecord(path)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			s.after, err = s.protected()
			if err != nil {
				t.Fatal(err)
			}
			if err = s.registryPreserved(); (err == nil) != (fault == "none") {
				t.Fatalf("preservation control %s: %v", fault, err)
			}
		})
	}
}

func TestIsolationRegisteredRetryRequiresOriginalSessionAndSafeDiagnostic(t *testing.T) {
	for _, fault := range []string{"none", "missing retry", "foreign retry", "token retry", "extra argument", "duplicate retry", "missing project guidance", "body marker", "completion claim"} {
		t.Run(fault, func(t *testing.T) {
			s := isolationRegistryFixture(t)
			r := s.runs[1]
			retry := "itos container stop " + r.sessionID
			diagnostic := fmt.Sprintf("run.cleanup-temporary: %s\nRetry: %s\nUse the same effective project.\n", s.registrationIntent(r), retry)
			switch fault {
			case "missing retry":
				diagnostic = strings.ReplaceAll(diagnostic, retry, "no retry")
			case "foreign retry":
				diagnostic = strings.ReplaceAll(diagnostic, r.sessionID, strings.Repeat("f", 64))
			case "token retry":
				diagnostic = strings.ReplaceAll(diagnostic, retry, "itos container stop "+strings.TrimPrefix(fmt.Sprint(r.expected["name"]), "itos-container-"))
			case "extra argument":
				diagnostic = strings.ReplaceAll(diagnostic, retry, retry+" --all")
			case "duplicate retry":
				diagnostic += retry + "\n"
			case "missing project guidance":
				diagnostic = strings.ReplaceAll(diagnostic, "same effective project", "another project")
			case "body marker":
				diagnostic += "SLICE_14_SYNTHETIC_REMOVE_BODY"
			case "completion claim":
				diagnostic += "cleanup complete"
			}
			r.stderr = []byte(diagnostic)
			if err := s.safeDiagnostic(); (err == nil) != (fault == "none") {
				t.Fatalf("diagnostic control %s: %v", fault, err)
			}
		})
	}
}

func isolationRegistryFixture(t *testing.T) *isolationWorld {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("native registration assertions target Linux")
	}
	w, dir, _ := offlineRegistryFixture(t)
	if err := os.RemoveAll(filepath.Dir(filepath.Dir(dir))); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Dir(w.admissionPath(""))); err != nil {
		t.Fatal(err)
	}
	s := &isolationWorld{w: w}
	var err error
	s.before, err = s.protected()
	if err != nil {
		t.Fatal(err)
	}
	for n, id := range []string{strings.Repeat("b", 64), strings.Repeat("d", 64)} {
		token := []string{strings.Repeat("c", 64), strings.Repeat("e", 64)}[n]
		r := &isolationRun{config: isolationConfig{Nonce: fmt.Sprintf("run-%d", n), Status: 7}, absent: n == 0,
			expected: map[string]any{"name": "itos-container-" + token}}
		path := filepath.Join(s.registrationRoot(), id)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		record := map[string]any{"schema": 1, "id": id, "project": w.approval.project,
			"intent": s.registrationIntent(r), "mount_detector": "verified", "state": "stopped", "workload_exit": 7, "last_problem": nil}
		if n == 1 {
			record["state"], record["last_problem"] = "failed", "cleanup-incomplete"
		}
		for name, data := range map[string][]byte{"record.json": approvalJSON(record), "controller.lock": nil, "operation.lock": nil} {
			if err := os.WriteFile(filepath.Join(path, name), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
		seedAdmissionRecord(t, w, id, s.registrationIntent(r), r.absent)
		if err := s.observeRegistration(r); err != nil {
			t.Fatal(err)
		}
		s.runs = append(s.runs, r)
	}
	s.after, err = s.protected()
	if err != nil {
		t.Fatal(err)
	}
	return s
}
