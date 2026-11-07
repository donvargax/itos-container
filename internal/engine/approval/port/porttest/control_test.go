package porttest

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"testing"
)

func TestControlContract(t *testing.T) {
	ControlContract(t, func() ControlFixture {
		state := NewControlState()
		reservations := &Reservations{RootsByLocation: map[string][]string{}}
		first := Control{State: state, Account: &Account{Home: "/account"}, Reservations: reservations, Fixed: "/account/.local/state/itos-container", Selected: "/selected/itos-container"}
		second := first
		second.Selected = "/other/itos-container"
		return ControlFixture{First: first, Second: second, Candidate: port.Candidate{Root: "/checkout"},
			Expose: func(on bool) {
				if on {
					tx, f := second.Begin(context.Background(), port.Candidate{Root: "/checkout"}, true)
					if f != "" {
						t.Fatal(f)
					}
					tx.Close()
					reservations.RootsByLocation["/worker-state"] = []string{"/selected"}
				} else {
					delete(reservations.RootsByLocation, "/worker-state")
				}
			},
			WriteFault: func(on bool) { state.WriteFault = on }, Snapshot: state.Snapshot, Durable: func() bool { return state.Bound[first.Selected] }, Replace: func() { state.Replaced = true },
		}
	})
}
