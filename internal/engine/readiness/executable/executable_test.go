package executable

import (
	"testing"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Slice-27's step checkpoint: the adapter reads nothing yet, and says so as a
// failure with no observation beside it.
func TestProgramObservesNothingUntilBuilt(t *testing.T) {
	observed, failure := Program{Path: func() (string, error) { return "/installed/itos-container", nil }}.Observe()
	if failure != port.EgressExecutable || observed != (port.Executable{}) {
		t.Errorf("observed %+v with failure %v", observed, failure)
	}
}
