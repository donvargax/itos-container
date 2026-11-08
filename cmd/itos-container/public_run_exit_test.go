package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/cli"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/presentation"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
	runfake "github.com/donvargax/itos-container/internal/slices/run/port/porttest"
	status "github.com/donvargax/itos-container/internal/slices/status/command"
	stop "github.com/donvargax/itos-container/internal/slices/stop/command"
)

type runExitCommand struct {
	Exit           int
	Stdout, Stderr string
}

type runExitObservation struct {
	Run, Stop, Repeated, Status                     runExitCommand
	CompletedRecord, FinalRecord                    string
	CompletedIntent, FinalIntent                    string
	MutationsAtCompletion, MutationsAfter           int
	Events                                          []string
	ProbeSeconds, CompletionSeconds, ElapsedSeconds int
	DrainBudgetSeconds                              int
}

// Emit command observations, not a PASS-line oracle. The feature's Then steps
// judge them. Every case starts with empty registration/intent/engine storage;
// only the production commands create and complete the session.
func TestPublicRunExitContract(t *testing.T) {
	schedule := os.Getenv("SLICE_24_SCHEDULE")
	if schedule == "" {
		t.Skip("run-exit feature subprocess only")
	}
	if schedule != "before stdin reconciliation" && schedule != "after the stdin reconciliation deadline" {
		t.Fatal("unknown run-exit schedule")
	}
	results := map[string]runExitObservation{}
	for _, name := range []string{"unknown", "zero", "seven", "seventy-five", "unknown-drain", "seven-drain"} {
		synctest.Test(t, func(t *testing.T) {
			results[name] = observeRunExit(t, schedule, name, nil)
		})
	}
	data, err := json.Marshal(results)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("SLICE_24_RESULT=%s\n", data)
}

func TestRunExitCancellationJournalOrdering(t *testing.T) {
	for _, schedule := range []string{"before stdin reconciliation", "after the stdin reconciliation deadline"} {
		t.Run(schedule, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// The finite wait wrapper may return before the canceled adapter
				// goroutine runs again. Force that scheduling gap in fake time.
				r := observeRunExit(t, schedule, "unknown", func() { time.Sleep(time.Millisecond) })
				time.Sleep(2 * time.Millisecond)
				synctest.Wait()
				previous := -1
				for _, event := range []string{"stdin-closed", "run-wait-canceled", "probe-start", "drain", "run-returned"} {
					index := slices.Index(r.Events, event)
					if index <= previous {
						t.Fatalf("missing or out-of-order %q in %v", event, r.Events)
					}
					previous = index
				}
			})
		})
	}
}

// The journal also makes the lock/probe ordering observable; elapsed time alone
// cannot distinguish the retry path from an unrelated eleven-second delay.
type runExitJournal struct {
	mu     sync.Mutex
	events []string
}

func (j *runExitJournal) add(event string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.events = append(j.events, event)
}

type runExitStorage struct {
	ap.RegistryStorage
	journal       *runExitJournal
	mu            sync.Mutex
	probes        int
	result        *runExitObservation
	waitCanceled  <-chan struct{}
	probeObserved chan struct{}
}

func (s *runExitStorage) Open(ctx context.Context, c ap.Candidate, id string, readOnly bool) (ap.SessionDirectory, ap.Failure) {
	if !readOnly {
		return s.RegistryStorage.Open(ctx, c, id, readOnly)
	}
	s.mu.Lock()
	s.probes++
	probe := s.probes
	if probe == 1 {
		s.probeObserved = make(chan struct{})
	}
	probeObserved := s.probeObserved
	s.mu.Unlock()
	if probe > 2 {
		return s.RegistryStorage.Open(ctx, c, id, readOnly)
	}
	if probe == 1 {
		defer close(probeObserved)
	} else {
		// The finite registry wrapper can start reconciliation before the
		// expired probe's adapter goroutine has journaled its return. Await
		// that observation before starting the fixture's next checkpoint.
		select {
		case <-probeObserved:
		case <-ctx.Done():
			return nil, "state.unavailable"
		}
	}
	// The finite engine wrapper can return on cancellation before its inner
	// Wait goroutine records that cancellation. Hold this fixture checkpoint
	// until that observation is published, so the intended schedule does not
	// depend on which goroutine the scheduler wakes first.
	select {
	case <-s.waitCanceled:
	case <-ctx.Done():
		return nil, "state.unavailable"
	}
	name := "probe"
	if probe == 2 {
		name = "reconcile"
	}
	s.journal.add(name + "-start")
	start := time.Now()
	d, f := s.RegistryStorage.Open(ctx, c, id, readOnly)
	if ctx.Err() == context.DeadlineExceeded {
		s.result.ProbeSeconds = int(time.Since(start) / time.Second)
		s.journal.add(name + "-expired")
	} else {
		s.journal.add(name + "-acquired")
	}
	return d, f
}

func TestRunExitProbeWaitsForCancellationObservation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := publicMemoryFixture(t)
		id, _ := m.seed("a")
		ack := make(chan struct{})
		j := &runExitJournal{}
		var result runExitObservation
		s := &runExitStorage{RegistryStorage: m.files, journal: j, result: &result, waitCanceled: ack}
		done := make(chan struct{})
		go func() {
			d, failure := s.Open(context.Background(), m.c, id, true)
			if failure != "" {
				t.Error(failure)
			}
			if d != nil {
				d.Close()
			}
			close(done)
		}()
		synctest.Wait()
		if len(j.events) != 0 {
			t.Fatalf("probe ran before wait observation: %v", j.events)
		}
		j.add("run-wait-canceled")
		close(ack)
		<-done
		if fmt.Sprint(j.events) != "[run-wait-canceled probe-start probe-acquired]" {
			t.Fatal(j.events)
		}
	})
}

// Hold the expired adapter call after its caller can already have timed out.
// This makes the finite wrapper's late-result schedule deterministic.
type runExitDelayedStorage struct {
	ap.RegistryStorage
	release <-chan struct{}
}

func (s runExitDelayedStorage) Open(ctx context.Context, c ap.Candidate, id string, readOnly bool) (ap.SessionDirectory, ap.Failure) {
	<-s.release
	return s.RegistryStorage.Open(ctx, c, id, readOnly)
}

func TestRunExitReconciliationWaitsForProbeObservation(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(fmt.Sprintf("canceled=%t", canceled), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := publicMemoryFixture(t)
				id, _ := m.seed("a")
				ack, release := make(chan struct{}), make(chan struct{})
				close(ack)
				journal := &runExitJournal{}
				var result runExitObservation
				storage := &runExitStorage{
					RegistryStorage: runExitDelayedStorage{RegistryStorage: m.files, release: release},
					journal:         journal, result: &result, waitCanceled: ack,
				}
				probe, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				done := make(chan ap.Failure, 2)
				open := func(ctx context.Context) {
					d, failure := storage.Open(ctx, m.c, id, true)
					if d != nil {
						d.Close()
					}
					done <- failure
				}
				go open(probe)
				synctest.Wait()
				<-probe.Done()
				reconcile, end := context.WithCancel(context.Background())
				defer end()
				go open(reconcile)
				synctest.Wait()
				if fmt.Sprint(journal.events) != "[probe-start]" {
					t.Errorf("reconciliation overtook the pending expiry observation: %v", journal.events)
				}
				want := "[probe-start probe-expired reconcile-start reconcile-acquired]"
				pending := 2
				if canceled {
					end()
					synctest.Wait()
					select {
					case failure := <-done:
						pending--
						if failure != "state.unavailable" {
							t.Errorf("canceled reconciliation = %q", failure)
						}
					default:
						t.Error("canceled reconciliation still waits for the probe observation")
					}
					want = "[probe-start probe-expired]"
				}
				close(release)
				for range pending {
					<-done
				}
				if fmt.Sprint(journal.events) != want || result.ProbeSeconds != 10 {
					t.Errorf("probe/reconciliation observations = %v, probe seconds = %d", journal.events, result.ProbeSeconds)
				}
			})
		})
	}
}

type runExitEngine struct {
	sp.RunEngine
	wait       func(context.Context) (int, sp.Outcome)
	attachment *runExitAttachment
}

func (e runExitEngine) OpenRun(ctx context.Context, b sp.Binding) (sp.RunConnection, sp.Outcome) {
	c, f := e.RunEngine.OpenRun(ctx, b)
	if f != "" {
		return c, f
	}
	return runExitConnection{c, e}, ""
}

type runExitConnection struct {
	sp.RunConnection
	e runExitEngine
}

func (c runExitConnection) Wait(ctx context.Context, _ string) (int, sp.Outcome) {
	return c.e.wait(ctx)
}

func (c runExitConnection) Attach(ctx context.Context, id string, streams sp.Streams) (sp.Attachment, sp.Outcome) {
	a, f := c.RunConnection.Attach(ctx, id, streams)
	if f != "" {
		return a, f
	}
	c.e.attachment.Attachment, c.e.attachment.streams = a, streams
	return c.e.attachment, ""
}

type runExitAttachment struct {
	sp.Attachment
	failures  chan sp.Outcome
	completed chan struct{}
	streams   sp.Streams
	journal   *runExitJournal
	result    *runExitObservation
}

func (a *runExitAttachment) Failures() <-chan sp.Outcome { return a.failures }
func (a *runExitAttachment) Drain(ctx context.Context) sp.Outcome {
	deadline, _ := ctx.Deadline()
	a.result.DrainBudgetSeconds = int(time.Until(deadline) / time.Second)
	select {
	case <-a.completed:
	case <-ctx.Done():
		return sp.ExecutionFailed
	}
	a.journal.add("drain")
	for _, stream := range []struct {
		writer sp.Writer
		marker string
	}{{a.streams.Out, "slice-24-trailing-stdout\n"}, {a.streams.Err, "slice-24-trailing-stderr\n"}} {
		if n, err := stream.writer.Write([]byte(stream.marker)); err != nil || n != len(stream.marker) {
			return sp.ExecutionFailed
		}
	}
	return a.Attachment.Drain(ctx)
}

func observeRunExit(t *testing.T, schedule, name string, afterCancel func()) runExitObservation {
	t.Helper()
	m := publicMemoryFixture(t)
	var result runExitObservation
	journal := &runExitJournal{}
	waitCanceled := make(chan struct{})
	storage := &runExitStorage{RegistryStorage: m.files, journal: journal, result: &result, waitCanceled: waitCanceled}
	m.registry.Storage = storage
	switch name {
	case "zero":
		m.e.WaitFailure, m.e.Exit = "", 0
	case "seven", "seven-drain":
		m.e.WaitFailure, m.e.Exit = "", 7
	case "seventy-five":
		m.e.WaitFailure, m.e.Exit = "", 75
	}
	if name == "unknown-drain" || name == "seven-drain" {
		m.e.DrainFailure = sp.ExecutionFailed
	}
	start := time.Now()
	completed, locked := make(chan struct{}), make(chan struct{})
	attachment := &runExitAttachment{failures: make(chan sp.Outcome, 1), completed: completed, journal: journal, result: &result}
	id, intentName := "", ""
	m.files.AfterRead = func() {
		for _, files := range m.files.Sessions {
			r, f := m.registry.Codec.DecodePublic(files.Data)
			if f != "" {
				t.Fatal(f)
			}
			id = r.ID
			if r.State == "stopped" && result.CompletedRecord == "" {
				result.CompletedRecord = string(files.Data)
				for filename, data := range m.storage.Directories["intents/"+m.c.Project].Files {
					intentName, result.CompletedIntent = filename, string(data)
				}
				result.MutationsAtCompletion = recoveryReviewMutations(m.e)
				journal.add("completion-published")
			}
		}
	}
	signals := 0
	delayedStopWait := false
	m.e.After = func(event string) {
		if event == "SIGTERM" {
			journal.add("stop-locked")
			signals++
			if signals == 1 {
				close(locked)
				if schedule == "after the stdin reconciliation deadline" {
					// Keep stop's lock across run's probe with two healthy calls,
					// each inside slice-12's ten-second finite-operation limit.
					time.Sleep(6 * time.Second)
				}
			}
		}
		if event == "wait" {
			journal.add("stop-wait")
			if schedule == "after the stdin reconciliation deadline" && !delayedStopWait {
				delayedStopWait = true
				time.Sleep(5 * time.Second)
			}
		}
	}
	recoverCommand := func(command string) runExitCommand {
		var out, errout bytes.Buffer
		ui := presentation.UI{Out: &out, Err: &errout}
		var code int
		if command == "status" {
			code = (status.CLI{Service: m.service(), Presentation: ui, JSON: true}).Run()
		} else {
			code = (stop.CLI{Service: m.service(), Presentation: ui, JSON: true, Args: []string{id}}).Run()
		}
		return runExitCommand{code, out.String(), errout.String()}
	}
	engine := runExitEngine{RunEngine: m.e, attachment: attachment, wait: func(ctx context.Context) (int, sp.Outcome) {
		journal.add("run-wait-pending")
		go func() {
			result.Stop = recoverCommand("stop")
			result.CompletionSeconds = int(time.Since(start) / time.Second)
			journal.add("stop-returned")
			close(completed)
		}()
		if schedule == "before stdin reconciliation" {
			<-completed
		} else {
			<-locked
		}
		journal.add("stdin-closed")
		attachment.failures <- sp.StdinClosed
		<-ctx.Done()
		if afterCancel != nil {
			afterCancel()
		}
		journal.add("run-wait-canceled")
		close(waitCanceled)
		return 0, sp.ExecutionFailed
	}}
	runner := domain.Runner{Store: m.registry.Store, Registry: m.registry, Engine: engine, Workspace: sf.MountWorkspace(m.b), Entropy: &publicMemoryEntropy{}}
	approval := &runfake.Approval{View: ap.View{Inspection: af.Ready(), Candidate: m.c, Record: ap.Record{Candidate: m.c, Generation: "1"}, Status: "matching"}}
	var out, errout bytes.Buffer
	code := (runcommand.CLI{Approval: approval, Snapshots: &af.Snapshots{}, Runner: runner, Args: []string{"--", "probe"}}).Run(&cli.UI{Stdout: &out, Stderr: &errout, Classify: runcommand.Classify})
	journal.add("run-returned")
	result.Run = runExitCommand{code, out.String(), errout.String()}
	result.ElapsedSeconds = int(time.Since(start) / time.Second)
	// A refusal before wait is captured as a result, without blocking the fixture.
	if id != "" {
		result.Repeated, result.Status = recoverCommand("stop"), recoverCommand("status")
		result.FinalRecord = string(m.files.Sessions[m.index().Project+"/"+id].Data)
		result.FinalIntent = string(m.storage.Directories["intents/"+m.c.Project].Files[intentName])
	}
	result.MutationsAfter = recoveryReviewMutations(m.e)
	result.Events = journal.events
	return result
}
