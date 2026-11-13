package domain

import (
	"testing"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Slice-27's step checkpoint: nothing is deliverable to the proxy until the
// rule is built, not even the observation the rule will admit.
func TestProxyExecutableRefusesEveryObservationUntilBuilt(t *testing.T) {
	static := port.Executable{Path: "/installed/itos-container", Regular: true, OS: "linux", Architecture: "amd64"}
	for _, failure := range []port.Failure{port.None, port.EgressExecutable} {
		if got := ProxyExecutable(static, failure, "/checkout"); got != port.EgressExecutable {
			t.Errorf("observation failure %v judged %v", failure, got)
		}
	}
}
