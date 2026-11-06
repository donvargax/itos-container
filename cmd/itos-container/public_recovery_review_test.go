package main

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"slices"
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

// The black-box scenario subprocess selects individual regression cases. Its
// commands use production domains, codecs and binding adapters over stateful
// port fakes; faults never supply a successful command result or completed record.
func TestPublicRecoveryReviewContract(t *testing.T) {
	switch os.Getenv("SLICE_11_REVIEW_CASE") {
	case "":
		t.Skip("recovery review feature subprocess only")
	case "inspection deadline after lock":
		publicRecoveryInspectionDeadline(t)
	case "controller probe unavailable":
		publicRecoveryUnknownController(t)
	case "completed stop during stdin closure":
		publicRecoveryStoppedWait(t)
	default:
		t.Fatal("unknown recovery review fixture")
	}
}

func TestPublicRecoveryInterruptionRegressions(t *testing.T) {
	t.Run("deadline", publicRecoveryInspectionDeadline)
	t.Run("controller", publicRecoveryUnknownController)
	t.Run("completed-wait", publicRecoveryStoppedWait)
}

func publicRecoveryInspectionDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := publicMemoryFixture(t)
		id, worker := m.seed("a")
		original := m.e.Resources[worker]
		files := m.files.Sessions[m.index().Project+"/"+id]
		before := bytes.Clone(files.Data)
		locked, f := m.files.Open(context.Background(), m.c, id, false)
		if f != "" {
			t.Fatal(f)
		}
		go func() {
			time.Sleep(25 * time.Second)
			locked.Close()
		}()
		m.e.BeforeInspect = func() {
			time.Sleep(5 * time.Second)
			synctest.Wait()
		}
		start := time.Now()
		r := publicMemoryCommand(t, m.service(), "stop", id, 75)
		if time.Since(start) != 30*time.Second || len(r.Problems) != 1 || r.Problems[0].Rule != "recovery.temporary" || len(r.Sessions) != 1 {
			t.Fatal("stop lost its original deadline or temporary cause", r, time.Since(start))
		}
		e := r.Sessions[0]
		if e.State != "failed" || e.Resource != "unknown" || e.Cleanup != "unknown" || e.WorkloadExit != nil || !slices.Equal(e.Retry, []string{"itos", "container", "stop", id}) {
			t.Fatal("timeout invented completion or lost the retry", e)
		}
		if !bytes.Equal(before, files.Data) || len(m.files.Events) != 0 || !reflect.DeepEqual(m.e.Resources[worker], original) {
			t.Fatal("expired stop changed protected progress or its worker")
		}
		m.e.BeforeInspect = nil
		r = publicMemoryCommand(t, m.service(), "stop", id, 0)
		if len(r.Sessions) != 1 || r.Sessions[0].Cleanup != "complete" || r.Sessions[0].WorkloadExit != nil || len(m.e.Resources) != 0 {
			t.Fatal("safe authority was not available to retry", r)
		}
	})
}

// Decorate only the advisory lease observation. Protected paths, operation
// locks, record comparisons and all storage transactions retain their behavior.
type unavailableControllerStorage struct{ ap.RegistryStorage }
type unavailableControllerDirectory struct{ ap.SessionDirectory }

func (s unavailableControllerStorage) Open(ctx context.Context, c ap.Candidate, id string, readOnly bool) (ap.SessionDirectory, ap.Failure) {
	d, f := s.RegistryStorage.Open(ctx, c, id, readOnly)
	if f != "" || d == nil {
		return d, f
	}
	return unavailableControllerDirectory{d}, ""
}
func (unavailableControllerDirectory) Controller() (string, ap.Failure) {
	return "unknown", "state.unavailable"
}

func publicRecoveryUnknownController(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		t.Run(map[bool]string{false: "owned", true: "foreign"}[foreign], func(t *testing.T) {
			m := publicMemoryFixture(t)
			id, worker := m.seed("a")
			_, other := m.seed("b")
			unrelated := m.e.Resources[other]
			o := m.e.Resources[worker]
			o.Status, o.Running, o.Started, o.PID = "running", true, true, 123
			if foreign {
				o.ImageID = "foreign"
			}
			m.e.Resources[worker] = o
			m.files.Sessions[m.index().Project+"/"+id].Controller = true
			m.registry.Storage = unavailableControllerStorage{m.files}
			status := publicMemoryCommand(t, m.service(), "status", "", 75)
			if len(status.Problems) != 2 || status.Problems[0].Rule != "recovery.temporary" || status.Sessions[0].Controller != "unknown" || len(m.files.Events) != 0 {
				t.Fatal("status hid the failed lease observation", status)
			}
			want := 0
			if foreign {
				want = 1
			}
			r := publicMemoryCommand(t, m.service(), "stop", id, want)
			if foreign {
				if len(r.Problems) != 1 || r.Problems[0].Rule != "recovery.not-owned" || !reflect.DeepEqual(m.e.Resources[worker], o) || len(m.files.Events) != 0 || slices.Contains(m.e.Events, "SIGTERM") {
					t.Fatal("unknown controller bypassed ownership checks", r)
				}
				return
			}
			if len(r.Sessions) != 1 || r.Sessions[0].Controller != "unknown" || r.Sessions[0].Cleanup != "complete" || r.Sessions[0].WorkloadExit != nil || r.Sessions[0].Retry != nil {
				t.Fatal("unknown controller blocked owned completion", r)
			}
			if !reflect.DeepEqual(m.e.Resources, map[string]sp.Observation{other: unrelated}) || !m.files.Sessions[m.index().Project+"/"+id].Controller || !slices.Contains(m.e.Events, "SIGTERM") {
				t.Fatal("stop failed to terminate only its owned worker", m.e.Events)
			}
		})
	}
}

func publicRecoveryStoppedWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := publicMemoryFixture(t)
		m.e.StdinClosed, m.e.WaitUntilCancel = true, true
		id := ""
		var completed []byte
		mutations := 0
		m.e.After = func(event string) {
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
			stopped := publicMemoryCommand(t, m.service(), "stop", id, 0)
			if len(stopped.Sessions) != 1 || stopped.Sessions[0].Cleanup != "complete" || stopped.Sessions[0].WorkloadExit != nil {
				t.Fatal("concurrent stop did not complete with an unknown exit", stopped)
			}
			completed = bytes.Clone(m.files.Sessions[m.index().Project+"/"+id].Data)
			mutations = recoveryReviewMutations(m.e)
			// Let stdin's notification cancel the original pending wait before
			// returning to its fake engine response. No real sleep or engine runs.
			synctest.Wait()
		}
		runner := domain.Runner{Store: m.registry.Store, Registry: m.registry, Engine: m.e, Workspace: &sf.Workspace{}, Entropy: &publicMemoryEntropy{}}
		approval := &runfake.Approval{View: ap.View{Inspection: af.Ready(), Candidate: m.c, Record: ap.Record{Candidate: m.c, Generation: "1"}, Status: "matching"}}
		var out, errout bytes.Buffer
		code := (runcommand.CLI{Approval: approval, Snapshots: &af.Snapshots{}, Runner: runner, Args: []string{"--", "probe"}}).Run(&cli.UI{Stdout: &out, Stderr: &errout, Classify: runcommand.Classify})
		if code != 0 || errout.Len() != 0 {
			t.Fatalf("run misreported completed cleanup as a wait failure: exit %d, %s", code, errout.String())
		}
		status := publicMemoryCommand(t, m.service(), "status", "", 0)
		if id == "" || len(status.Sessions) != 1 || status.Sessions[0].State != "stopped" || status.Sessions[0].Cleanup != "complete" || status.Sessions[0].WorkloadExit != nil || status.Sessions[0].Retry != nil || len(m.e.Resources) != 0 {
			t.Fatal("run invented a workload result or lost completed cleanup", status)
		}
		if !bytes.Equal(completed, m.files.Sessions[m.index().Project+"/"+id].Data) || mutations != recoveryReviewMutations(m.e) {
			t.Fatal("resumed run repeated mutation or rewrote completion")
		}
	})
}

func recoveryReviewMutations(e *sf.RunEngine) int {
	n := 0
	for _, event := range e.Engine.Events {
		if event.Method == "CREATE" || event.Method == "REMOVE" {
			n++
		}
	}
	for _, event := range e.Events {
		if event == "start" || event == "SIGTERM" || event == "SIGKILL" {
			n++
		}
	}
	return n
}
