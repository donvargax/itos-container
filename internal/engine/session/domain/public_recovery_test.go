package domain_test

import (
	"context"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func publicRecoveryFixture(t *testing.T) (domain.Recovery, *f.Registry, *f.RunEngine, string) {
	t.Helper()
	b, store, engine := f.Fixture()
	id, token, worker := strings.Repeat("a", 64), strings.Repeat("1", 64), strings.Repeat("2", 64)
	i, err := store.New(context.Background(), ap.Durable{Frozen: b.Frozen}, token)
	if err != "" {
		t.Fatal(err)
	}
	r := i.Record()
	r.Stage, r.ContainerID = "owned", worker
	if err = i.Save(r); err != "" {
		t.Fatal(err)
	}
	i.Close()
	registry := &f.Registry{Store: store, Project: b.Frozen.Project, Records: map[string]port.PublicRecord{id: {Schema: 1, ID: id, Project: b.Frozen.Project, Intent: port.Reference(b.Frozen.Project, token), State: "running"}}, Leases: map[string]bool{}}
	engine.Resources = map[string]port.Observation{worker: {ID: worker, Name: r.Name, ImageID: r.ImageID, Token: token, Project: r.Project, Fingerprint: r.Fingerprint, StatePresent: true, Status: "running", Running: true, Started: true, PID: 123}}
	run := &f.RunEngine{Engine: engine, Exit: 7}
	return domain.Recovery{Registry: registry, Engine: run}, registry, run, id
}
func TestPublicStatusKeepsUnknownExitAndNeverMutates(t *testing.T) {
	for _, state := range []string{"created", "running", "exited", "absent", "unavailable", "malformed", "foreign", "stopped", "contradictory"} {
		t.Run(state, func(t *testing.T) {
			s, r, e, id := publicRecoveryFixture(t)
			worker := strings.Repeat("2", 64)
			observation := e.Resources[worker]
			record := r.Records[id]
			switch state {
			case "created":
				observation.Status, observation.Running, observation.Started, observation.PID = "created", false, false, 0
			case "exited":
				observation.Status, observation.Running, observation.PID = "exited", false, 0
			case "absent":
				delete(e.Resources, worker)
			case "unavailable":
				e.Unavailable = true
			case "malformed":
				observation.StatePresent = false
			case "foreign":
				observation.ImageID = "foreign"
			case "stopped":
				record.State = "stopped"
				r.Records[id] = record
				i, err := r.Store.Open(context.Background(), record.Intent)
				if err != "" {
					t.Fatal(err)
				}
				owned := i.Record()
				owned.Stage = "absent"
				if err = i.Save(owned); err != "" {
					t.Fatal(err)
				}
				i.Close()
				delete(e.Resources, worker)
			case "contradictory":
				observation.Status = "exited"
			}
			if state != "absent" && state != "stopped" {
				e.Resources[worker] = observation
			}
			before := len(r.Store.Events)
			result := s.Status(context.Background())
			if len(result.Entries) != 1 || result.Entries[0].WorkloadExit != nil || r.Writes != 0 || r.Syncs != 0 || len(r.Store.Events) != before {
				t.Fatal("status fabricated exit or wrote evidence", result)
			}
			entry := result.Entries[0]
			switch state {
			case "unavailable", "malformed", "foreign", "contradictory":
				if len(result.Failures) != 1 || entry.Resource != "unknown" || entry.Cleanup != "unknown" || entry.State != "failed" {
					t.Fatal(result)
				}
			default:
				if len(result.Failures) != 0 {
					t.Fatal(result)
				}
			}
			if state == "stopped" {
				if entry.Retry != nil || entry.StopCandidate || entry.Cleanup != "complete" {
					t.Fatal(entry)
				}
			} else if !entry.StopCandidate || !reflect.DeepEqual(entry.Retry, []string{"itos", "container", "stop", id}) {
				t.Fatal(entry)
			}
			for _, event := range e.Engine.Events {
				if event.Method == "REMOVE" || event.Method == "CREATE" {
					t.Fatal("status mutated engine")
				}
			}
		})
	}
}

func TestPublicStatusObservesRunningAfterInterruptedLaunch(t *testing.T) {
	for _, tc := range []struct{ phase, resource, want string }{
		{"preparing", "created", "preparing"},
		{"ready", "created", "ready"},
		{"preparing", "running", "running"},
		{"ready", "running", "running"},
		{"running", "running", "running"},
		{"stopping", "running", "stopping"},
		{"failed", "running", "failed"},
	} {
		t.Run(tc.phase+"/"+tc.resource, func(t *testing.T) {
			s, registry, engine, id := publicRecoveryFixture(t)
			record := registry.Records[id]
			record.State = tc.phase
			registry.Records[id] = record
			if tc.resource == "created" {
				worker := strings.Repeat("2", 64)
				o := engine.Resources[worker]
				o.Status, o.Running, o.Started, o.PID = "created", false, false, 0
				engine.Resources[worker] = o
			}
			before := len(registry.Store.Events)
			result := s.Status(context.Background())
			if len(result.Failures) != 0 || len(result.Entries) != 1 {
				t.Fatal(result)
			}
			entry := result.Entries[0]
			if entry.State != tc.want || entry.Resource != tc.resource || entry.Controller != "absent" || entry.Cleanup != "pending" || entry.WorkloadExit != nil || !entry.StopCandidate || !reflect.DeepEqual(entry.Retry, []string{"itos", "container", "stop", id}) {
				t.Fatal("lost controller's worker was not reported accurately", entry)
			}
			if registry.Records[id] != record || registry.Writes != 0 || registry.Syncs != 0 || len(registry.Store.Events) != before || !reflect.DeepEqual(engine.Events, []string{"open"}) {
				t.Fatal("status changed recorded progress or worker state")
			}
		})
	}
}

func TestPublicStopPreservesResultAndReloadsCompletion(t *testing.T) {
	for _, known := range []bool{false, true} {
		t.Run(map[bool]string{false: "unknown", true: "known"}[known], func(t *testing.T) {
			s, r, e, id := publicRecoveryFixture(t)
			if known {
				record := r.Records[id]
				n := 75
				record.WorkloadExit = &n
				r.Records[id] = record
			}
			first := s.Stop(context.Background(), id)
			if len(first.Failures) != 0 || len(first.Entries) != 1 || first.Entries[0].Cleanup != "complete" {
				t.Fatal(first)
			}
			want := 7
			if known {
				want = 75
			}
			if first.Entries[0].WorkloadExit == nil || *first.Entries[0].WorkloadExit != want {
				t.Fatal(first)
			}
			writes := r.Writes
			mutations := 0
			for _, event := range e.Engine.Events {
				if event.Method == "REMOVE" {
					mutations++
				}
			}
			if mutations != 1 {
				t.Fatal("wrong removal count", mutations)
			}
			second := s.Stop(context.Background(), id)
			if len(second.Failures) != 0 || r.Writes != writes || r.Syncs == 0 {
				t.Fatal("repeated completion rewrote state", second)
			}
			mutations = 0
			for _, event := range e.Engine.Events {
				if event.Method == "REMOVE" {
					mutations++
				}
			}
			if mutations != 1 {
				t.Fatal("repeated stop mutated engine")
			}
		})
	}
}

func TestPublicStopUsesWorkerIdentityInsteadOfObservedPID(t *testing.T) {
	for _, pid := range []int{-1, 0, 123} {
		for _, ignoreTERM := range []bool{false, true} {
			t.Run(strconv.Itoa(pid)+"/ignoreTERM="+strconv.FormatBool(ignoreTERM), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					s, registry, engine, id := publicRecoveryFixture(t)
					worker, other := strings.Repeat("2", 64), strings.Repeat("f", 64)
					o := engine.Resources[worker]
					o.PID = pid
					engine.Resources[worker] = o
					foreign := o
					foreign.ID, foreign.Name = other, "unrelated-worker"
					engine.Resources[other] = foreign
					registry.Leases[id] = true
					engine.IgnoreSignal, engine.WaitUntilCancel = ignoreTERM, true

					result := s.Stop(context.Background(), id)
					if len(result.Failures) != 0 || len(result.Entries) != 1 || result.Entries[0].Cleanup != "complete" || result.Entries[0].Controller != "present" {
						t.Fatal("explicit stop did not finish owned cleanup with a live controller", result)
					}
					if !registry.Leases[id] || !reflect.DeepEqual(engine.Resources, map[string]port.Observation{other: foreign}) {
						t.Fatal("stop changed the controller or a resource other than its owned worker", engine.Resources)
					}
					want := []string{"open", "SIGTERM", "wait"}
					if ignoreTERM {
						want = append(want, "SIGKILL", "wait")
					}
					if !reflect.DeepEqual(engine.Events, want) {
						t.Fatal("stop did not use the explicit engine-side termination sequence", engine.Events)
					}
				})
			})
		}
	}
}

func TestPublicRecoveryRefusesFaultsWithoutMutation(t *testing.T) {
	for _, fault := range []port.Outcome{port.Refused, port.Unsafe, port.Invalid, port.Incomplete, port.Environment, port.Internal} {
		t.Run(string(fault), func(t *testing.T) {
			s, r, e, id := publicRecoveryFixture(t)
			r.Failure = fault
			for _, result := range []port.RecoveryResult{s.Status(context.Background()), s.Stop(context.Background(), id)} {
				if len(result.Failures) != 1 || result.Failures[0].Outcome != fault {
					t.Fatal(result)
				}
			}
			if len(e.Events) != 0 || r.Writes != 0 {
				t.Fatal("refused state reached engine")
			}
		})
	}
}
func TestPublicIntentOnlyStopRemainsUncertain(t *testing.T) {
	s, r, e, id := publicRecoveryFixture(t)
	record := r.Records[id]
	reference := record.Intent
	original := r.Store.Records[reference]
	i, err := r.Store.Codec.Decode(original)
	if err != "" {
		t.Fatal(err)
	}
	i.Stage, i.ContainerID = "intent", ""
	r.Store.Records[reference] = r.Store.Codec.Encode(i)
	e.Resources = map[string]port.Observation{}
	result := s.Stop(context.Background(), id)
	if len(result.Failures) != 1 || result.Failures[0].Outcome != port.Uncertain || result.Entries[0].Cleanup != "unknown" || r.Records[id].State != "failed" {
		t.Fatal(result)
	}
	i, err = r.Store.Codec.Decode(r.Store.Records[reference])
	if err != "" || i.ContainerID != "" || i.Stage != "intent" {
		t.Fatal("uncertain create acquired invented absence", i)
	}
}
func TestPublicStopRevalidatesBeforeSignalAndRemove(t *testing.T) {
	for _, boundary := range []string{"SIGTERM", "SIGKILL", "remove"} {
		t.Run(boundary, func(t *testing.T) {
			s, r, e, id := publicRecoveryFixture(t)
			worker := strings.Repeat("2", 64)
			if boundary == "SIGTERM" {
				r.Store.AfterSave = func() { o := e.Resources[worker]; o.ImageID = "foreign"; e.Resources[worker] = o }
			}
			if boundary == "remove" {
				e.After = func(event string) {
					if event == "wait" {
						o := e.Resources[worker]
						o.ImageID = "foreign"
						e.Resources[worker] = o
					}
				}
			}
			if boundary == "SIGKILL" {
				e.WaitFailure = port.ExecutionFailed
				e.After = func(event string) {
					if event == "wait" {
						o := e.Resources[worker]
						o.ImageID = "foreign"
						e.Resources[worker] = o
					}
				}
			}
			result := s.Stop(context.Background(), id)
			if len(result.Failures) != 1 || result.Failures[0].Outcome != port.Refused {
				t.Fatal(result)
			}
			for _, event := range e.Events {
				if event == boundary {
					t.Fatal("barred mutation reached", boundary)
				}
			}
			for _, event := range e.Engine.Events {
				if event.Method == "REMOVE" {
					t.Fatal("foreign resource removed")
				}
			}
		})
	}
}
func TestPublicStopGraceUsesOriginalBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, _, e, id := publicRecoveryFixture(t)
		e.IgnoreSignal, e.WaitUntilCancel = true, true
		start := time.Now()
		result := s.Stop(context.Background(), id)
		if len(result.Failures) != 0 || time.Since(start) != 10*time.Second {
			t.Fatal("grace budget", result, time.Since(start))
		}
		if !strings.Contains(strings.Join(e.Events, ","), "SIGTERM,wait,SIGKILL,wait") {
			t.Fatal(e.Events)
		}
	})
}

func TestPublicRecoveryLockUsesTheWholeInvocationBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, r, e, id := publicRecoveryFixture(t)
		r.Locks = map[string]bool{id: true}
		r.WaitForLock = true
		start := time.Now()
		status := s.Status(context.Background())
		if time.Since(start) != 10*time.Second || len(status.Failures) != 1 || status.Failures[0].Outcome != port.Incomplete {
			t.Fatal(status, time.Since(start))
		}
		start = time.Now()
		stop := s.Stop(context.Background(), id)
		if time.Since(start) != 30*time.Second || len(stop.Failures) != 1 || stop.Failures[0].Outcome != port.Incomplete {
			t.Fatal(stop, time.Since(start))
		}
		if len(e.Events) != 0 || r.Writes != 0 || r.Syncs != 0 {
			t.Fatal("contended recovery mutated state")
		}
	})
}

func TestRegisteredRunPersistsObservedExitAndActionableCleanup(t *testing.T) {
	for _, cleanupFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "retry"}[cleanupFailure], func(t *testing.T) {
			b, store, engine := f.Fixture()
			registry := &f.Registry{Store: store, Project: b.Frozen.Project}
			runEngine := &f.RunEngine{Engine: engine, Exit: 7}
			if cleanupFailure {
				engine.Following = "still present"
			}
			runner := domain.Runner{Store: store, Registry: registry, Engine: runEngine, Entropy: f.Entropy{Value: strings.Repeat("1", 64)}, Workspace: &f.Workspace{}}
			result := runner.Run(context.Background(), ap.Durable{Frozen: b.Frozen}, []string{"probe"}, port.Streams{})
			if result.SessionID == "" || !result.Exited || result.Exit != 7 || registry.Leases[result.SessionID] {
				t.Fatal(result)
			}
			r := registry.Records[result.SessionID]
			if r.WorkloadExit == nil || *r.WorkloadExit != 7 {
				t.Fatal("lost observed result", r)
			}
			if cleanupFailure {
				if result.Cleanup == port.Complete || r.State != "failed" {
					t.Fatal(result, r)
				}
			} else if result.Cleanup != port.Complete || r.State != "stopped" {
				t.Fatal(result, r)
			}
		})
	}
}

func TestRegisteredWaitReconcilesConcurrentStop(t *testing.T) {
	for _, name := range []string{"unknown", "zero", "nonzero", "drain failure", "nonterminal intent", "different worker"} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				b, store, engine := f.Fixture()
				registry := &f.Registry{Store: store, Project: b.Frozen.Project}
				stdin := make(chan port.Outcome, 1)
				e := &f.RunEngine{Engine: engine, StdinFailures: stdin, WaitUntilCancel: true, WaitFailure: port.ExecutionFailed}
				known := name == "zero" || name == "nonzero"
				if known {
					e.WaitFailure = ""
				}
				if name == "nonzero" {
					e.Exit = 7
				}
				if name == "drain failure" {
					e.DrainFailure = port.ExecutionFailed
				}
				stopped := false
				e.After = func(event string) {
					if event != "wait" || stopped {
						return
					}
					stopped = true
					for id, record := range registry.Records {
						result := (domain.Recovery{Registry: registry, Engine: e}).Stop(context.Background(), id)
						if len(result.Failures) != 0 || result.Entries[0].Cleanup != "complete" {
							t.Fatal(result)
						}
						if name == "nonterminal intent" || name == "different worker" {
							i, failure := store.Codec.Decode(store.Records[record.Intent])
							if failure != "" {
								t.Fatal(failure)
							}
							if name == "nonterminal intent" {
								i.Stage = "owned"
							} else {
								i.ContainerID = strings.Repeat("f", 64)
							}
							store.Records[record.Intent] = store.Codec.Encode(i)
						}
					}
					// Reconciliation observes completed stop, including the selected
					// record fault, only after the pending input actually closes.
					stdin <- port.StdinClosed
					synctest.Wait()
				}
				runner := domain.Runner{Store: store, Registry: registry, Engine: e, Entropy: f.Entropy{Value: strings.Repeat("1", 64)}, Workspace: &f.Workspace{}}
				result := runner.Run(context.Background(), ap.Durable{Frozen: b.Frozen}, []string{"probe"}, port.Streams{})
				want := port.Outcome("")
				if name == "drain failure" || name == "nonterminal intent" || name == "different worker" {
					want = port.ExecutionFailed
				}
				if !stopped || (result.Outcome != want && result.Stream != want) || result.Exited != known || result.Exit != e.Exit {
					t.Fatal("concurrent completion lost exit knowledge or failure", result)
				}
				if (registry.Records[result.SessionID].WorkloadExit != nil) != known {
					t.Fatal("reconciliation fabricated or lost workload exit")
				}
				if want == "" && (result.Cleanup != port.Complete || !slices.Contains(e.Events, "drain")) {
					t.Fatal("completed reconciliation skipped drain or cleanup", result, e.Events)
				}
			})
		})
	}
}

func TestRegisteredWaitDoesNotExtendTeardownForUnavailableCompletion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b, store, engine := f.Fixture()
		registry := &f.Registry{Store: store, Project: b.Frozen.Project, WaitForLock: true}
		stdin := make(chan port.Outcome, 1)
		e := &f.RunEngine{Engine: engine, StdinFailures: stdin, WaitUntilCancel: true}
		e.After = func(event string) {
			if event == "wait" {
				for id := range registry.Records {
					// Keep the competing operation unavailable through both
					// the stdin probe and the complete teardown budget.
					registry.Locks[id] = true
				}
				stdin <- port.StdinClosed
				synctest.Wait()
			}
		}
		runner := domain.Runner{Store: store, Registry: registry, Engine: e, Entropy: f.Entropy{Value: strings.Repeat("1", 64)}, Workspace: &f.Workspace{}}
		start := time.Now()
		result := runner.Run(context.Background(), ap.Durable{Frozen: b.Frozen}, []string{"probe"}, port.Streams{})
		if time.Since(start) != 39*time.Second || time.Until(result.Deadline) != time.Second || result.Outcome != port.ExecutionFailed || result.Teardown != port.TeardownTimeout || result.Cleanup != port.Incomplete || result.Exited {
			t.Fatal("unavailable completion extended teardown or erased failure", result, time.Since(start))
		}
		if len(engine.Resources) != 1 || registry.Records[result.SessionID].State != "running" || registry.Leases[result.SessionID] || slices.Contains(e.Events, "SIGTERM") {
			t.Fatal("unavailable ownership authorized completion or mutation", e.Events)
		}
	})
}

func TestRegisteredRunStartupDeadlineCleansCreatedWorker(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b, store, engine := f.Fixture()
		registry := &f.Registry{Store: store, Project: b.Frozen.Project}
		runEngine := &f.RunEngine{Engine: engine}
		registry.AfterClose = func(id string) {
			if registry.Records[id].State == "ready" {
				registry.AfterClose = nil
				// Expire run's actual startup budget after releasing the create
				// lock, before it can reopen the session to start the worker.
				time.Sleep(120 * time.Second)
				synctest.Wait()
			}
		}
		runner := domain.Runner{Store: store, Registry: registry, Engine: runEngine, Entropy: f.Entropy{Value: strings.Repeat("1", 64)}, Workspace: &f.Workspace{}}
		start := time.Now()
		result := runner.Run(context.Background(), ap.Durable{Frozen: b.Frozen}, []string{"probe"}, port.Streams{})
		if time.Since(start) != 120*time.Second || result.Outcome != port.StartupTimeout || result.Exited || result.Cleanup != port.Complete || len(engine.Resources) != 0 {
			t.Fatal("startup deadline skipped cleanup or invented a workload result", result, time.Since(start))
		}
		record := registry.Records[result.SessionID]
		if record.State != "stopped" || record.WorkloadExit != nil || registry.Leases[result.SessionID] {
			t.Fatal("cleanup lost completion, unknown exit or released lease", record)
		}
		for _, event := range runEngine.Events {
			if event == "start" {
				t.Fatal("expired launch started its worker")
			}
		}
	})
}

func TestRegisteredRunAdoptsExitObservedAfterPartialCleanup(t *testing.T) {
	b, store, engine := f.Fixture()
	registry := &f.Registry{Store: store, Project: b.Frozen.Project}
	e := &f.RunEngine{Engine: engine, WaitFailure: port.ExecutionFailed}
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	e.After = func(event string) {
		if event == "wait" {
			cancel(port.Interrupt{Signal: "SIGTERM"})
		}
	}
	published := false
	registry.AfterClose = func(id string) {
		record := registry.Records[id]
		if record.State != "stopping" || published {
			return
		}
		// Release of the partial-cleanup lock permits a late observer to
		// publish an explicit exit. This independently supplied observation
		// is unknown to run until its final cleanup reopens the record.
		published = true
		exit := 7
		record.WorkloadExit = &exit
		registry.Records[id] = record
	}
	runner := domain.Runner{Store: store, Registry: registry, Engine: e, Entropy: f.Entropy{Value: strings.Repeat("1", 64)}, Workspace: &f.Workspace{}}
	result := runner.Run(ctx, ap.Durable{Frozen: b.Frozen}, []string{"probe"}, port.Streams{})
	if !published || !result.Exited || result.Exit != 7 || result.Cleanup != port.Complete || result.Signal != "SIGTERM" {
		t.Fatal("run lost the exit observed during cleanup", result)
	}
	if len(engine.Resources) != 0 || registry.Leases[result.SessionID] {
		t.Fatal("completed run retained its worker or lease")
	}
}

func TestRegisteredRunRegistrationAndLateLaunchRefusals(t *testing.T) {
	for _, fault := range []port.Outcome{port.Refused, port.Unsafe, port.Invalid, port.Internal, port.Incomplete} {
		t.Run(string(fault), func(t *testing.T) {
			b, store, engine := f.Fixture()
			registry := &f.Registry{Store: store, Project: b.Frozen.Project, Failure: fault}
			runEngine := &f.RunEngine{Engine: engine}
			runner := domain.Runner{Store: store, Registry: registry, Engine: runEngine, Entropy: f.Entropy{Value: strings.Repeat("1", 64)}, Workspace: &f.Workspace{}}
			result := runner.Run(context.Background(), ap.Durable{Frozen: b.Frozen}, []string{"probe"}, port.Streams{})
			if result.SessionID != "" || result.Reference != "" || !strings.HasPrefix(string(result.Outcome), "session-registration-") || len(engine.Resources) != 0 {
				t.Fatal(result)
			}
		})
	}
	for _, boundary := range []string{"pre-create", "ready"} {
		t.Run(boundary, func(t *testing.T) {
			b, store, engine := f.Fixture()
			registry := &f.Registry{Store: store, Project: b.Frozen.Project}
			runEngine := &f.RunEngine{Engine: engine}
			runner := domain.Runner{Store: store, Registry: registry, Engine: runEngine, Entropy: f.Entropy{Value: strings.Repeat("1", 64)}, Workspace: &f.Workspace{}}
			if boundary == "pre-create" {
				registry.AfterRegister = func() {
					for id, r := range registry.Records {
						r.State = "failed"
						registry.Records[id] = r
					}
				}
			} else {
				registry.AfterClose = func(id string) {
					if registry.Records[id].State == "ready" {
						registry.AfterClose = nil
						result := (domain.Recovery{Registry: registry, Engine: runEngine}).Stop(context.Background(), id)
						if len(result.Failures) != 0 {
							t.Fatal(result)
						}
					}
				}
			}
			result := runner.Run(context.Background(), ap.Durable{Frozen: b.Frozen}, []string{"probe"}, port.Streams{})
			if result.Exited {
				t.Fatal("invented successful workload", result)
			}
			for _, event := range runEngine.Events {
				if event == "start" {
					t.Fatal("late start after stop")
				}
			}
		})
	}
}

// Slice-12 supersedes serial drain-before-removal: cleanup must progress while
// output is blocked. The deadline and authoritative exit remain independent.
func TestRegisteredRunCleansWhileDrainIsPending(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b, store, engine := f.Fixture()
		registry := &f.Registry{Store: store, Project: b.Frozen.Project}
		e := &f.RunEngine{Engine: engine, Exit: 7, DrainUntilCancel: true}
		checked := make(chan struct{})
		e.After = func(event string) {
			if event == "drain" {
				<-checked
			}
		}
		runner := domain.Runner{Store: store, Registry: registry, Engine: e, Entropy: f.Entropy{Value: strings.Repeat("1", 64)}, Workspace: &f.Workspace{}}
		done := make(chan port.RunResult, 1)
		go func() {
			done <- runner.Run(context.Background(), ap.Durable{Frozen: b.Frozen}, []string{"probe"}, port.Streams{})
		}()
		synctest.Wait()
		if len(engine.Resources) != 0 {
			t.Fatal("blocked drain prevented owned removal")
		}
		for id := range registry.Records {
			entry, failure := registry.Open(context.Background(), id, true)
			if failure != "" {
				t.Fatal("drain retained operation lock", failure)
			}
			entry.Close()
		}
		close(checked)
		time.Sleep(10 * time.Second)
		r := <-done
		if !r.Exited || r.Exit != 7 || r.Cleanup != port.Complete || r.Stream != port.StreamTimeout {
			t.Fatal(r)
		}
	})
}
