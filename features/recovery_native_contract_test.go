package features

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRecoveryReportEvidence(t *testing.T) {
	entry := map[string]any{"id": strings.Repeat("a", 64), "project": strings.Repeat("b", 64),
		"fingerprint": "sha256:" + strings.Repeat("c", 64), "state": "stopped", "resource": "absent", "controller": "absent",
		"cleanup": "complete", "workload_exit": nil, "stop_candidate": false, "retry": nil}
	for _, fault := range []string{"", "verified", "not observed: worker exited", "refused", "invalid detector", "empty detector", "missing unknown exit", "missing retry", "missing controller", "extra field", "wrong casing", "trailing object", "null problems", "failed process"} {
		t.Run(fault, func(t *testing.T) {
			session := approvalClone(entry)
			report := map[string]any{"schema": 1, "ok": true, "problems": []any{}, "sessions": []any{session}}
			switch fault {
			case "verified", "not observed: worker exited", "refused":
				session["mount_detector"] = fault
			case "invalid detector":
				session["mount_detector"] = true
			case "empty detector":
				session["mount_detector"] = ""
			case "missing unknown exit":
				delete(session, "workload_exit")
			case "missing retry":
				delete(session, "retry")
			case "missing controller":
				delete(session, "controller")
			case "extra field":
				session["extra"] = true
			case "wrong casing":
				session["Controller"] = session["controller"]
				delete(session, "controller")
			case "null problems":
				report["problems"] = nil
			}
			data, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			if fault == "trailing object" {
				data = append(data, []byte(`{}`)...)
			}
			p := &recoveryProcess{done: make(chan struct{})}
			close(p.done)
			_, _ = p.out.Write(data)
			if fault == "failed process" {
				p.exit = 75
			}
			_, err = recoveryReadReport(p)
			if (err == nil) != (fault == "" || fault == "verified" || fault == "not observed: worker exited" || fault == "refused") {
				t.Fatalf("result control %q: %v", fault, err)
			}
		})
	}
}

func TestRecoveryPrintedArgv(t *testing.T) {
	id := strings.Repeat("a", 64)
	for _, tc := range []struct {
		text string
		want bool
	}{
		{"itos container stop " + id, true},
		{"cleanup failed\nRetry: itos container stop " + id + "\nUse the same effective project.\n", true},
		{"itos container stop <session>", false},
		{"itos container stop " + strings.ToUpper(id), false},
		{"itos container stop " + id + "a", false},
		{"itos container stop " + id + "; touch marker", false},
		{"itos container stop " + id + " --all", false},
		{"itos container stop intents/" + id + ".json", false},
	} {
		matches := recoveryRetryPattern.FindAllStringSubmatch(tc.text, -1)
		valid := len(matches) == 1 && matches[0][1] == id
		if valid != tc.want {
			t.Fatalf("retry extraction for %q = %v, expected %v", tc.text, valid, tc.want)
		}
	}
}
