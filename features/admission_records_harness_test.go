package features

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdmissionAuditSeparatesOutputFromCleanup(t *testing.T) {
	for _, test := range []struct {
		name, state, problem string
		complete, accepted   bool
	}{
		{"clean completion", "stopped", "", true, true},
		{"completed output timeout", "stopped", "stream-timeout", true, true},
		{"pending output timeout", "failed", "stream-timeout", false, true},
		{"running output timeout", "running", "stream-timeout", true, false},
		{"failed output timeout", "failed", "stream-timeout", true, false},
		{"incomplete cleanup", "stopped", "cleanup-incomplete", true, false},
		{"failed completion write", "stopped", "completion-write-failed", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := &world{doctor: &doctorWorld{checkout: "/checkout", env: map[string]string{"XDG_STATE_HOME": "/state"}}, approval: &approvalWorld{uid: "1000", project: strings.Repeat("a", 64)}}
			id := strings.Repeat("b", 64)
			intent := "intents/" + w.approval.project + "/" + strings.Repeat("c", 64) + ".json"
			var problem any
			if test.problem != "" {
				problem = test.problem
			}
			record, err := json.Marshal(map[string]any{"schema": 1, "id": id, "project": w.approval.project, "intent": intent, "state": test.state, "workload_exit": 137, "last_problem": problem})
			if err != nil {
				t.Fatal(err)
			}
			claim, err := json.Marshal(map[string]any{"schema": 1, "root": w.doctor.checkout, "id": id, "intent": intent, "complete": test.complete})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("/state", "itos-container", "sessions", w.recoveryIndexKey(), id)
			admission := w.admissionPath(id)
			tree := map[string]runEntry{
				path:                               {Mode: os.ModeDir | 0700},
				filepath.Join(path, "record.json"): {Data: string(record)},
				filepath.Dir(admission):            {Mode: os.ModeDir | 0700, Owner: 1000},
				admission:                          {Data: string(claim), Mode: 0600, Owner: 1000, Links: 1},
			}
			allowed, err := w.currentAdmissionPaths(tree)
			if (err == nil) != test.accepted {
				t.Fatalf("admission audit accepted=%t, want %t: %v", err == nil, test.accepted, err)
			}
			if test.accepted && (len(allowed) != 2 || !allowed[admission] || !allowed[filepath.Dir(admission)]) {
				t.Fatal("audit did not constrain allowance to the exact owned certificate", allowed)
			}
		})
	}
}
