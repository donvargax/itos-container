package porttest

import (
	"context"
	"fmt"
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
		return ControlFixture{First: first, Second: second, Candidate: port.Candidate{Root: "/checkout"}, Location: first.Selected,
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

func TestControlPendingLocationProtectsCandidate(t *testing.T) {
	state := NewControlState()
	state.Bound["/selected/itos-container"] = false
	control := Control{State: state, Account: &Account{Home: "/account"}, Reservations: &Reservations{}, Fixed: "/account/.local/state/itos-container", Selected: "/other/itos-container"}
	ControlCandidateContract(t, control, port.Candidate{Root: "/checkout"}, "/selected/itos-container", state.Snapshot)
}

func TestControlNamespaceContract(t *testing.T) {
	for _, initialized := range []bool{false, true} {
		t.Run(fmt.Sprint(initialized), func(t *testing.T) {
			state := NewControlState()
			control := Control{State: state, Account: &Account{Home: "/account"}, Reservations: &Reservations{}, Fixed: "/account/.local/state/itos-container", Selected: "/selected/itos-container"}
			candidate := port.Candidate{Root: "/checkout"}
			if initialized {
				tx, failure := control.Begin(t.Context(), candidate, true)
				if failure != "" {
					t.Fatal(failure)
				}
				tx.Close()
			}
			ControlNamespaceContract(t, control.Fixed, candidate, func(selected string) port.Control {
				next := control
				next.Selected = selected
				return next
			}, state.Snapshot)
		})
	}
}
