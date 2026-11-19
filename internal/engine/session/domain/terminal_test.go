package domain

import (
	"context"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	terminal "github.com/donvargax/itos-container/internal/engine/terminal/port"
	tf "github.com/donvargax/itos-container/internal/engine/terminal/port/porttest"
	"slices"
	"testing"
	"testing/synctest"
	"time"
)

type noTerminalConnection struct{ port.RunConnection }

type noTerminalEngine struct{ port.RunEngine }

func (e noTerminalEngine) OpenRun(ctx context.Context, b port.Binding) (port.RunConnection, port.Outcome) {
	c, f := e.RunEngine.OpenRun(ctx, b)
	return noTerminalConnection{c}, f
}

type invalidInitialTerminal struct{ terminal.Lease }

func (invalidInitialTerminal) Initial() terminal.Dimensions { return terminal.Dimensions{} }

func TestRegisteredLaunchRefusesUnsupportedTerminalBeforeCreate(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "invalid size", true: "missing capability"}[missing], func(t *testing.T) {
			runner, registry, engine, workspace, id := registeredSessionFixture(t)
			provider := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}}
			lease, err := provider.Acquire(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			selected := terminal.Lease(invalidInitialTerminal{lease})
			if missing {
				runner.Engine = noTerminalEngine{runner.Engine}
				selected = lease
			}
			c, a, worker, failure := runner.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{Terminal: selected})
			if c != nil {
				defer c.Close()
			}
			if failure != port.TerminalUnavailable || a != nil || worker != "" || len(engine.Resources) != 0 || registry.Locks[id] || !workspace.Closed {
				t.Fatalf("terminal refusal advanced launch or retained authority: %s %q", failure, worker)
			}
		})
	}
}

func TestMissingTerminalCapabilityRefuses(t *testing.T) {
	c := finiteConnection{RunConnection: noTerminalConnection{}}
	if f := c.SetTerminal(terminal.Dimensions{Columns: 113, Rows: 37}); f != port.TerminalUnavailable {
		t.Fatal(f)
	}
	if f := c.Resize(t.Context(), "worker", terminal.Dimensions{Columns: 113, Rows: 37}); f != port.TerminalResize {
		t.Fatal(f)
	}
}

// A client terminal that already ended must keep its owned workload from
// reaching an irreversible launch edge: no worker may be created, attached
// or started for a client that is gone, whatever ended the lease.
func TestRegisteredLaunchRefusesLostClientBeforeIrreversibleEdges(t *testing.T) {
	for _, tc := range []struct {
		name string
		lost func(*tf.Fake)
		want port.Outcome
	}{
		{"hangup", func(f *tf.Fake) { f.Hangup(false) }, port.TerminalDisconnected},
		{"observation", func(f *tf.Fake) { f.Break() }, port.TerminalUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner, registry, engine, workspace, id := registeredSessionFixture(t)
			provider := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}}
			lease, err := provider.Acquire(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			tc.lost(provider)
			c, a, worker, failure := runner.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{Terminal: lease})
			if c != nil {
				defer c.Close()
			}
			if failure != tc.want || a != nil || worker != "" || len(engine.Resources) != 0 || registry.Locks[id] || !workspace.Closed {
				t.Fatalf("a lost client reached an irreversible launch edge: %s %q", failure, worker)
			}
		})
	}
}

func TestRegisteredLaunchStopsAtClientLossDuringCreateOrAttach(t *testing.T) {
	for _, tc := range []struct {
		event    string
		lose     func(*tf.Fake)
		outcome  port.Outcome
		attached bool
	}{
		{"create", func(f *tf.Fake) { f.Hangup(false) }, port.TerminalDisconnected, false},
		{"attach", (*tf.Fake).Break, port.TerminalUnavailable, true},
	} {
		t.Run(tc.event, func(t *testing.T) {
			runner, registry, engine, _, id := registeredSessionFixture(t)
			provider := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}}
			lease, err := provider.Acquire(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			engine.After = func(event string) {
				if event == tc.event {
					tc.lose(provider)
					<-lease.Done()
				}
			}
			conn, attachment, _, failure := runner.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{Terminal: lease})
			if conn != nil {
				defer conn.Close()
			}
			if attachment != nil {
				defer attachment.Close()
			}
			if failure != tc.outcome || registry.Locks[id] || len(engine.Resources) != 1 {
				t.Fatalf("launch returned %q, lock held %t, workers %d", failure, registry.Locks[id], len(engine.Resources))
			}
			if (attachment != nil) != tc.attached || slices.Contains(engine.Events, "attach") != tc.attached || slices.Contains(engine.Events, "start") {
				t.Fatalf("unexpected launch operations after client loss: %v", engine.Events)
			}
		})
	}
}

func TestStreamFailureRechecksTerminalBeforeDelayedNotification(t *testing.T) {
	for _, tc := range []struct {
		name           string
		broken, exited bool
		exit           int
		want           port.Outcome
	}{
		{"hangup", false, false, 0, port.TerminalDisconnected},
		{"observation fails", true, false, 0, port.TerminalUnavailable},
		{"observed zero exit", false, true, 0, ""},
		{"observed nonzero exit", false, true, 17, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				runner, _, engine, _, id := registeredSessionFixture(t)
				provider := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}, ObservationDelay: time.Hour}
				lease, err := provider.Acquire(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				defer lease.Close()
				c, a, worker, failure := runner.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{Terminal: lease})
				if failure != "" {
					t.Fatal(failure)
				}
				defer c.Close()
				defer a.Close()
				engine.Exit = tc.exit
				if !tc.exited {
					engine.WaitFailure = port.ExecutionFailed
				}
				engine.AfterWait = func() {
					if tc.broken {
						provider.Break()
					} else {
						provider.Hangup(false)
					}
				}
				exit, exited, got, _ := runner.waitForeground(t.Context(), c, a, lease, port.Binding{}, port.Record{}, id, worker)
				if got != tc.want || exited != tc.exited || exit != tc.exit {
					t.Fatalf("terminal race: exit=%d observed=%v outcome=%s", exit, exited, got)
				}
				select {
				case <-lease.Done():
					t.Fatal("test failed to delay terminal notification")
				default:
				}
			})
		})
	}
}

func TestLaterHostSignalCannotReplaceTerminalFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runner, _, engine, _, id := registeredSessionFixture(t)
		provider := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}}
		lease, err := provider.Acquire(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer lease.Close()
		c, a, worker, failure := runner.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{Terminal: lease})
		if failure != "" {
			t.Fatal(failure)
		}
		defer c.Close()
		defer a.Close()
		ctx, signal := context.WithCancelCause(t.Context())
		defer signal(nil)
		engine.WaitUntilCancel = true
		engine.After = func(event string) {
			if event == "wait" {
				provider.Break()
			}
		}
		engine.AfterCancelledWait = func() { signal(port.Interrupt{Signal: "SIGTERM"}) }
		_, exited, got, _ := runner.waitForeground(ctx, c, a, lease, port.Binding{}, port.Record{}, id, worker)
		if exited || got != port.TerminalUnavailable || context.Cause(ctx) == nil {
			t.Fatalf("later host signal replaced first terminal cause: observed=%v outcome=%s cause=%v", exited, got, context.Cause(ctx))
		}
	})
}

func TestResizeReconcilesObservedExitAndHostCancellation(t *testing.T) {
	for _, fault := range []string{"successful resize races exit", "failed resize races exit", "host signal during resize"} {
		t.Run(fault, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				runner, registry, engine, _, id := registeredSessionFixture(t)
				provider := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}}
				lease, err := provider.Acquire(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				defer lease.Close()
				c, a, worker, failure := runner.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{Terminal: lease})
				if failure != "" {
					t.Fatal(failure)
				}
				defer c.Close()
				defer a.Close()
				ctx, signal := context.WithCancelCause(t.Context())
				defer signal(nil)
				release := make(chan struct{})
				engine.WaitRelease, engine.Exit = release, 17
				engine.After = func(event string) {
					if event == "wait" {
						provider.Resize(terminal.Dimensions{Columns: 167, Rows: 53})
					}
				}
				engine.BeforeResize = func() {
					close(release)
					synctest.Wait() // The authoritative wait completes during resize.
				}
				if fault == "failed resize races exit" {
					engine.ResizeFailure = port.TerminalResize
				}
				if fault == "host signal during resize" {
					registry.AfterOpen = func(string) { signal(port.Interrupt{Signal: "SIGINT"}) }
				}
				exit, exited, got, _ := runner.waitForeground(ctx, c, a, lease, port.Binding{}, port.Record{}, id, worker)
				if fault == "host signal during resize" {
					if exited || got != port.ExecutionFailed || context.Cause(ctx) == nil {
						t.Fatalf("host cancellation replaced by resize failure: %v %s", exited, got)
					}
				} else if !exited || exit != 17 || got != "" {
					t.Fatalf("resize replaced authoritative exit: %d %v %s", exit, exited, got)
				}
			})
		})
	}
}

func TestResizeRequiresFreshOwnedRunningSession(t *testing.T) {
	for _, change := range []string{"healthy", "already bounded", "invalid size", "unavailable registry", "changed session", "stopped session", "unavailable engine", "nil engine", "foreign worker", "stopped worker"} {
		t.Run(change, func(t *testing.T) {
			s, registry, engine, _, id := registeredSessionFixture(t)
			c, a, worker, f := s.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{})
			if f != "" {
				t.Fatal(f)
			}
			defer c.Close()
			defer a.Close()
			size := terminal.Dimensions{Columns: 113, Rows: 37}
			want := port.TerminalResize
			switch change {
			case "healthy":
				want = ""
			case "already bounded":
				s.Registry = finiteRegistry{s.Registry}
				s.Engine = finiteEngine{s.Engine}
				want = ""
			case "invalid size":
				size.Rows = 0
				want = port.TerminalUnavailable
			case "unavailable registry":
				registry.Failure = port.Incomplete
			case "changed session":
				registry.AfterOpen = func(string) {
					r := registry.Records[id]
					r.State = "stopped"
					registry.Records[id] = r
				}
			case "stopped session":
				r := registry.Records[id]
				r.State = "stopped"
				registry.Records[id] = r
			case "unavailable engine":
				engine.OpenFailure = port.EngineLost
			case "nil engine":
				engine.OpenNil = true
			case "foreign worker":
				r := engine.Resources[worker]
				r.Name = "foreign"
				engine.Resources[worker] = r
			case "stopped worker":
				r := engine.Resources[worker]
				r.Running = false
				engine.Resources[worker] = r
			}
			if got := s.resizeOwned(context.Background(), id, worker, size); got != want {
				t.Fatalf("resize: %s want %s", got, want)
			}
		})
	}
}

func TestResizeKeepsWaitForWorkerThatAlreadyExited(t *testing.T) {
	for _, when := range []string{"before inspection", "during resize"} {
		t.Run(when, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				runner, _, engine, _, id := registeredSessionFixture(t)
				provider := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}}
				lease, err := provider.Acquire(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				defer lease.Close()
				c, a, worker, failure := runner.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{Terminal: lease})
				if failure != "" {
					t.Fatal(failure)
				}
				defer c.Close()
				defer a.Close()
				release := make(chan struct{})
				engine.WaitRelease, engine.Exit = release, 0
				stopWorker := func() {
					o := engine.Resources[worker]
					o.Running, o.Status, o.PID = false, "exited", 0
					engine.Resources[worker] = o
				}
				engine.After = func(event string) {
					if event == "wait" {
						if when == "before inspection" {
							stopWorker()
						}
						provider.Resize(terminal.Dimensions{Columns: 167, Rows: 53})
					}
				}
				resized := false
				engine.BeforeResize = func() {
					resized = true
					stopWorker()
				}
				engine.ResizeFailure = port.TerminalResize
				go func() {
					time.Sleep(time.Second) // The wait reply trails the engine state change.
					close(release)
				}()
				exit, exited, got, _ := runner.waitForeground(t.Context(), c, a, lease, port.Binding{}, port.Record{}, id, worker)
				<-release
				if !exited || exit != 0 || got != "" || resized != (when == "during resize") {
					t.Fatalf("resize raced pending exit: exit=%d observed=%v outcome=%s resized=%v", exit, exited, got, resized)
				}
			})
		})
	}
}

func TestResizeBoundsUnresponsiveSessionAcquisition(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, registry, _, _, id := registeredSessionFixture(t)
		c, a, worker, f := s.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{})
		if f != "" {
			t.Fatal(f)
		}
		defer c.Close()
		defer a.Close()
		release := make(chan struct{})
		defer close(release)
		registry.AfterOpen = func(string) { <-release }
		result := make(chan port.Outcome, 1)
		go func() { result <- s.resizeOwned(t.Context(), id, worker, terminal.Dimensions{Columns: 113, Rows: 37}) }()
		time.Sleep(budget.CallDuration)
		synctest.Wait()
		select {
		case got := <-result:
			if got != port.CallTimeout {
				t.Fatal("session acquisition lost finite timeout", got)
			}
		default:
			t.Fatal("unresponsive session acquisition held resize past its deadline")
		}
	})
}

func TestRegisteredTerminalStaysRawUntilBoundedDrainEnds(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			runner, frozen, _, engine, _ := registeredLifecycleFixture()
			provider := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}}
			lease, err := provider.Acquire(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			rawDuringDrain := false
			engine.DrainUntilCancel = blocked
			engine.After = func(event string) {
				if event == "drain" {
					rawDuringDrain = provider.Raw()
				}
			}
			start := time.Now()
			result := runner.Run(t.Context(), frozen, []string{"probe"}, port.Streams{Terminal: lease})
			if !rawDuringDrain || provider.Raw() || result.Restore != "" || !result.Exited || result.Exit != 17 {
				t.Fatalf("terminal restored before final drain: raw=%v result=%+v", rawDuringDrain, result)
			}
			if blocked && (time.Since(start) != 10*time.Second || result.Stream != port.StreamTimeout) {
				t.Fatalf("blocked output retained raw terminal beyond drain: elapsed=%v result=%+v", time.Since(start), result)
			}
		})
	}
}

func TestTerminalRestoresWhenDrainFinishesWhileCleanupIsPending(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runner, frozen, _, engine, _ := registeredLifecycleFixture()
		provider := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}}
		lease, err := provider.Acquire(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer lease.Close()
		engine.After = func(event string) {
			switch event {
			case "drain":
				time.Sleep(time.Second)
			case "anchor remove":
				time.Sleep(8 * time.Second)
			}
		}
		restored := make(chan time.Time, 1)
		go func() { <-lease.Done(); restored <- time.Now() }()
		start := time.Now()
		result := runner.Run(t.Context(), frozen, []string{"probe"}, port.Streams{Terminal: lease})
		if at := <-restored; at.Sub(start) != time.Second || time.Since(start) != 8*time.Second || result.Cleanup != port.Complete {
			t.Fatalf("restoration waited for cleanup or preceded drain: restored=%v returned=%v result=%+v", at.Sub(start), time.Since(start), result)
		}
	})
}
