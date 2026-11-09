package porttest

import (
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"testing"
)

func TestEgressConnectionContract(t *testing.T) {
	EgressConnectionContract(t, func() (port.RunEngine, port.Binding) {
		b, _, e := Fixture()
		return &RunEngine{Engine: e}, EgressBinding(b)
	})
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
		RefusalLogContract(t, RefusalLogFixture{Log: log, Read: func(id string) []byte { return log.Copies[id] }, Fault: func(f string) { log.Fault = f }, Report: func(r port.RefusalReport) { log.Report = r }}, input, input, report)
	}
}
