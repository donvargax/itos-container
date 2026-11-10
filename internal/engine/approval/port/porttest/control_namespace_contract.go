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

// ControlCollectionContract protects reservation metadata in both prospective
// fixed trees and enrolled locations, including incomplete location claims.
func ControlCollectionContract(t Reporter, root string, candidate port.Candidate, at func(string) port.Control, snapshot func() string) {
	t.Helper()
	for _, collection := range []string{"sessions", "admission"} {
		for _, suffix := range []string{"/itos-container", "/nested/itos-container"} {
			selected := root + "/" + collection + suffix
			for _, write := range []bool{false, true} {
				before := snapshot()
				tx, failure := at(selected).Begin(context.Background(), candidate, write)
				if tx != nil {
					tx.Close()
				}
				if failure != "state.path" || tx != nil {
					t.Errorf("collection selection %q write=%t: %s, want state.path", selected, write, failure)
				}
				if before != snapshot() {
					t.Errorf("collection selection wrote state")
				}
			}
		}
	}
}

// ControlNestedCheckoutContract refuses a checkout rooted below a protected
// tree on reads and direct writes, as it refuses one above it, without changing
// state. Whole-component siblings stay usable for independent checkouts.
func ControlNestedCheckoutContract(t Reporter, trees []string, candidate port.Candidate, control port.Control, snapshot func() string) {
	t.Helper()
	for _, tree := range trees {
		for _, root := range []string{tree + "/approvals", tree + "/approvals/nested/checkout", tree + "/other"} {
			for _, write := range []bool{false, true} {
				candidate.Root = root
				before := snapshot()
				tx, failure := control.Begin(context.Background(), candidate, write)
				if tx != nil {
					tx.Close()
				}
				if failure != "state.path" || tx != nil {
					t.Errorf("checkout %q write=%t: %s, want state.path", root, write, failure)
				}
				if before != snapshot() {
					t.Errorf("checkout %q write=%t changed state", root, write)
				}
			}
		}
	}
	for _, tree := range trees {
		candidate.Root = tree + "-more/approvals"
		for _, write := range []bool{false, true} {
			tx, failure := control.Begin(context.Background(), candidate, write)
			if tx != nil {
				tx.Close()
			}
			if failure != "" || tx == nil {
				t.Errorf("sibling checkout %q write=%t: %s", candidate.Root, write, failure)
			}
		}
	}
}
