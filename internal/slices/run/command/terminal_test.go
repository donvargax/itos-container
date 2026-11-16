package command

import (
	"context"
	"errors"
	"github.com/donvargax/itos-container/internal/cli"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	terminal "github.com/donvargax/itos-container/internal/engine/terminal/port"
	tf "github.com/donvargax/itos-container/internal/engine/terminal/port/porttest"
	rf "github.com/donvargax/itos-container/internal/slices/run/port/porttest"
	"strings"
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

func TestTerminalRestorationDiagnosticIsBoundedOnEveryReturn(t *testing.T) {
	for _, phase := range []string{"preflight", "zero", "nonzero"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				fake := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}, RestoreError: errors.New("restore fails")}
				approval := &rf.Approval{View: ap.View{Status: "matching", Inspection: af.Ready(), Candidate: ap.Candidate{Project: "p", Canonical: "selected"}, Record: ap.Record{Generation: "1"}}}
				if phase == "preflight" {
					approval.View.Status = "missing"
				}
				exit, want := 0, 75
				if phase == "nonzero" {
					exit, want = 17, 17
				}
				result := sp.RunResult{Exited: true, Exit: exit, Deadline: time.Now().Add(3 * time.Second)}
				runner := &rf.Runner{Result: result}
				release := make(chan struct{})
				writer := &terminalDiagnosticRecorder{release: release, until: time.Now().Add(41 * time.Second)}
				c := CLI{TTY: true, Terminal: fake, Approval: approval, Snapshots: &af.Snapshots{}, Runner: runner, Args: []string{"--", "probe"}}
				start := time.Now()
				code := c.Run(&cli.UI{Stderr: writer, Classify: Classify})
				elapsed := time.Since(start)
				close(release)
				synctest.Wait()
				bound := 3 * time.Second
				if phase == "preflight" {
					bound = 10 * time.Second
				}
				if code != want || elapsed != bound || strings.Count(writer.String(), "run.terminal-restore") != 1 {
					t.Fatalf("terminal return code=%d elapsed=%v diagnostics=%q", code, elapsed, writer.String())
				}
			})
		})
	}
}

type terminalDiagnosticRecorder struct {
	strings.Builder
	release <-chan struct{}
	until   time.Time
}

func (w *terminalDiagnosticRecorder) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	n, err := w.Builder.Write(p)
	timer := time.NewTimer(time.Until(w.until))
	defer timer.Stop()
	select {
	case <-w.release:
	case <-timer.C:
	}
	return n, err
}
