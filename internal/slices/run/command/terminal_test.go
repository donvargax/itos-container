package command

import (
	"context"
	"errors"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	terminal "github.com/donvargax/itos-container/internal/engine/terminal/port"
	tf "github.com/donvargax/itos-container/internal/engine/terminal/port/porttest"
	"testing"
	"testing/synctest"
	"time"
)

type lateTerminal struct {
	provider terminal.Provider
	release  <-chan struct{}
	closed   chan struct{}
}

func (p lateTerminal) Acquire(context.Context) (terminal.Lease, error) {
	<-p.release
	l, e := p.provider.Acquire(context.Background())
	return observedTerminal{Lease: l, closed: p.closed}, e
}

type observedTerminal struct {
	terminal.Lease
	closed chan struct{}
}

func (l observedTerminal) Close() terminal.Result { r := l.Lease.Close(); close(l.closed); return r }

func TestTerminalAcquisitionBudgetReleasesLateLease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fake := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}}
		release, closed := make(chan struct{}), make(chan struct{})
		ctx, end := budget.Start(context.Background(), time.Second, "startup")
		defer end()
		l, err := acquireTerminal(ctx, lateTerminal{fake, release, closed})
		var failure budget.Failure
		if l != nil || !errors.As(err, &failure) {
			t.Fatal("late acquisition admitted", l, err)
		}
		close(release)
		<-closed
		if fake.Raw() {
			t.Fatal("late lease retained raw state")
		}
	})
}

func TestTerminalAcquisitionAndRollbackDiagnostics(t *testing.T) {
	if l, e := acquireTerminal(context.Background(), nil); l != nil || e == nil {
		t.Fatal("missing terminal admitted")
	}
	for _, tc := range []struct {
		kind    terminal.FailureKind
		restore bool
		code    int
		rule    string
	}{
		{terminal.InvalidTerminal, false, 2, "run.terminal-required"},
		{terminal.AcquisitionFailed, false, 3, "run.terminal-unavailable"},
		{terminal.Unsupported, false, 3, "run.terminal-unavailable"},
		{terminal.AcquisitionFailed, true, 75, "run.terminal-restore"},
	} {
		err := &terminal.Failure{Kind: tc.kind}
		if tc.restore {
			err.RestoreErr = errors.New("private rollback payload")
		}
		code, problems := Classify(err)
		if code != tc.code || problems[len(problems)-1].Rule != tc.rule {
			t.Fatal(code, problems)
		}
	}
}
