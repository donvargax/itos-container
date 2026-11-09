package porttest

import (
	"context"
	"strings"
	"time"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// ControlFixture drives the same separation, cancellation, persistence and
// uncertain-publication contract through memory and protected disk adapters.
type ControlFixture struct {
	First, Second port.Control
	Candidate     port.Candidate
	Location      string
	Expose        func(bool)
	WriteFault    func(bool)
	Snapshot      func() string
	Durable       func() bool
	Replace       func()
}

func ControlContract(t Reporter, open func() ControlFixture) {
	t.Helper()
	f := open()
	ctx := context.Background()
	before := f.Snapshot()
	read, failure := f.First.Begin(ctx, f.Candidate, false)
	if failure != "" {
		t.Errorf("read-only empty inspection: %s", failure)
		return
	}
	read.Close()
	if f.Snapshot() != before {
		t.Errorf("inspection created control state")
	}
	f.Expose(true)
	before = f.Snapshot()
	for _, write := range []bool{false, true} {
		tx, failure := f.First.Begin(ctx, f.Candidate, write)
		if tx != nil {
			tx.Close()
		}
		if failure != "state.exposed" || tx != nil {
			t.Errorf("exposed location accepted: %s", failure)
		}
	}
	if f.Snapshot() != before {
		t.Errorf("exposure refusal wrote state")
	}
	f.Expose(false)
	f.WriteFault(true)
	tx, failure := f.First.Begin(ctx, f.Candidate, true)
	if tx != nil {
		tx.Close()
	}
	if failure != "state.write" || tx != nil || f.Durable() {
		t.Errorf("failed publication granted authority: %s", failure)
	}
	f.WriteFault(false)
	retained := f.Snapshot()
	read, failure = f.First.Begin(ctx, f.Candidate, false)
	if failure != "" {
		t.Errorf("safe interrupted bootstrap inspection: %s", failure)
		return
	}
	read.Close()
	if retained != f.Snapshot() {
		t.Errorf("inspection repaired interrupted state")
	}
	tx, failure = f.First.Begin(ctx, f.Candidate, true)
	if failure != "" {
		t.Errorf("fresh retry: %s", failure)
		return
	}
	if !tx.Unchanged() || !f.Durable() {
		t.Errorf("publication lacks durable identity")
	}
	deadline, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	other, blocked := f.Second.Begin(deadline, f.Candidate, true)
	cancel()
	if other != nil {
		other.Close()
	}
	if blocked != "state.busy" || other != nil {
		t.Errorf("another selected root bypassed exclusion: %s", blocked)
	}
	tx.Close()
	retained = f.Snapshot()
	tx, failure = f.First.Begin(ctx, f.Candidate, true)
	if failure != "" {
		t.Errorf("matching reuse: %s", failure)
		return
	}
	tx.Close()
	if retained != f.Snapshot() {
		t.Errorf("reuse replaced persistent bindings")
	}
	other, failure = f.Second.Begin(ctx, f.Candidate, true)
	if failure != "" {
		t.Errorf("independent root after release: %s", failure)
		return
	}
	other.Close()
	ControlCandidateContract(t, f.Second, f.Candidate, f.Location, f.Snapshot)
	tx, failure = f.First.Begin(ctx, f.Candidate, false)
	if failure != "" {
		t.Errorf("retained first root: %s", failure)
		return
	}
	f.Replace()
	if tx.Unchanged() {
		t.Errorf("replaced identity still trusted")
	}
	tx.Close()
	tx, failure = f.First.Begin(ctx, f.Candidate, false)
	if tx != nil {
		tx.Close()
	}
	if failure != "state.path" {
		t.Errorf("replaced cataloged root: %s", failure)
	}
}

// ControlCandidateContract checks retained protection without any outstanding
// reservation, for both inspection and admission from another selected root.
// It also applies to a pending claim whose managed directory does not yet exist.
func ControlCandidateContract(t Reporter, control port.Control, candidate port.Candidate, location string, snapshot func() string) {
	t.Helper()
	parent := location[:strings.LastIndex(location, "/")]
	for _, test := range []struct {
		root string
		want port.Failure
	}{
		{location, "state.path"},
		{parent, "state.path"},
		{location + "/approvals", "state.path"}, // nested checkouts mount the records
		{location + "/approvals/nested/checkout", "state.path"},
		{location[:len(location)-1], ""}, // textual prefix, not a path component
		{location + "-more/approvals", ""},
		{parent + "-more/itos-container", ""},
		{parent + "/other/itos-container", ""}, // same basename, disjoint tree
	} {
		candidate.Root = test.root
		for _, write := range []bool{false, true} {
			before := snapshot()
			tx, failure := control.Begin(context.Background(), candidate, write)
			if tx != nil {
				tx.Close()
			}
			if failure != test.want || (tx == nil) != (test.want != "") {
				t.Errorf("candidate %q write=%t: %s, want %s", test.root, write, failure, test.want)
			}
			if test.want != "" && snapshot() != before {
				t.Errorf("refused candidate changed retained state")
			}
		}
	}
}
