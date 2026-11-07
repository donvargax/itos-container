package porttest

import (
	"context"
	"time"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// ControlFixture drives the same separation, cancellation, persistence and
// uncertain-publication contract through memory and protected disk adapters.
type ControlFixture struct {
	First, Second port.Control
	Candidate     port.Candidate
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
	if blocked != "state.unavailable" || other != nil {
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
