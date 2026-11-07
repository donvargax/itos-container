package domain

import (
	"context"
	"slices"
	"strings"
	"testing"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestRegisteredLaunchCancelledAfterLockAcquisition(t *testing.T) {
	runner, registry, engine, workspace, id := registeredSessionFixture(t)
	before := registry.Records[id]
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	registry.AfterOpen = func(string) { cancel() }
	c, a, worker, failure := runner.registeredLaunch(ctx, id, []string{"probe"}, port.Streams{})
	if c != nil || a != nil || worker != "" || failure != port.ExecutionFailed {
		t.Fatal("cancelled lock acquisition advanced launch", worker, failure)
	}
	if registry.Locks[id] || registry.Store.Locked || workspace.Closed || len(engine.Events) != 0 || registry.Records[id] != before {
		t.Fatal("cancelled launch retained authority or entered another startup phase")
	}
}

func registeredSessionFixture(t *testing.T) (Runner, *f.Registry, *f.RunEngine, *f.Workspace, string) {
	t.Helper()
	b, store, engine := f.Fixture()
	id, token := strings.Repeat("1", 64), strings.Repeat("2", 64)
	durable := ap.Durable{Frozen: b.Frozen}
	registry := &f.Registry{Store: store, Project: b.Frozen.Project}
	lease, failure := registry.Register(t.Context(), durable, id, token)
	if failure != "" {
		t.Fatal(failure)
	}
	t.Cleanup(lease.Close)
	intent, failure := store.Prepare(t.Context(), durable, token)
	if failure != "" {
		t.Fatal(failure)
	}
	intent.Close()
	runEngine, workspace := &f.RunEngine{Engine: engine}, &f.Workspace{}
	return Runner{Store: store, Registry: registry, Engine: runEngine, Workspace: workspace}, registry, runEngine, workspace, id
}

func TestRegisteredLaunchRefusesBeforeStartAndReleasesAuthority(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		want                     port.Outcome
		events                   []string
		phase                    string
		connected, pinned, owned bool
	}{
		{"session unavailable", port.Incomplete, nil, "preparing", false, false, false},
		{"workspace refused", port.Refused, nil, "preparing", false, false, false},
		{"daemon unavailable", port.Incomplete, nil, "preparing", false, true, false},
		{"create refused", port.Refused, []string{"open", "create"}, "preparing", true, true, false},
		{"stop won before start", launchSuperseded, []string{"open", "create"}, "stopping", true, true, true},
		{"worker ID changed", port.Refused, []string{"open", "create"}, "ready", true, true, true},
		{"isolation unverified", port.IsolationUnverified, []string{"open", "create", "verify"}, "ready", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner, registry, engine, workspace, id := registeredSessionFixture(t)
			original := registry.Records[id]
			switch tc.name {
			case "session unavailable":
				registry.Failure = port.Incomplete
			case "workspace refused":
				workspace.Failure = port.Refused
			case "daemon unavailable":
				engine.Unavailable = true
			case "create refused":
				engine.ImageInspection = "declared volume"
			case "stop won before start", "worker ID changed":
				registry.AfterClose = func(sessionID string) {
					if registry.Records[sessionID].State != "ready" {
						return
					}
					registry.AfterClose = nil
					// Run releases the create lock before reacquiring authority
					// to start. Change the committed state in that interval.
					if tc.name == "stop won before start" {
						r := registry.Records[sessionID]
						r.State = "stopping"
						registry.Records[sessionID] = r
					} else {
						i, failure := registry.Store.Codec.Decode(registry.Store.Records[original.Intent])
						if failure != "" {
							t.Fatal(failure)
						}
						i.ContainerID = strings.Repeat("f", 64)
						registry.Store.Records[original.Intent] = registry.Store.Codec.Encode(i)
					}
				}
			case "isolation unverified":
				engine.Boundary = port.IsolationUnverified
			}

			connection, attachment, worker, failure := runner.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{})
			if connection != nil {
				defer connection.Close()
			}
			if attachment != nil {
				defer attachment.Close()
			}
			if failure != tc.want || worker != "" || attachment != nil || (connection != nil) != tc.connected {
				t.Fatal("refused launch lost its cause or returned a started workload", worker, failure)
			}
			if registry.Locks[id] || registry.Store.Locked || workspace.Closed != tc.pinned {
				t.Fatal("refused launch retained operation or workspace authority")
			}
			if !slices.Equal(engine.Events, tc.events) {
				t.Fatal("refused launch advanced beyond its last safe boundary", engine.Events)
			}
			r := registry.Records[id]
			if r.State != tc.phase || r.ID != original.ID || r.Project != original.Project || r.Intent != original.Intent || r.WorkloadExit != nil || r.LastProblem != nil {
				t.Fatal("launch overwrote committed recovery progress or invented an exit", r)
			}
			wantResources, wantWrites := 0, 1
			if tc.owned {
				wantResources, wantWrites = 1, 2
				o := engine.Resources[engine.CreateID]
				if o.ID != engine.CreateID || o.Status != "created" || o.Running || o.Started || o.PID != 0 {
					t.Fatal("refused launch changed its unstarted worker", o)
				}
			}
			if len(engine.Resources) != wantResources || registry.Writes != wantWrites || engine.Removed {
				t.Fatal("refused launch lost recovery evidence or mutated worker resources")
			}
		})
	}
}
