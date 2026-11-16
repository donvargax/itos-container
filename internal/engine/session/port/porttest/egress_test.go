package porttest

import (
	"bytes"
	"fmt"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"testing"
)

func TestEgressConnectionContract(t *testing.T) {
	EgressConnectionContract(t, func() (port.RunEngine, port.Binding) {
		b, _, e := Fixture()
		return &RunEngine{Engine: e}, EgressBinding(b)
	})
}

type refusalContractReporter struct{ errors []string }

func (*refusalContractReporter) Helper() {}
func (r *refusalContractReporter) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func TestRefusalLogContractRequiresExplicitSession(t *testing.T) {
	r := &refusalContractReporter{}
	// No log can be called in this incomplete fixture: setup must reject it.
	RefusalLogContract(r, RefusalLogFixture{}, []byte("input"), []byte("stored"), []byte("other input"), []byte("other stored"), port.RefusalReport{})
	if len(r.errors) != 1 || r.errors[0] != "refusal log fixture requires a session ID" {
		t.Fatalf("missing ID diagnostics = %v", r.errors)
	}
}
func TestRefusalLogContract(t *testing.T) {
	for _, gap := range []bool{false, true} {
		log := &RefusalLog{}
		input := []byte(ProxyStdout)
		report := port.RefusalReport{}
		if gap {
			input = []byte("{\"schema\":1,\"seq\":1,\"time\":\"2026-11-09T12:00:00Z\",\"event\":\"ready\"}\n{\"schema\":1,\"seq\":3,\"time\":\"2026-11-09T12:00:01Z\",\"event\":\"refused\",\"reason\":\"not-listed\"}\n")
			report = port.RefusalReport{Outcome: port.RefusalLogGap, Gaps: []port.RefusalGap{{First: 2, Last: 2}}}
		}
		replacement := bytes.ReplaceAll(input, []byte("not-listed"), []byte("port"))
		RefusalLogContract(t, RefusalLogFixture{SessionID: "session", Log: log, Read: func(id string) []byte { return log.Copies[id] }, Fault: func(f string) { log.Fault = f }, Report: func(r port.RefusalReport) { log.Report = r }}, input, input, replacement, replacement, report)
	}
}
