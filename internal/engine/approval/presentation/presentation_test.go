package presentation

import (
	"bytes"
	"encoding/json"
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"io"
	"strings"
	"testing"
)

func TestReviewContract(t *testing.T) {
	porttest.ReviewContract(t, func(line string) port.Reviewer {
		return UI{In: strings.NewReader(line), Out: io.Discard, Err: io.Discard}
	})
}
func TestReportsAndFailureRules(t *testing.T) {
	for _, tc := range []struct {
		failure port.Failure
		exit    int
	}{{"identity.root", 1}, {"identity.unavailable", 3}, {"identity.mismatch", 1}, {"approve.confirmation-required", 1}, {"approve.declined", 1}, {"approve.fingerprint", 1}, {"approve.fingerprint-format", 2}, {"approve.changed", 1}, {"approve.conflict", 1}, {"state.location", 2}, {"state.path", 1}, {"state.unsafe", 1}, {"state.invalid", 1}, {"state.unavailable", 3}, {"state.write", 75}} {
		var stdout, stderr bytes.Buffer
		ui := UI{In: strings.NewReader(""), Out: &stdout, Err: &stderr}
		v := port.View{Inspection: porttest.Ready(), Failure: tc.failure, Status: "invalid", Candidate: port.Candidate{Canonical: `{"manifest":{"containerEnv":{"x":"new"},"name":null}}`}, Record: port.Record{Candidate: port.Candidate{Canonical: `{"manifest":{"containerEnv":{"x":"old"},"name":"prior"}}`}, Generation: "1"}}
		if code := ui.Print(v, true); code != tc.exit {
			t.Fatal(tc, code)
		}
		var obj map[string]any
		if json.Unmarshal(stdout.Bytes(), &obj) != nil || obj["ok"] != false || obj["schema"] != float64(1) {
			t.Fatal(stdout.String())
		}
		r := ui.Report(v)
		if len(r.Differences) != 2 || r.Differences[0].Path != "/manifest/containerEnv" || r.Differences[1].Path != "/manifest/name" || r.Generation == nil {
			t.Fatal(r)
		}
		if ui.Terminal() || stderr.Len() == 0 {
			t.Fatal("invalid terminal or missing diagnostic")
		}
	}
}
