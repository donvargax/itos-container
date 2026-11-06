package presentation_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/donvargax/itos-container/internal/engine/session/presentation"
)

func TestRecoveryPresentationClassifiesAndSortsWithoutRawErrors(t *testing.T) {
	for _, example := range []struct {
		failure port.Outcome
		rule    string
		code    int
	}{{port.Usage, "usage", 2}, {port.Refused, "not-owned", 1}, {port.Unsafe, "unsafe-state", 1}, {port.Invalid, "invalid-state", 2}, {port.Environment, "environment", 3}, {port.EnvironmentMissing, "environment", 3}, {port.Uncertain, "temporary", 75}, {port.Incomplete, "temporary", 75}, {port.IntentWrite, "temporary", 75}, {port.OwnershipWrite, "temporary", 75}, {port.CompletionWrite, "temporary", 75}, {port.ExecutionFailed, "temporary", 75}, {port.Internal, "internal", 70}, {"raw-sensitive-marker", "internal", 70}} {
		var out, errout bytes.Buffer
		r := port.RecoveryResult{Failures: []port.RecoveryFailure{{Session: "raw-sensitive-marker", Outcome: example.failure}}}
		code := (presentation.UI{Out: &out, Err: &errout}).Print(r, true)
		var report struct {
			Schema   int
			OK       bool
			Problems []presentation.Problem
			Sessions []port.RecoveryEntry
		}
		if json.Unmarshal(out.Bytes(), &report) != nil || code != example.code || report.Schema != 1 || report.OK || report.Sessions == nil || len(report.Problems) != 1 || report.Problems[0].Rule != "recovery."+example.rule || report.Problems[0].Session != "" || strings.Contains(out.String()+errout.String(), "raw-sensitive-marker") {
			t.Fatal(example, out.String(), errout.String(), code)
		}
	}
	var out, errout bytes.Buffer
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	r := port.RecoveryResult{Failures: []port.RecoveryFailure{{Session: b, Outcome: port.Invalid}, {Session: a, Outcome: port.Incomplete}, {Session: a, Outcome: port.Unsafe}, {Session: a, Outcome: port.Unsafe}}, Entries: []port.RecoveryEntry{{ID: b}, {ID: a}, {ID: a}}}
	if code := (presentation.UI{Out: &out, Err: &errout}).Print(r, true); code != 75 {
		t.Fatal(code)
	}
	var report struct {
		Problems []presentation.Problem
		Sessions []port.RecoveryEntry
	}
	if json.Unmarshal(out.Bytes(), &report) != nil {
		t.Fatal(out.String())
	}
	if report.Problems[0].Session != a || report.Problems[0].Rule != "recovery.temporary" || report.Problems[3].Session != b || report.Sessions[0].ID != a || report.Sessions[2].ID != b {
		t.Fatal(report)
	}
}
func TestPlainRecoveryExplainsEmptyUnknownAndKnownOutcomes(t *testing.T) {
	var out, errout bytes.Buffer
	u := presentation.UI{Out: &out, Err: &errout}
	if u.Print(port.RecoveryResult{}, false) != 0 || !strings.Contains(out.String(), "No owned sessions") {
		t.Fatal(out.String())
	}
	out.Reset()
	exit := 7
	id := strings.Repeat("b", 64)
	r := port.RecoveryResult{Entries: []port.RecoveryEntry{{ID: id, State: "running", Resource: "running", Controller: "absent", Cleanup: "pending", StopCandidate: true, Retry: []string{"itos", "container", "stop", id}}, {ID: strings.Repeat("a", 64), State: "stopped", Resource: "absent", Cleanup: "complete", WorkloadExit: &exit}}}
	if u.Print(r, false) != 0 || !strings.Contains(out.String(), "workload_exit unknown") || !strings.Contains(out.String(), "workload_exit 7") || !strings.Contains(out.String(), "itos container stop "+id) {
		t.Fatal(out.String())
	}
}
