package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/presentation"
	status "github.com/donvargax/itos-container/internal/slices/status/command"
	stop "github.com/donvargax/itos-container/internal/slices/stop/command"
)

// The public command assembly uses the same stateful, I/O-free port fakes as the
// domain contracts. Filesystem protections and competing processes are exercised
// separately by the disk/SDK feature route, not simulated as OS behavior here.
func TestPublicRecoveryCommandsWithPortFakes(t *testing.T) {
	for _, state := range []string{"preparing", "ready", "running", "failed", "stopped"} {
		for _, known := range []bool{false, true} {
			t.Run(state+map[bool]string{false: "-unknown", true: "-observed"}[known], func(t *testing.T) {
				b, store, engine := sf.Fixture()
				ctx := context.Background()
				id, token, worker := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
				i, f := store.New(ctx, ap.Durable{Frozen: b.Frozen}, token)
				if f != "" {
					t.Fatal(f)
				}
				r := i.Record()
				r.Stage, r.ContainerID = "owned", worker
				if state == "stopped" {
					r.Stage = "absent"
				}
				if f = i.Save(r); f != "" {
					t.Fatal(f)
				}
				i.Close()
				public := sp.PublicRecord{Schema: 1, ID: id, Project: r.Project, Intent: sp.Reference(r.Project, token), State: state}
				if known {
					exit := 7
					public.WorkloadExit = &exit
				}
				registry := &sf.Registry{Store: store, Project: r.Project, Records: map[string]sp.PublicRecord{id: public}}
				resource := "created"
				if state == "running" {
					resource = "running"
				}
				if state == "failed" {
					resource = "exited"
				}
				engine.Resources = map[string]sp.Observation{}
				if state != "stopped" {
					engine.Resources[worker] = sp.Observation{ID: worker, Name: r.Name, ImageID: r.ImageID, Token: token, Project: r.Project, Fingerprint: r.Fingerprint, StatePresent: true, Status: resource, Running: resource == "running", Started: resource != "created"}
				} else {
					resource = "absent"
				}
				recovery := domain.Recovery{Registry: registry, Engine: &sf.RunEngine{Engine: engine, WaitFailure: sp.ExecutionFailed}}
				var out, errout bytes.Buffer
				ui := presentation.UI{Out: &out, Err: &errout}
				if code := (status.CLI{Service: recovery, Presentation: ui, JSON: true}).Run(); code != 0 {
					t.Fatal(code, errout.String())
				}
				var report struct {
					Schema   int
					OK       bool
					Sessions []sp.RecoveryEntry
					Problems []presentation.Problem
				}
				if json.Unmarshal(out.Bytes(), &report) != nil || report.Schema != 1 || !report.OK || len(report.Sessions) != 1 || report.Problems == nil {
					t.Fatal(out.String())
				}
				e := report.Sessions[0]
				if e.ID != id || e.Resource != resource || e.Fingerprint != r.Fingerprint || e.Project != r.Project || e.State != state || (e.WorkloadExit != nil) != known || registry.Writes != 0 {
					t.Fatal(e)
				}
				for n := 0; n < 2; n++ {
					out.Reset()
					errout.Reset()
					writes := registry.Writes
					if code := (stop.CLI{Service: recovery, Presentation: ui, JSON: true, Args: []string{id}}).Run(); code != 0 {
						t.Fatal(code, errout.String())
					}
					if json.Unmarshal(out.Bytes(), &report) != nil || len(report.Sessions) != 1 {
						t.Fatal(out.String())
					}
					e = report.Sessions[0]
					if e.Cleanup != "complete" || e.Resource != "absent" || e.State != "stopped" || e.Retry != nil || (e.WorkloadExit != nil) != known || known && *e.WorkloadExit != 7 {
						t.Fatal(e)
					}
					if n == 1 && writes != registry.Writes {
						t.Fatal("repeat stop rewrote completion")
					}
				}
			})
		}
	}
}

func TestPublicRecoveryCommandRefusalsWithPortFakes(t *testing.T) {
	for _, fault := range []struct {
		outcome sp.Outcome
		rule    string
		code    int
	}{{sp.Refused, "not-owned", 1}, {sp.Unsafe, "unsafe-state", 1}, {sp.Invalid, "invalid-state", 2}, {sp.Environment, "environment", 3}, {sp.Incomplete, "temporary", 75}, {"synthetic-internal-marker", "internal", 70}} {
		for _, operation := range []string{"status", "stop"} {
			t.Run(operation+fault.rule, func(t *testing.T) {
				var out, errout bytes.Buffer
				ui := presentation.UI{Out: &out, Err: &errout}
				registry := &sf.Registry{Failure: fault.outcome}
				service := domain.Recovery{Registry: registry}
				code := 0
				if operation == "status" {
					code = (status.CLI{Service: service, Presentation: ui, JSON: true}).Run()
				} else {
					code = (stop.CLI{Service: service, Presentation: ui, JSON: true, Args: []string{strings.Repeat("a", 64)}}).Run()
				}
				var r struct {
					Schema   int
					OK       bool
					Problems []presentation.Problem
					Sessions []sp.RecoveryEntry
				}
				if json.Unmarshal(out.Bytes(), &r) != nil || code != fault.code || r.Schema != 1 || r.OK || len(r.Problems) != 1 || r.Problems[0].Rule != "recovery."+fault.rule || r.Sessions == nil || registry.Writes != 0 || strings.Contains(out.String()+errout.String(), "synthetic-internal-marker") {
					t.Fatal(out.String(), errout.String(), code)
				}
			})
		}
	}
}
