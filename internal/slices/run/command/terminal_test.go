package command

import (
	"context"
	"errors"
	"fmt"
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

type partialTerminal struct {
	terminal.Provider
	failure error
}

func (p partialTerminal) Acquire(ctx context.Context) (terminal.Lease, error) {
	lease, err := p.Provider.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	return lease, p.failure
}

func TestTerminalAcquisitionRestoresPartialLeaseOnError(t *testing.T) {
	fake := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}, State: 9}
	before := fake.Snapshot()
	failure := &terminal.Failure{Kind: terminal.AcquisitionFailed}
	lease, err := acquireTerminal(t.Context(), partialTerminal{fake, failure})
	if lease != nil || !errors.Is(err, failure) || fake.Snapshot() != before {
		t.Fatalf("partial acquisition returned lease %v, error %v, terminal state %v", lease, err, fake.Snapshot())
	}
}

func TestTerminalAcquisitionRefusesAlreadyCancelledStartup(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	fake := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}}
	lease, err := acquireTerminal(ctx, fake)
	if lease != nil || !errors.Is(err, context.Canceled) || fake.Raw() {
		t.Fatalf("cancelled acquisition returned lease %v, error %v, raw %t", lease, err, fake.Raw())
	}
}

func TestTerminalAcquisitionFailureClosesLateLeaseBeforeReturn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fake := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}}
		release, closed := make(chan struct{}), make(chan struct{})
		go func() {
			// The acquisition finishes after the finite call budget expires.
			time.Sleep(15 * time.Second)
			close(release)
		}()
		ctx, end := budget.Start(context.Background(), time.Hour, "startup")
		defer end()
		l, err := acquireTerminal(ctx, lateTerminal{fake, release, closed})
		var failure budget.Failure
		var problems []string
		if l != nil || !errors.As(err, &failure) {
			problems = append(problems, fmt.Sprintf("late acquisition admitted: %v %v", l, err))
		}
		select {
		case <-closed:
		default:
			problems = append(problems, "acquireTerminal returned before the late lease was closed")
			// Release the acquisition this return did not join so cleanup settles.
			close(release)
		}
		synctest.Wait()
		if fake.Raw() {
			problems = append(problems, "late lease retained raw state")
		}
		if len(problems) > 0 {
			t.Fatal(strings.Join(problems, "; "))
		}
	})
}

// A host signal that lands while the terminal is being acquired keeps the
// run.cancelled result of every other startup phase: the interrupt cause is
// reported, not classified as an internal error.
func TestHostSignalDuringTerminalAcquisitionReportsCancellation(t *testing.T) {
	for _, tc := range []struct {
		signal string
		code   int
	}{{"SIGINT", 130}, {"SIGTERM", 143}} {
		t.Run(tc.signal, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				parent, cancel := context.WithCancelCause(context.Background())
				defer cancel(nil)
				approval, snapshots, runner := startupApprovalFixture(), &af.Snapshots{}, &rf.Runner{}
				provider := interruptingTerminal{interrupt: cancel, signal: tc.signal}
				c := CLI{TTY: true, Context: parent, Terminal: provider, Approval: approval, Snapshots: snapshots, Runner: runner, Args: []string{"--", "probe"}}
				var out, diagnostics strings.Builder
				start := time.Now()
				code := c.Run(&cli.UI{Stdout: &out, Stderr: &diagnostics, Classify: Classify})
				elapsed := time.Since(start)
				if code != tc.code || diagnostics.String() != "itos-container: run.cancelled "+tc.signal+"; workload outcome not-started\n" || out.Len() != 0 || elapsed != 0 {
					t.Fatal("an acquisition-window host signal lost its typed cancellation", code, elapsed, diagnostics.String())
				}
				if approval.Inspections != 0 || runner.Argv != nil {
					t.Fatal("an interrupted acquisition entered a later startup phase")
				}
			})
		})
	}
}

// interruptingTerminal delivers the host signal as soon as the acquisition
// begins, then ends with the acquisition lifetime as the joined cleanup does.
type interruptingTerminal struct {
	interrupt context.CancelCauseFunc
	signal    string
}

func (p interruptingTerminal) Acquire(ctx context.Context) (terminal.Lease, error) {
	p.interrupt(sp.Interrupt{Signal: p.signal})
	<-ctx.Done()
	return nil, &terminal.Failure{Kind: terminal.Cancelled, Err: ctx.Err()}
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
