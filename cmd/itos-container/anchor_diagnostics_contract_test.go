package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"syscall"
	"testing"

	"github.com/donvargax/itos-container/internal/cli"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/disk"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
	runfake "github.com/donvargax/itos-container/internal/slices/run/port/porttest"
)

type anchorDiagnosticObservation struct {
	Run                runExitCommand
	Expected, Observed string
	Events             []string
	Workers, Anchors   int
}

func TestPublicAnchorDiagnosticContract(t *testing.T) {
	mode := os.Getenv("SLICE_17_DIAGNOSTIC")
	if mode == "" {
		t.Skip("anchor diagnostic feature subprocess only")
	}
	m := publicMemoryFixture(t)
	// This row describes a failed observation of a still-running worker, not
	// delayed publication of a real exit. The fake must not manufacture an exit
	// when reconciliation waits for the engine's independent confirmation.
	m.e.WaitUntilCancel = mode == "pidfd"
	w := &diagnosticWorkspace{Workspace: &disk.Workspace{}, anchor: sf.MountWorkspace(m.b), mode: mode}
	w.MountStat = func(path string) (fs.FileInfo, error) {
		return nil, &os.PathError{Op: "stat", Path: "SYNTHETIC_PRIVATE_PATH", Err: syscall.EACCES}
	}
	j := &runExitJournal{}
	m.e.After = j.add
	m.e.AfterStart = func() {
		for id, o := range m.e.Resources {
			if mode == "pidfd" {
				o.PID = 1 << 30
			}
			if mode == "invalid PID" {
				o.PID = 0
			}
			m.e.Resources[id] = o
		}
	}
	runner := domain.Runner{Store: m.registry.Store, Registry: m.registry, Engine: m.e, Workspace: w, Entropy: &publicMemoryEntropy{}}
	approval := &runfake.Approval{View: ap.View{Inspection: af.Ready(), Candidate: m.c, Record: ap.Record{Candidate: m.c, Generation: "1"}, Status: "matching"}}
	var out, errout bytes.Buffer
	code := (runcommand.CLI{Approval: approval, Snapshots: &af.Snapshots{}, Runner: runner, Args: []string{"--", "SYNTHETIC_PRIVATE_ARGV"}}).Run(&cli.UI{Stdout: &out, Stderr: &errout, Classify: runcommand.Classify})
	r := anchorDiagnosticObservation{Run: runExitCommand{code, out.String(), errout.String()}, Expected: m.b.Checkout.Device + ":" + m.b.Checkout.Inode, Workers: len(m.e.Resources), Anchors: len(m.e.Anchors), Events: j.events}
	observed := w.anchor.Mounts[123]
	if mode == "device" {
		observed.Device += "1"
	}
	if mode == "inode" {
		observed.Inode += "1"
	}
	r.Observed = observed.Device + ":" + observed.Inode
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("SLICE_17_DIAGNOSTIC_RESULT=%s\n", data)
}

// Keep the fake engine's source in memory; exercise the real syscall adapter
// only for the failing observation. No native engine or real state is accessed.
type diagnosticWorkspace struct {
	*disk.Workspace
	anchor *sf.Workspace
	mode   string
}

func (w *diagnosticWorkspace) Pin(b sp.Binding) sp.Outcome          { return w.anchor.Pin(b) }
func (w *diagnosticWorkspace) Validate() sp.Outcome                 { return w.anchor.Validate() }
func (w *diagnosticWorkspace) Source() (sp.MountSource, sp.Outcome) { return w.anchor.Source() }
func (w *diagnosticWorkspace) Close()                               { w.anchor.Close(); w.Workspace.Close() }
func (w *diagnosticWorkspace) HoldProcess(pid int) (sp.Process, sp.Outcome) {
	if w.mode == "pidfd" {
		return w.Workspace.HoldProcess(pid)
	}
	return &sf.Process{}, ""
}
func (w *diagnosticWorkspace) Mounted(pid int) (sp.MountSource, sp.Outcome) {
	if w.mode == "stat" {
		return w.Workspace.Mounted(pid)
	}
	s, f := w.anchor.Mounted(pid)
	if w.mode == "device" {
		s.Device += "1"
	}
	if w.mode == "inode" {
		s.Inode += "1"
	}
	return s, f
}
