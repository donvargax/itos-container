package porttest

import (
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// A home-capable engine wraps the run engine's connection. The wrapper must
// keep the terminal capability, so a terminal run on the home fixture reaches
// the checks after it instead of being refused as unavailable.
func TestHomeEnginePreservesTheRunTerminalContract(t *testing.T) {
	TerminalEngineContract(t, func() (port.RunEngine, port.Binding) {
		b, _, e := Fixture()
		return &HomeEngine{RunEngine: &RunEngine{Engine: e, Exit: 7, Output: []byte("stdout\x00\xff"), ErrorOutput: []byte("stderr\x00\xfe")}}, b
	})
}
