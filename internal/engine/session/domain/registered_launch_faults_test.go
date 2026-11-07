package domain

import (
	"context"
	"slices"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestRegisteredLaunchRetainsEvidenceAtEveryFailedBoundary(t *testing.T) {
	for _, tc := range []struct {
		name       string
		want       port.Outcome
		phase      string
		last       string
		attached   bool
		started    bool
		returnedID bool
	}{
		{"session changed before create", port.Refused, "failed", "open", false, false, false},
		{"workspace changed before create", port.Refused, "preparing", "open", false, false, false},
		{"create boundary write failed", port.IntentWrite, "preparing", "open", false, false, false},
		{"invalid create identity", port.Uncertain, "preparing", "create", false, false, false},
		{"missing resource state", port.IsolationInspection, "preparing", "create", false, false, false},
		{"foreign resource", port.Refused, "preparing", "create", false, false, false},
		{"ownership write failed", port.OwnershipWrite, "preparing", "create", false, false, false},
		{"ready write failed", port.IntentWrite, "preparing", "create", false, false, false},
		{"attachment failed", port.ExecutionFailed, "ready", "attach", false, false, false},
		{"session changed during attach", port.Refused, "failed", "attach", true, false, false},
		{"workspace changed during attach", port.Refused, "ready", "attach", true, false, false},
		{"isolation changed during attach", port.IsolationUnverified, "ready", "verify", true, false, false},
		{"start failed", port.ExecutionFailed, "ready", "start", true, false, false},
		{"deadline after start", port.ExecutionFailed, "ready", "start", true, true, true},
		{"running write failed", port.IntentWrite, "ready", "start", true, true, true},
		{"deadline during running write", port.ExecutionFailed, "ready", "start", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner, registry, engine, workspace, id := registeredSessionFixture(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			original := registry.Records[id]
			changeRecord := func() {
				r := registry.Records[id]
				r.State = "failed"
				registry.Records[id] = r
			}
			switch tc.name {
			case "session changed before create":
				engine.AfterOpen = changeRecord
			case "workspace changed before create":
				engine.AfterOpen = func() { workspace.Changed = true }
			case "create boundary write failed":
				registry.Store.Fault = "intent file fsync"
			case "invalid create identity":
				engine.CreateID = "invalid"
			case "missing resource state":
				engine.Inspection = "missing State"
			case "foreign resource":
				engine.Inspection = "different intent label"
			case "ownership write failed":
				registry.Store.Fault = "owned-record file fsync"
			case "ready write failed":
				registry.SaveFailure = port.IntentWrite
			case "attachment failed":
				engine.AttachFailure = port.ExecutionFailed
			case "session changed during attach", "workspace changed during attach", "isolation changed during attach":
				engine.After = func(event string) {
					if event == "attach" {
						switch tc.name {
						case "session changed during attach":
							changeRecord()
						case "workspace changed during attach":
							workspace.Changed = true
						case "isolation changed during attach":
							engine.Boundary = port.IsolationUnverified
						}
					}
				}
			case "start failed":
				engine.StartFailure = port.ExecutionFailed
			case "deadline after start":
				engine.AfterStart = cancel
			case "running write failed", "deadline during running write":
				registry.BeforeSave = func(r port.PublicRecord) {
					if r.State == "running" {
						registry.SaveFailure = port.IntentWrite
						if tc.name == "deadline during running write" {
							cancel()
						}
					}
				}
			}

			connection, attachment, worker, failure := runner.registeredLaunch(ctx, id, []string{"probe"}, port.Streams{})
			if connection != nil {
				defer connection.Close()
			}
			if attachment != nil {
				defer attachment.Close()
			}
			if failure != tc.want || (attachment != nil) != tc.attached || (worker != "") != tc.returnedID || (tc.returnedID && worker != engine.CreateID) {
				t.Fatal("launch lost its failure or reported unproven progress", worker, failure)
			}
			if engine.Events[len(engine.Events)-1] != tc.last || slices.Contains(engine.Events, "wait") || slices.Contains(engine.Events, "drain") || engine.Removed {
				t.Fatal("launch advanced beyond the failed boundary", engine.Events)
			}
			if engine.Resources[engine.CreateID].Started != tc.started || engine.Resources[engine.CreateID].Running != tc.started {
				t.Fatal("launch started a refused worker or lost accepted start evidence", engine.Resources)
			}
			r := registry.Records[id]
			if r.State != tc.phase || r.ID != original.ID || r.Project != original.Project || r.Intent != original.Intent || r.WorkloadExit != nil {
				t.Fatal("launch replaced original recovery evidence or invented exit", r)
			}
			if registry.Locks[id] || registry.Store.Locked || !workspace.Closed {
				t.Fatal("launch retained operation or workspace authority")
			}
		})
	}
}

func TestRegisteredCreateRefusesAnAlreadyOwnedIntent(t *testing.T) {
	runner, registry, engine, workspace, id := registeredSessionFixture(t)
	c, a, worker, failure := runner.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{})
	if failure != "" {
		t.Fatal(failure)
	}
	defer c.Close()
	defer a.Close()
	entry, failure := registry.Open(t.Context(), id, false)
	if failure != "" {
		t.Fatal(failure)
	}
	defer entry.Close()
	if failure = workspace.Pin(entry.Intent().Binding()); failure != "" {
		t.Fatal(failure)
	}
	defer workspace.Close()
	before := len(engine.Events)
	if got, f := createRegistered(t.Context(), entry, c, workspace, []string{"probe"}); got != "" || f != port.Refused {
		t.Fatal("existing ownership allowed another create", got, f)
	}
	if len(engine.Events) != before || len(engine.Resources) != 1 || !engine.Resources[worker].Running || registry.Records[id].State != "running" {
		t.Fatal("duplicate create changed the owned worker or registration")
	}
}
