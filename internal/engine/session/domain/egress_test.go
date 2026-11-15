package domain

import (
	"context"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func egressSessionFixture(t *testing.T) (Runner, *f.Registry, *f.RunEngine, string) {
	t.Helper()
	runner, registry, engine, _, id := registeredSessionFixture(t)
	codec := registry.Store.Codec.(*f.Codec)
	codec.Value = f.EgressBinding(codec.Value)
	runner.Refusals = &f.RefusalLog{}
	engine.ProxyOutput = f.ProxyStdout
	engine.ProxyErrorOutput = f.ProxyStderr
	return runner, registry, engine, id
}
func seedEgressSession(t *testing.T, stage int) (Runner, *f.Registry, *f.RunEngine, string, port.Record) {
	t.Helper()
	runner, registry, engine, id := egressSessionFixture(t)
	b := registry.Store.Codec.(*f.Codec).Value
	r := f.EgressRecords(b)[stage]
	original := registry.Records[id]
	original.Intent = port.Reference(b.Frozen.Project, r.NameToken())
	original.State = "running"
	registry.Records[id] = original
	r.Anchor.Session = id
	registry.Store.Records[original.Intent] = registry.Store.Codec.Encode(r)
	engine.Networks = map[string]port.NetworkObservation{}
	for _, kind := range []port.NetworkKind{port.InternalNetwork, port.EgressNetwork} {
		o := f.EgressNetwork(r, kind)
		if o.ID != "" {
			engine.Networks[o.ID] = o
		}
	}
	if r.Egress.Proxy.ID != "" && r.Egress.Proxy.Stage != "absent" {
		o := f.EgressProxy(r, b)
		o.Running, o.Started, o.Status, o.PID = true, true, "running", 123
		engine.Resources[o.ID] = o
		engine.ProxyID = o.ID
		for k, n := range engine.Networks {
			n.Endpoints, n.ContainerID = 1, o.ID
			engine.Networks[k] = n
		}
	}
	if r.ContainerID != "" && r.Stage != "absent" {
		engine.Resources[r.ContainerID] = port.Observation{ID: r.ContainerID, Name: r.Name, ImageID: r.ImageID, Token: r.NameToken(), Project: r.Project, Fingerprint: r.Fingerprint, StatePresent: true, Status: "running", Running: true, Started: true, PID: 123, AnchorName: r.Anchor.Name, AnchorExact: true}
	}
	engine.Anchors = map[string]port.AnchorObservation{r.Anchor.Name: {Name: r.Anchor.Name, Created: r.Anchor.Created, Driver: "local", Scope: "local", Source: r.Anchor.Source, Type: "none", Options: "bind", Token: r.NameToken(), Project: r.Project, Fingerprint: r.Fingerprint, Session: id, Device: r.Anchor.Device, Inode: r.Anchor.Inode, ExactOptions: true, ExactLabels: true}}
	return runner, registry, engine, id, r
}
func launchEgress(t *testing.T, runner Runner, id string) port.Outcome {
	t.Helper()
	c, a, _, failure := runner.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{})
	if c != nil {
		c.Close()
	}
	if a != nil {
		a.Close()
	}
	return failure
}
func TestEgressLaunchOrder(t *testing.T) {
	runner, registry, engine, id := egressSessionFixture(t)
	if failure := launchEgress(t, runner, id); failure != "" {
		t.Fatalf("launch failed: %s", failure)
	}
	want := []string{"internal-network create", "egress-network create", "proxy create", "proxy verify", "proxy start", "proxy ready", "create", "start"}
	var got []string
	for _, event := range engine.Events {
		if slices.Contains(want, event) {
			got = append(got, event)
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("launch order = %v, want %v", got, want)
	}
	for _, event := range engine.Engine.Events {
		if strings.HasSuffix(event.Method, "CREATE") {
			r, failure := registry.Store.Codec.Decode([]byte(event.Intent))
			if failure != "" || r != event.Record {
				t.Error("create lacks durable intent", event)
			}
		}
	}
}
func TestEgressLaunchFailuresStartNoWorker(t *testing.T) {
	for _, tc := range []struct {
		fault, operation string
		outcome          port.Outcome
		removals         []string
		intent           int
	}{
		{"internal-network create", "network creation", port.Uncertain, nil, 0},
		{"egress-network create", "network creation", port.Uncertain, nil, 1},
		{"internal-network exact", "network inspection", port.IsolationUnverified, []string{"internal-network remove", "anchor remove"}, -1},
		{"internal-network occupied at create", "network inspection", port.IsolationUnverified, []string{"internal-network remove", "anchor remove"}, -1},
		{"egress-network occupied at create", "network inspection", port.IsolationUnverified, []string{"egress-network remove", "internal-network remove", "anchor remove"}, -1},
		{"proxy create", "proxy creation", port.Uncertain, nil, 2},
		{"proxy verify", "proxy verification", port.IsolationUnverified, []string{"proxy remove", "egress-network remove", "internal-network remove", "anchor remove"}, -1},
		{"proxy start", "proxy start", port.ExecutionFailed, []string{"proxy remove", "egress-network remove", "internal-network remove", "anchor remove"}, -1},
		{"proxy ready", "proxy readiness", port.ProxyFailed, []string{"proxy remove", "egress-network remove", "internal-network remove", "anchor remove"}, -1},
		{"proxy stopped", "proxy inspection", port.ProxyFailed, []string{"proxy remove", "egress-network remove", "internal-network remove", "anchor remove"}, -1},
		{"foreign endpoint", "network inspection", port.IsolationUnverified, []string{"proxy remove", "egress-network remove", "internal-network remove", "anchor remove"}, -1},
		{"internal-network foreign single endpoint", "network inspection", port.IsolationUnverified, []string{"proxy remove", "egress-network remove", "internal-network remove", "anchor remove"}, -1},
		{"egress-network foreign single endpoint", "network inspection", port.IsolationUnverified, []string{"proxy remove", "egress-network remove", "internal-network remove", "anchor remove"}, -1},
	} {
		t.Run(tc.fault, func(t *testing.T) {
			runner, registry, engine, id := egressSessionFixture(t)
			engine.EgressFault = tc.fault
			state := &startupState{}
			ctx := context.WithValue(t.Context(), startupKey{}, state)
			c, a, _, failure := runner.registeredLaunch(ctx, id, []string{"probe"}, port.Streams{})
			if c != nil {
				c.Close()
			}
			if a != nil {
				a.Close()
			}
			if failure != tc.outcome || state.operation != tc.operation || slices.Contains(engine.Events, "start") {
				t.Errorf("launch = %s at %q, want %s at %q, no worker start; %v", failure, state.operation, tc.outcome, tc.operation, engine.Events)
			}
			before, decode := registry.Store.Codec.Decode(registry.Store.Records[registry.Records[id].Intent])
			if decode != "" {
				t.Fatal(decode)
			}
			if strings.HasSuffix(tc.fault, " occupied at create") {
				if slices.Contains(engine.Events, "proxy create") || slices.Contains(engine.Events, "create") {
					t.Error("occupied new network permitted later resource creation", engine.Events)
				}
				// The foreign endpoint leaves before recovery. Its presence must
				// still have stopped launch after recording the owned network ID.
				for key, o := range engine.Networks {
					o.Endpoints, o.ContainerID = 0, ""
					engine.Networks[key] = o
				}
			}
			workerPresentAtRemoval := false
			engine.After = func(event string) {
				if strings.HasSuffix(event, " remove") {
					if _, present := engine.Resources[before.ContainerID]; present {
						workerPresentAtRemoval = true
					}
				}
			}
			result := (Recovery{Registry: registry, Engine: engine, Refusals: runner.Refusals}).Stop(t.Context(), id)
			saved, decode := registry.Store.Codec.Decode(registry.Store.Records[registry.Records[id].Intent])
			if decode != "" || len(result.Entries) != 1 {
				t.Fatalf("teardown lost record: %s, %#v", decode, result)
			}
			var removals []string
			for _, event := range engine.Events {
				if event == "remove" || strings.HasSuffix(event, " remove") {
					removals = append(removals, event)
				}
			}
			if !slices.Equal(removals, tc.removals) || workerPresentAtRemoval {
				t.Errorf("removals = %v, want %v after worker absence; worker present = %v", removals, tc.removals, workerPresentAtRemoval)
			}
			if tc.intent >= 0 {
				if len(result.Failures) != 1 || result.Failures[0].Outcome != port.Incomplete || result.Entries[0].Cleanup == "complete" || len(result.Entries[0].Retry) == 0 || saved.Completed() || saved.Egress != before.Egress || saved.Anchor != before.Anchor || egressResource(&saved, tc.intent).Stage != "intent" {
					t.Errorf("lost reply discarded intent or earlier ownership: %#v, %#v", result, saved)
				}
			} else if len(result.Failures) != 0 || result.Entries[0].Cleanup != "complete" || !saved.Completed() || len(engine.Resources) != 0 || len(engine.Networks) != 0 || len(engine.Anchors) != 0 {
				t.Errorf("launch failure left resources: %#v, %#v; %v", result, saved, engine.Events)
			}
		})
	}
}
func TestEgressTeardownOrder(t *testing.T) {
	runner, registry, engine, id, r := seedEgressSession(t, 8)
	result := (Recovery{Registry: registry, Engine: engine, Refusals: runner.Refusals}).Stop(t.Context(), id)
	if len(result.Failures) != 0 || len(result.Entries) != 1 || result.Entries[0].Cleanup != "complete" {
		t.Fatalf("teardown = %#v", result)
	}
	saved, _ := registry.Store.Codec.Decode(registry.Store.Records[registry.Records[id].Intent])
	if !saved.Completed() || len(engine.Resources) != 0 || len(engine.Networks) != 0 || len(engine.Anchors) != 0 || string(runner.Refusals.(*f.RefusalLog).Copies[id]) != f.ProxyStdout {
		t.Error("incomplete teardown", saved, engine.Events)
	}
	want := []string{"proxy SIGKILL", "proxy wait", "proxy copy", "proxy remove", "egress-network remove", "internal-network remove", "anchor remove"}
	var got []string
	for _, event := range engine.Events {
		if slices.Contains(want, event) {
			got = append(got, event)
		}
	}
	if !slices.Equal(got, want) {
		t.Error("teardown order", got, want, r.Egress.Proxy.ID)
	}
}
func TestEgressRecoveryAfterControllerLoss(t *testing.T) {
	runner, registry, engine, id, _ := seedEgressSession(t, 8)
	registry.Leases[id] = false
	result := (Recovery{Registry: registry, Engine: engine, Refusals: runner.Refusals}).Stop(t.Context(), id)
	if len(result.Failures) != 0 || len(result.Entries) != 1 || result.Entries[0].Controller != "absent" || result.Entries[0].Cleanup != "complete" || !slices.Contains(engine.Events, "SIGTERM") {
		t.Errorf("lost controller recovery = %#v; %v", result, engine.Events)
	}
}
func TestEgressCopyFailureRetainsProxy(t *testing.T) {
	for _, fault := range []string{"begin", "write", "commit", "nil"} {
		t.Run(fault, func(t *testing.T) {
			runner, registry, engine, id, r := seedEgressSession(t, 9)
			log := runner.Refusals.(*f.RefusalLog)
			log.Fault = fault
			recovery := Recovery{Registry: registry, Engine: engine, Refusals: log}
			want := port.Incomplete
			if fault == "nil" {
				recovery.Refusals = nil
				want = port.Internal
			}
			result := recovery.Stop(t.Context(), id)
			if len(result.Failures) != 1 || result.Failures[0].Outcome != want || len(engine.Networks) != 2 || len(engine.Anchors) != 1 || engine.Resources[r.Egress.Proxy.ID].Running || len(result.Entries[0].Retry) == 0 {
				t.Errorf("copy failure did not retain stopped proxy: %#v", result)
			}
			log.Fault = ""
			recovery.Refusals = log
			result = recovery.Stop(t.Context(), id)
			if len(result.Failures) != 0 || result.Entries[0].Cleanup != "complete" || string(log.Copies[id]) != f.ProxyStdout {
				t.Errorf("copy retry failed: %#v", result)
			}
		})
	}
}
func TestEgressCopyGapReleasesResources(t *testing.T) {
	runner, registry, engine, id, r := seedEgressSession(t, 9)
	log := runner.Refusals.(*f.RefusalLog)
	log.Report = port.RefusalReport{Outcome: port.RefusalLogGap, Gaps: []port.RefusalGap{{First: 2, Last: 2}}}
	engine.ProxyOutput = strings.Replace(f.ProxyStdout, `"seq":2`, `"seq":3`, 1)
	result := (Recovery{Registry: registry, Engine: engine, Refusals: log}).Stop(t.Context(), id)
	if len(result.Failures) != 1 || result.Failures[0].Outcome != port.RefusalLogGap || !reflect.DeepEqual(result.Failures[0].Gaps, log.Report.Gaps) || len(result.Entries) != 1 || result.Entries[0].Cleanup != "complete" || len(result.Entries[0].Retry) != 0 || len(engine.Networks) != 0 || len(engine.Resources) != 0 || len(log.Copies[id]) == 0 {
		t.Errorf("committed gap lost or retained resources: %#v %v", result, r.Egress)
	}
}
func TestEgressRecoveryRefusesForeignResources(t *testing.T) {
	for _, fault := range []string{"name", "label", "id", "socket"} {
		t.Run(fault, func(t *testing.T) {
			runner, registry, engine, id, r := seedEgressSession(t, 9)
			o := engine.Resources[r.Egress.Proxy.ID]
			switch fault {
			case "name":
				o.Name = "foreign"
			case "label":
				o.Kind = "foreign"
			case "id":
				o.ID = strings.Repeat("f", 64)
			case "socket":
				engine.Replaced = true
			}
			engine.Resources[r.Egress.Proxy.ID] = o
			result := (Recovery{Registry: registry, Engine: engine, Refusals: runner.Refusals}).Stop(t.Context(), id)
			if len(result.Failures) != 1 || result.Failures[0].Outcome != port.Refused || len(engine.Resources) != 1 || len(engine.Networks) != 2 || slices.Contains(engine.Events, "proxy remove") {
				t.Errorf("foreign resource changed: %#v %v", result, engine.Events)
			}
		})
	}
}
func TestEgressUncertainCreates(t *testing.T) {
	for _, found := range []bool{false, true} {
		runner, registry, engine, id, r := seedEgressSession(t, 1)
		if found {
			o := f.EgressNetwork(r, port.InternalNetwork)
			o.ID = strings.Repeat("a", 64)
			engine.Networks[o.ID] = o
		}
		result := (Recovery{Registry: registry, Engine: engine, Refusals: runner.Refusals}).Stop(t.Context(), id)
		if found {
			if len(result.Failures) != 0 || result.Entries[0].Cleanup != "complete" {
				t.Error("owned uncertain create not adopted", result)
			}
		} else {
			if len(result.Failures) != 1 || result.Failures[0].Outcome != port.Incomplete || len(engine.Anchors) != 1 {
				t.Error("missing intent incorrectly fenced", result)
			}
		}
	}
}
func TestEgressStatus(t *testing.T) {
	for _, stage := range []int{6, 8, 9, 13} {
		runner, registry, engine, id, _ := seedEgressSession(t, stage)
		before := registry.Writes
		if stage == 13 {
			r := registry.Records[id]
			r.State = "stopped"
			registry.Records[id] = r
			engine.Networks = nil
			engine.Anchors = nil
		}
		result := (Recovery{Registry: registry, Engine: engine, Refusals: runner.Refusals}).Status(t.Context())
		want := "pending"
		if stage == 13 {
			want = "complete"
		}
		if len(result.Failures) != 0 || len(result.Entries) != 1 || result.Entries[0].Cleanup != want || registry.Writes != before {
			t.Error("egress status", stage, result)
		}
	}
}
func TestEgressGrantRefusedBeforeResources(t *testing.T) {
	for _, fault := range []string{"empty names", "bad ID", "empty line", "nil"} {
		b, store, engine := f.Fixture()
		b = f.EgressBinding(b)
		switch fault {
		case "empty names":
			b.Egress.Names = ""
		case "bad ID":
			b.Egress.ImageID = "short"
		case "empty line":
			b.Egress.Names += "\n"
		}
		store.Codec.(*f.Codec).Value = b
		registry := &f.Registry{Store: store, Project: b.Frozen.Project}
		runEngine := &f.RunEngine{Engine: engine}
		runner := Runner{Store: store, Registry: registry, Engine: runEngine, Workspace: f.MountWorkspace(b), Entropy: f.Entropy{Value: strings.Repeat("1", 64)}, Refusals: &f.RefusalLog{}}
		want := port.Invalid
		if fault == "nil" {
			runner.Refusals = nil
			want = port.Internal
		}
		result := runner.Run(t.Context(), ap.Durable{Frozen: b.Frozen}, []string{"probe"}, port.Streams{})
		if result.Outcome != want || len(registry.Records) != 0 || len(runEngine.Events) != 0 {
			t.Error("grant reached resources", fault, result, runEngine.Events)
		}
	}
}
func TestUnregisteredRunRefusesEgress(t *testing.T) {
	b, store, engine := f.Fixture()
	b = f.EgressBinding(b)
	store.Codec.(*f.Codec).Value = b
	runEngine := &f.RunEngine{Engine: engine}
	runner := Runner{Store: store, Engine: runEngine, Workspace: f.MountWorkspace(b), Entropy: f.Entropy{Value: strings.Repeat("1", 64)}}
	result := runner.Run(t.Context(), ap.Durable{Frozen: b.Frozen}, []string{"probe"}, port.Streams{})
	if result.Outcome != port.Refused || len(runEngine.Events) != 0 {
		t.Error("unregistered egress reached engine", result, runEngine.Events)
	}
}
func TestOfflineRunCreatesNoEgress(t *testing.T) {
	runner, _, engine, _, id := registeredSessionFixture(t)
	if failure := launchEgress(t, runner, id); failure != "" {
		t.Fatal(failure)
	}
	want := []string{"open", "anchor allocate", "anchor inspect", "anchor inspect", "create", "verify", "attach", "verify", "anchor inspect", "start", "detach"}
	if !slices.Equal(engine.Events, want) {
		t.Error("offline events changed", engine.Events)
	}
}
