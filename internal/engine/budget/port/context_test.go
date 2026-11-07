package port_test

import (
	"fmt"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/budget/port"
)

func TestFailureDiagnosticDoesNotExposeContext(t *testing.T) {
	failure := port.Failure{Phase: "private-phase-marker", Operation: "private-operation-marker"}
	if got := fmt.Sprint(failure); got != "controller deadline exhausted" {
		t.Fatalf("unsafe deadline diagnostic: %q", got)
	}
}
