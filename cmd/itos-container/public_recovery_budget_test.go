package main

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/cli"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
	runfake "github.com/donvargax/itos-container/internal/slices/run/port/porttest"
)

// These cases run through the scenario subprocess while red. They exercise
// production command, registry and record adapters with a controlled clock.
func TestPublicRecoveryBudgetRegressions(t *testing.T) {
	t.Run("SIGTERM deadline", func(t *testing.T) { publicRecoveryTerminationDeadline(t, false) })
	t.Run("SIGKILL deadline", func(t *testing.T) { publicRecoveryTerminationDeadline(t, true) })
	for _, name := range []string{"unknown", "nonzero", "drain failure", "incomplete stop", "changed record"} {
		t.Run(name, func(t *testing.T) { publicRecoveryDelayedStop(t, name) })
	}
}

func publicRecoveryTerminationDeadline(t *testing.T, kill bool) {
	synctest.Test(t, func(t *testing.T) {
		m := publicMemoryFixture(t)
		id, worker := m.seed("a")
		o := m.e.Resources[worker]
		o.Status, o.Running, o.Started, o.PID = "running", true, true, 123
		m.e.Resources[worker] = o
		m.e.WaitUntilCancel, m.e.IgnoreSignal = true, true
		files := m.files.Sessions[m.index().Project+"/"+id]
		locked, f := m.files.Open(context.Background(), m.index(), id, false)
		if f != "" || locked == nil {
			t.Fatal(f)
		}
		lockTime := 25 * time.Second
		if kill {
			lockTime = 15 * time.Second
		}
		go func() {
			time.Sleep(lockTime)
			locked.Close()
		}()
		waits, writes := 0, 0
		var progress []byte
		m.e.After = func(event string) {
			if event != "wait" {
				return
			}
			waits++
			progress, writes = bytes.Clone(files.Data), len(m.files.Events)
			if kill && waits == 2 {
				// SIGKILL has been requested but its wait response stalls through
				// the last five seconds of the original stop budget.
				time.Sleep(5 * time.Second)
				synctest.Wait()
			}
		}
		start := time.Now()
		r := publicMemoryCommand(t, m.service(), "stop", id, 75)
		if time.Since(start) != 30*time.Second || len(r.Problems) != 1 || r.Problems[0].Rule != "recovery.temporary" || len(r.Sessions) != 1 {
			t.Fatal("termination lost its original deadline or temporary cause", r, time.Since(start))
		}
		e := r.Sessions[0]
		if e.State != "failed" || e.Resource != "unknown" || e.Cleanup != "unknown" || e.WorkloadExit != nil || !slices.Equal(e.Retry, []string{"itos", "container", "stop", id}) {
			t.Fatal("termination timeout invented completion or lost the retry", e)
		}
		wantWaits := 1
		if kill {
			wantWaits = 2
		}
		if waits != wantWaits || !slices.Contains(m.e.Events, "SIGTERM") || slices.Contains(m.e.Events, "SIGKILL") != kill ||
			!bytes.Equal(progress, files.Data) || writes != len(m.files.Events) || len(m.e.Resources) != 1 {
			t.Fatal("termination exceeded its budget or lost owned progress", m.e.Events, waits, m.files.Events)
		}
		m.e.After, m.e.WaitUntilCancel, m.e.IgnoreSignal = nil, false, false
		r = publicMemoryCommand(t, m.service(), "stop", id, 0)
		if len(r.Sessions) != 1 || r.Sessions[0].Cleanup != "complete" || r.Sessions[0].WorkloadExit != nil || len(m.e.Resources) != 0 {
			t.Fatal("termination timeout lost safe retry authority", r)
		}
	})
}

func publicRecoveryPendingStop(t *testing.T) {
	publicRecoveryDelayedStop(t, "unknown")
}

func publicRecoveryDelayedStop(t *testing.T, name string) {
	synctest.Test(t, func(t *testing.T) {
		m := publicMemoryFixture(t)
		m.e.StdinClosed, m.e.WaitUntilCancel = true, true
		want := 0
		if name == "nonzero" {
			m.e.WaitFailure, m.e.Exit, want = "", 7, 7
		}
		if name == "drain failure" {
			m.e.DrainFailure, want = sp.ExecutionFailed, 75
		}
		if name == "changed record" {
			want = 75
		}
		if name == "incomplete stop" {
			m.e.SignalFailure, want = sp.Incomplete, 75
		}
		id := ""
		finished := make(chan struct{})
		var completed []byte
		mutations := 0
		signals := 0
		if name == "changed record" {
			reads := 0
			m.files.AfterRead = func() {
				for _, files := range m.files.Sessions {
					r, f := m.registry.Codec.DecodePublic(files.Data)
					if f != "" || r.State != "stopped" {
						continue
					}
					reads++
					if reads == 4 {
						// Completion validation reads once, run's Open twice.
						// Replace bytes before its held-session revalidation.
						files.Data = append(files.Data, ' ')
					}
				}
			}
		}
		m.e.After = func(event string) {
			if event == "SIGTERM" {
				// Hold the real operation-lock contract across run's ten-second
				// probe; the competing stop still has time to finish its total.
				signals++
				if signals == 1 {
					time.Sleep(11 * time.Second)
				}
				return
			}
			if event != "wait" || id != "" {
				return
			}
			for _, files := range m.files.Sessions {
				r, f := m.registry.Codec.DecodePublic(files.Data)
				if f != "" {
					t.Fatal(f)
				}
				id = r.ID
			}
			go func() {
				defer close(finished)
				stopExit := 0
				if name == "incomplete stop" {
					stopExit = 75
				}
				r := publicMemoryCommand(t, m.service(), "stop", id, stopExit)
				if len(r.Sessions) != 1 || (r.Sessions[0].Cleanup == "complete") != (stopExit == 0) || (r.Sessions[0].WorkloadExit != nil) != (name == "nonzero") {
					t.Error("competing stop lost its result", r)
				}
				completed = bytes.Clone(m.files.Sessions[m.index().Project+"/"+id].Data)
				mutations = recoveryReviewMutations(m.e)
			}()
			synctest.Wait()
		}
		runner := domain.Runner{Store: m.registry.Store, Registry: m.registry, Engine: m.e, Workspace: &sf.Workspace{}, Entropy: &publicMemoryEntropy{}}
		approval := &runfake.Approval{View: ap.View{Inspection: af.Ready(), Candidate: m.c, Record: ap.Record{Candidate: m.c, Generation: "1"}, Status: "matching"}}
		var out, errout bytes.Buffer
		start := time.Now()
		code := (runcommand.CLI{Approval: approval, Snapshots: &af.Snapshots{}, Runner: runner, Args: []string{"--", "probe"}}).Run(&cli.UI{Stdout: &out, Stderr: &errout, Classify: runcommand.Classify})
		<-finished
		if code != want || (want == 75 && !strings.Contains(errout.String(), "run.execution-unavailable")) || (want != 75 && errout.Len() != 0) {
			t.Fatalf("run lost the competing stop outcome: exit %d, want %d, %s", code, want, errout.String())
		}
		m.files.AfterRead = nil
		status := publicMemoryCommand(t, m.service(), "status", "", 0)
		if name == "incomplete stop" {
			if len(status.Sessions) != 1 || status.Sessions[0].Cleanup != "pending" || status.Sessions[0].WorkloadExit != nil || len(m.e.Resources) != 1 {
				t.Fatal("failed competing stop was adopted as completion", status)
			}
			return
		}
		if time.Since(start) != 11*time.Second || len(status.Sessions) != 1 || status.Sessions[0].State != "stopped" || status.Sessions[0].Resource != "absent" || status.Sessions[0].Cleanup != "complete" || (status.Sessions[0].WorkloadExit != nil) != (name == "nonzero") || status.Sessions[0].Retry != nil || len(m.e.Resources) != 0 {
			t.Fatal("run lost bounded cleanup or invented a workload exit", status)
		}
		if name == "changed record" {
			completed = append(completed, ' ')
		}
		if !bytes.Equal(completed, m.files.Sessions[m.index().Project+"/"+id].Data) || mutations != recoveryReviewMutations(m.e) || (want != 75 && !slices.Contains(m.e.Events, "drain")) {
			t.Fatal("run repeated cleanup or failed to drain the remaining output")
		}
	})
}
