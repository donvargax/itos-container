package disk

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// controlBudgetScan stands for the session engine's reservation scan on a state
// tree with many retained sessions. While slow reports true it works until the
// caller's budget ends and then gives up the way that scan does: with a refusal
// of its own (a path that no longer compares equal, an intent it could not
// open), never with the context's error.
type controlBudgetScan struct {
	port.Reservations
	slow    func() bool
	end     func()
	failure port.Failure
}

func (r *controlBudgetScan) Roots(ctx context.Context, c port.Candidate, path string) ([]string, port.Failure) {
	if !r.slow() {
		return r.Reservations.Roots(ctx, c, path)
	}
	r.end()
	<-ctx.Done()
	return nil, r.failure
}

// The call budget covers the lock wait and the scan together. A scan that the
// budget interrupts has seen nothing wrong with the tree: the refusal it
// returns is contention, which the person retries, so Begin reports it as
// state.busy instead of sending them to look for a substituted path.
func TestControlBudgetEndingDuringScanIsBusy(t *testing.T) {
	cases := []struct {
		name         string
		write        bool
		bootstrapped bool
		contended    bool
		cancel       bool
		underLock    bool
		failure      port.Failure
	}{
		{name: "reader after a lock wait, path", bootstrapped: true, contended: true, cancel: true, failure: "state.path"},
		{name: "reader after a lock wait, unavailable", bootstrapped: true, contended: true, cancel: true, failure: "state.unavailable"},
		{name: "writer after a lock wait", write: true, bootstrapped: true, contended: true, cancel: true, failure: "state.path"},
		{name: "reader without a lock, deadline", failure: "state.path"},
		{name: "writer unlocked scan, deadline", write: true, failure: "state.path"},
		{name: "writer scan under the new lock, deadline", write: true, underLock: true, failure: "state.path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			control, c, reservations, home, _ := controlFixture(t)
			lock := filepath.Join(home, ".local/state/itos-container/coordination", controlLockName)
			var peer port.ControlTransaction
			if tc.bootstrapped {
				var f port.Failure
				if peer, f = control.Begin(t.Context(), c, true); f != "" {
					t.Fatal(f)
				}
				if !tc.contended {
					peer.Close()
				}
			}
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			end := func() {}
			if tc.cancel {
				ctx, cancel = context.WithCancel(t.Context())
				defer cancel()
				end = cancel
			}
			control.Reservations = &controlBudgetScan{Reservations: reservations, end: end, failure: tc.failure, slow: func() bool {
				if !tc.underLock {
					return true
				}
				_, err := os.Lstat(lock)
				return err == nil
			}}
			type result struct {
				tx port.ControlTransaction
				f  port.Failure
			}
			done := make(chan result, 1)
			go func() {
				tx, f := control.Begin(ctx, c, tc.write)
				done <- result{tx, f}
			}()
			if peer != nil && tc.contended {
				peer.Close()
			}
			got := <-done
			if got.tx != nil {
				got.tx.Close()
			}
			if got.f != "state.busy" || got.tx != nil {
				t.Fatalf("budget ended during the scan: %v %q, want state.busy", got.tx, got.f)
			}
		})
	}
}

// A refusal found while the budget lasts is still the scan's own: only an ended
// budget turns it into contention.
func TestControlRefusalInsideBudgetIsReported(t *testing.T) {
	for _, write := range []bool{false, true} {
		t.Run(map[bool]string{false: "reader", true: "writer"}[write], func(t *testing.T) {
			control, c, reservations, _, _ := controlFixture(t)
			reservations.Failure = "state.path"
			tx, f := control.Begin(t.Context(), c, write)
			if tx != nil {
				tx.Close()
			}
			if f != "state.path" {
				t.Fatalf("refusal inside the budget: %q, want state.path", f)
			}
		})
	}
}
