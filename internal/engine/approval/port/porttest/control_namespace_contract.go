package porttest

import (
	"context"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// ControlNamespaceContract refuses selected state beneath catalog metadata on
// both reads and direct writes, without poisoning later safe transactions.
func ControlNamespaceContract(t Reporter, fixed string, candidate port.Candidate, at func(string) port.Control, snapshot func() string) {
	t.Helper()
	for _, selected := range []string{fixed + "/coordination/itos-container", fixed + "/coordination/nested/itos-container"} {
		for _, write := range []bool{false, true} {
			before := snapshot()
			tx, failure := at(selected).Begin(context.Background(), candidate, write)
			if tx != nil {
				tx.Close()
			}
			if failure != "state.path" || tx != nil {
				t.Errorf("catalog selection %q write=%t: %s, want state.path", selected, write, failure)
			}
			if before != snapshot() {
				t.Errorf("catalog selection wrote state")
			}
		}
	}
	for _, selected := range []string{fixed, fixed + "/coordination-more/itos-container"} {
		for _, write := range []bool{true, false} {
			tx, failure := at(selected).Begin(context.Background(), candidate, write)
			if tx != nil {
				tx.Close()
			}
			if failure != "" || tx == nil {
				t.Errorf("safe selection %q write=%t: %s", selected, write, failure)
			}
		}
	}
}
