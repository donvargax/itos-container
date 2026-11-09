package domain_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	st "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestForwardingFailureCancelsPendingWorkloadWait(t *testing.T) {
	for _, streams := range []port.Streams{{In: st.BrokenStream{}}, {Out: st.BrokenStream{}}, {Err: st.BrokenStream{}}} {
		s, d, _, e := runnerFixture()
		e.WaitUntilCancel = true
		e.Output, e.ErrorOutput = []byte("out"), []byte("err")
		r := s.Run(context.Background(), d, []string{"probe"}, streams)
		if r.Outcome != port.ExecutionFailed || r.Exited || r.Cleanup != port.Complete {
			t.Fatal(r)
		}
	}
}

func TestSuccessfulEOFDoesNotCancelWorkloadWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, d, _, e := runnerFixture()
		e.WaitUntilCancel = true
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		start := time.Now()
		r := s.Run(ctx, d, []string{"probe"}, port.Streams{})
		if time.Since(start) != time.Minute || r.Exited || r.Cleanup != port.Complete {
			t.Fatal(r)
		}
	})
}

func TestInterruptForwardingAndEscalation(t *testing.T) {
	for _, signal := range []string{"SIGINT", "SIGTERM"} {
		for _, ignore := range []bool{false, true} {
			t.Run(signal+map[bool]string{true: " ignored", false: " handled"}[ignore], func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					s, d, _, e := runnerFixture()
					e.WaitUntilCancel, e.IgnoreSignal, e.Exit = true, ignore, 23
					ctx, cancel := context.WithCancelCause(context.Background())
					e.After = func(event string) {
						if event == "wait" && ctx.Err() == nil {
							cancel(port.Interrupt{Signal: signal})
						}
					}
					start := time.Now()
					r := s.Run(ctx, d, []string{"probe"}, port.Streams{})
					if r.Outcome != "" || r.Signal != signal || !r.Exited || r.Exit != 23 || r.Cleanup != port.Complete {
						t.Fatal(r, e.Events)
					}
					if !slices.Contains(e.Events, signal) || slices.Contains(e.Events, "SIGKILL") != ignore {
						t.Fatal(e.Events)
					}
					want := time.Duration(0)
					if ignore {
						want = 10 * time.Second
					}
					if time.Since(start) != want {
						t.Fatal("interrupt grace changed", time.Since(start))
					}
					drain, remove := slices.Index(e.Events, "drain"), slices.Index(e.Events, "remove")
					if drain < 0 || drain >= remove || e.DrainContextErr != nil {
						t.Fatal("interrupted exit did not drain before cleanup with a live context", e.Events, e.DrainContextErr)
					}
				})
			})
		}
	}
}

func TestForegroundStartupHasAnAggregateDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, d, _, e := runnerFixture()
		s.Engine = &startupStall{RunEngine: e}
		start := time.Now()
		r := s.Run(context.Background(), d, []string{"probe"}, port.Streams{})
		if time.Since(start) != 120*time.Second || r.Outcome != port.Incomplete || r.Exited || slices.Contains(e.Events, "create") {
			t.Fatal("startup deadline changed or allowed a launch", time.Since(start), r, e.Events)
		}
	})
}

type startupStall struct {
	port.RunEngine
	opened bool
}

func (e *startupStall) OpenRun(ctx context.Context, b port.Binding) (port.RunConnection, port.Outcome) {
	if !e.opened {
		e.opened = true
		<-ctx.Done()
		return nil, port.Incomplete
	}
	return e.RunEngine.OpenRun(ctx, b)
}

func TestUnfinishedOutputDrainIsBoundedBeforeCleanup(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			s, d, _, e := runnerFixture()
			e.DrainUntilCancel, e.Exit = true, 23
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			e.After = func(event string) {
				if interrupted && event == "wait" && ctx.Err() == nil {
					cancel(port.Interrupt{Signal: "SIGTERM"})
				}
			}
			start := time.Now()
			r := s.Run(ctx, d, []string{"probe"}, port.Streams{})
			if !r.Exited || r.Exit != 23 || r.Outcome != port.ExecutionFailed || r.Cleanup != port.Complete ||
				time.Since(start) != 10*time.Second || e.DrainContextErr != context.DeadlineExceeded {
				t.Fatal("drain lost exit, exceeded its budget or bypassed cleanup", r, time.Since(start), e.DrainContextErr)
			}
		})
	}
}

func TestClosedWorkerStdinReconcilesOnlyAnObservedExit(t *testing.T) {
	for _, fault := range []string{
		"", "running", "unavailable", "foreign ID", "foreign name", "missing state", "created", "wait failure", "canceled", "wait stalls",
		"record", "snapshot", "engine", "record after open", "daemon after open", "unowned record", "different recorded ID", "shared budget",
	} {
		t.Run(fault, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s, d, store, e := runnerFixture()
				e.StdinClosed, e.WaitUntilCancel, e.Exit = true, true, 23
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				waits := 0
				e.After = func(event string) {
					if event != "wait" {
						return
					}
					waits++
					if waits == 2 && (fault == "wait stalls" || fault == "shared budget") {
						o := e.Resources[e.CreateID]
						o.Running = true
						e.Resources[e.CreateID] = o
					}
					if waits != 1 {
						return
					}
					switch fault {
					case "canceled":
						cancel()
					case "record":
						store.UnsafeRecord = true
					case "snapshot":
						store.UnsafeSnapshot = true
					case "engine":
						e.Unavailable = true
					case "record after open":
						e.AfterOpen = func() { store.UnsafeRecord = true }
					case "daemon after open":
						e.AfterOpen = func() { e.Replaced = true }
					case "unowned record", "different recorded ID":
						for ref, data := range store.Records {
							r, _ := store.Codec.Decode(data)
							if fault == "unowned record" {
								r.Stage = "absent"
							} else {
								r.ContainerID = strings.Repeat("e", 64)
							}
							store.Records[ref] = store.Codec.Encode(r)
						}
					case "shared budget":
						e.AfterOpen = func() {
							time.Sleep(4 * time.Second)
							e.AfterOpen = nil
						}
					}
					// Wait remains pending until the input-closure notification.
					// Only its subsequent independent inspection observes exit.
					e.BeforeInspect = func() {
						if fault == "shared budget" {
							time.Sleep(3 * time.Second)
							e.BeforeInspect = nil
						}
						o, exists := e.Resources[e.CreateID]
						if !exists {
							return
						}
						o.Running, o.Status = false, "exited"
						switch fault {
						case "running":
							o.Running = true
						case "unavailable":
							e.Inspection = "unavailable"
						case "foreign ID":
							e.Inspection = "different full container ID"
						case "foreign name":
							e.Inspection = "different name"
						case "missing state":
							o.StatePresent = false
						case "created":
							o.Status = "created"
						case "wait failure":
							e.WaitFailure = port.ExecutionFailed
						}
						e.Resources[e.CreateID] = o
					}
				}
				start := time.Now()
				r := s.Run(ctx, d, []string{"probe"}, port.Streams{})
				if fault == "" {
					if !r.Exited || r.Exit != 23 || r.Outcome != "" || r.Cleanup != port.Complete || waits != 2 {
						t.Fatal("closed stdin lost the confirmed worker exit", r, e.Events)
					}
				} else if r.Exited || r.Outcome != port.ExecutionFailed || slices.Contains(e.Events, "drain") {
					t.Fatal("unverified exit became successful", r, e.Events)
				}
				if (fault == "wait stalls" || fault == "shared budget") && time.Since(start) != 10*time.Second {
					t.Fatal("exit reconciliation was not bounded", time.Since(start))
				}
			})
		})
	}
}

func TestInterruptNeverSignalsForeignOrUnverifiableOwnership(t *testing.T) {
	for _, fault := range []string{"record", "engine", "container", "write", "signal", "non-running", "intent", "snapshot", "record after open", "daemon after open", "record after sync", "daemon after sync"} {
		t.Run(fault, func(t *testing.T) {
			s, d, store, e := runnerFixture()
			ctx, cancel := context.WithCancelCause(context.Background())
			e.After = func(event string) {
				if event != "wait" || ctx.Err() != nil {
					return
				}
				cancel(port.Interrupt{Signal: "SIGTERM"})
				switch fault {
				case "record":
					store.UnsafeRecord = true
				case "engine":
					e.Unavailable = true
				case "container":
					e.Inspection = "different full container ID"
				case "write":
					store.Fault = "owned-record file fsync"
				case "signal":
					e.SignalFailure = port.ExecutionFailed
				case "non-running":
					e.BeforeInspect = func() { o := e.Resources[e.CreateID]; o.Running = false; e.Resources[e.CreateID] = o }
				case "snapshot":
					store.UnsafeSnapshot = true
				case "record after open":
					e.AfterOpen = func() { store.UnsafeRecord = true }
				case "daemon after open":
					e.AfterOpen = func() { e.Replaced = true }
				case "record after sync":
					store.AfterSave = func() { store.UnsafeRecord = true }
				case "daemon after sync":
					store.AfterSave = func() { e.Replaced = true }
				case "intent":
					for ref, data := range store.Records {
						r, _ := store.Codec.Decode(data)
						r.Stage = "intent"
						r.ContainerID = ""
						store.Records[ref] = store.Codec.Encode(r)
					}
				}
			}
			r := s.Run(ctx, d, []string{"probe"}, port.Streams{})
			if r.Outcome != "" || r.Signal != "SIGTERM" || (fault != "signal" && slices.Contains(e.Events, "SIGTERM")) {
				t.Fatal(r, e.Events)
			}
			// An interruption that could not be authorized or delivered observed no
			// exit, so it reports none rather than a status of its own.
			if fault != "non-running" && (r.Exited || r.Exit != 0) {
				t.Fatal("refused interruption reported an exit", r.Exited, r.Exit)
			}
		})
	}
}

func TestEscalationRevalidatesAfterGrace(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, d, _, e := runnerFixture()
		e.WaitUntilCancel, e.IgnoreSignal = true, true
		ctx, cancel := context.WithCancelCause(context.Background())
		e.After = func(event string) {
			if event == "wait" && ctx.Err() == nil {
				cancel(port.Interrupt{Signal: "SIGINT"})
			}
			if event == "SIGINT" {
				e.Inspection = "different full container ID"
			}
		}
		r := s.Run(ctx, d, []string{"probe"}, port.Streams{})
		// The escalation that revalidation refused observed no exit, so the run
		// reports none rather than a status of its own.
		if r.Exited || r.Exit != 0 || r.Cleanup == port.Complete || slices.Contains(e.Events, "SIGKILL") || slices.Contains(e.Events, "remove") {
			t.Fatal(r, e.Events)
		}
	})
}
