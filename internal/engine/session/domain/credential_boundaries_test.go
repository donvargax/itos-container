package domain

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestCredentialPreparationFailureNeverStarts(t *testing.T) {
	for _, fault := range []string{"source", "read", "write", "verify", "stop won"} {
		t.Run(fault, func(t *testing.T) {
			runner, registry, engine, id, fixture := credentialRunner(t)
			stager := fixture.Stager.(*f.CredentialStager)
			switch fault {
			case "source":
				fixture.Sources.Failure = port.CredentialRefused
			case "read", "write":
				stager.Fault = fault
			case "verify":
				engine.Boundary = port.IsolationUnverified
			case "stop won":
				registry.AfterClose = func(id string) {
					if registry.Records[id].State == "ready" {
						registry.AfterClose = nil
						r := registry.Records[id]
						r.State = "stopping"
						registry.Records[id] = r
					}
				}
			}
			c, a, _, fail := runner.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{})
			if c != nil {
				c.Close()
			}
			if a != nil {
				a.Close()
			}
			if fail == "" || slices.Contains(engine.Events, "start") {
				t.Fatal("pre-start refusal launched a worker")
			}
			stager.Fault = ""
			result := (Recovery{Registry: registry, Engine: engine, Credentials: stager}).Stop(t.Context(), id)
			if len(result.Failures) != 0 || result.Entries[0].Cleanup != "complete" || len(engine.Resources) != 0 || len(engine.Anchors) != 0 {
				t.Fatal("pre-start failure lost owned cleanup")
			}
		})
	}
}
func TestCredentialIntentPrecedesEveryEffect(t *testing.T) {
	runner, registry, _, id, fixture := credentialRunner(t)
	stager := fixture.Stager.(*f.CredentialStager)
	stager.Before = func(event string) {
		if event == "mkdir" || event == "copy" {
			r, fail := registry.Store.Codec.Decode(registry.Store.Records[registry.Records[id].Intent])
			if fail != "" || r.Schema != 4 || r.Credentials.Directory == "" || event == "copy" && r.Credentials.Files[0] == "" {
				t.Error("staging effect has no protected intent")
			}
		}
	}
	if fail := launchEgress(t, runner, id); fail != "" {
		t.Fatal(fail)
	}
}
func TestCredentialRecoveryPresenceCombinations(t *testing.T) {
	for _, worker := range []bool{false, true} {
		for _, paths := range []string{"present", "absent", "unknown"} {
			t.Run(map[bool]string{false: "worker absent", true: "worker present"}[worker]+"/"+paths, func(t *testing.T) {
				runner, registry, engine, id, fixture := credentialRunner(t)
				stager := fixture.Stager.(*f.CredentialStager)
				stager.Fault = "unlink"
				c, a, _, fail := runner.registeredLaunch(t.Context(), id, []string{"probe"}, port.Streams{})
				if c != nil {
					c.Close()
				}
				if a != nil {
					a.Close()
				}
				if fail == "" {
					t.Fatal("failed unlink was hidden")
				}
				record, decode := registry.Store.Codec.Decode(registry.Store.Records[registry.Records[id].Intent])
				if decode != "" {
					t.Fatal(decode)
				}
				if !worker {
					delete(engine.Resources, record.ContainerID)
				}
				stager.Fault = ""
				if paths == "absent" {
					delete(stager.Directories, record.Credentials.Directory)
					for _, mount := range record.Credentials.Mounts() {
						delete(stager.Files, mount.Source)
					}
				}
				if paths == "unknown" {
					stager.Fault = "observe"
				}
				recovery := Recovery{Registry: registry, Engine: engine, Credentials: stager}
				writes := registry.Writes
				status := recovery.Status(t.Context())
				if len(status.Entries) != 1 || status.Entries[0].Staging != paths || registry.Writes != writes {
					t.Fatal("status changed records or conflated staging with worker state")
				}
				result := recovery.Stop(t.Context(), id)
				if paths == "unknown" {
					if len(result.Failures) == 0 || result.Entries[0].Cleanup == "complete" || !slices.Equal(result.Entries[0].Retry, []string{"itos", "container", "stop", id}) {
						t.Fatal("unknown staging discarded stop retry")
					}
					stager.Fault = ""
					result = recovery.Stop(t.Context(), id)
				}
				if len(result.Failures) != 0 || result.Entries[0].Cleanup != "complete" || result.Entries[0].Staging != "absent" {
					t.Fatal("confirmed staging and worker removal did not complete")
				}
				if !worker && result.Entries[0].WorkloadExit != nil {
					t.Fatal("missing worker invented a workload result")
				}
				if again := recovery.Stop(t.Context(), id); len(again.Failures) != 0 || again.Entries[0].Cleanup != "complete" {
					t.Fatal("confirmed completion was not idempotent")
				}
			})
		}
	}
}
func TestCredentialLateOperationsRetainAuthority(t *testing.T) {
	for _, phase := range []string{"read", "copy", "unlink", "intent"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				runner, registry, engine, id, fixture := credentialRunner(t)
				stager := fixture.Stager.(*f.CredentialStager)
				runner.Registry = finiteRegistry{registry}
				ctx, end := budget.Start(t.Context(), 120*time.Second, "startup")
				defer end()
				entry, fail := runner.Registry.Open(ctx, id, false)
				if fail != "" {
					t.Fatal(fail)
				}
				c, fail := engine.OpenRun(ctx, entry.Intent().Binding())
				if fail != "" {
					t.Fatal(fail)
				}
				defer c.Close()
				if fail = runner.Workspace.Pin(entry.Intent().Binding()); fail != "" {
					t.Fatal(fail)
				}
				if fail = prepareAnchor(ctx, entry.Intent(), c, runner.Workspace, id); fail != "" {
					t.Fatal(fail)
				}
				if phase == "unlink" {
					if fail = runner.prepareCredentials(ctx, entry.Intent()); fail != "" {
						t.Fatal(fail)
					}
				}
				blocked := false
				release := make(chan struct{})
				if phase == "intent" {
					registry.Store.AfterSave = func() {
						if !blocked {
							blocked = true
							<-release
						}
					}
				} else {
					stager.Before = func(event string) {
						if event == phase && !blocked {
							blocked = true
							<-release
						}
					}
				}
				before := time.Now()
				if phase == "unlink" {
					fail = cleanupCredentials(ctx, entry.Intent(), stager)
				} else {
					fail = runner.prepareCredentials(ctx, entry.Intent())
				}
				if fail == "" || time.Since(before) != 10*time.Second {
					t.Fatal("finite staging operation exceeded its ten-second cap", fail, time.Since(before))
				}
				entry.Close()
				synctest.Wait()
				if !registry.Locks[id] {
					t.Fatal("timed-out staging released serialization while the syscall was pending")
				}
				if phase != "intent" && phase != "unlink" && fixture.Sources.Closes != 0 {
					t.Fatal("timed-out copy closed its source while reading")
				}
				time.Sleep(time.Second)
				close(release)
				synctest.Wait()
				if registry.Locks[id] || registry.Store.Locked || slices.Contains(engine.Events, "create") || slices.Contains(engine.Events, "start") {
					t.Fatal("late reply kept authority or resumed launch")
				}
				stager.Before = nil
				registry.Store.AfterSave = nil
				result := (Recovery{Registry: registry, Engine: engine, Credentials: stager}).Stop(context.Background(), id)
				if len(result.Failures) != 0 || result.Entries[0].Cleanup != "complete" {
					t.Fatal("fresh recovery did not reconcile late staging")
				}
			})
		})
	}
}
func TestCredentialRegisteredRunAndEgressCleanup(t *testing.T) {
	for _, egress := range []bool{false, true} {
		for _, lostAck := range []bool{false, true} {
			t.Run(map[bool]string{false: "offline", true: "egress"}[egress]+map[bool]string{false: "/ack", true: "/lost ack"}[lostAck], func(t *testing.T) {
				runner, d, registry, engine, _ := freshEgressRunner()
				codec := registry.Store.Codec.(*f.Codec)
				if !egress {
					codec.Value.Egress = port.EgressGrant{}
				}
				fixture := f.MemoryCredentialFixture(map[string][]byte{"claude-token": []byte("synthetic\n"), "github-token": []byte("synthetic-github\n")})
				codec.Value.Credentials = fixture.Sources.Kinds()
				runner.Sources, runner.Credentials, runner.StagingEntropy = fixture.Sources, fixture.Stager, credentialEntropy{}
				engine.CredentialStaging = fixture.Stager.(*f.CredentialStager)
				if lostAck {
					engine.StartFailure = port.ExecutionFailed
					if egress {
						engine.StartFailure = ""
						engine.After = func(event string) {
							if event == "create" {
								engine.StartFailure = port.ExecutionFailed
							}
						}
					}
				}
				result := runner.Run(t.Context(), d, []string{"probe"}, port.Streams{})
				if result.Cleanup != port.Complete || len(engine.Resources) != 0 || len(engine.Anchors) != 0 || len(engine.Networks) != 0 || len(engine.CredentialAcquired) != 0 {
					t.Fatal("credential cleanup completed with owned resources or references left", result.Outcome, result.Cleanup)
				}
				if lostAck && result.Exited {
					t.Fatal("lost start acknowledgement invented a workload exit")
				}
				if !lostAck && (!result.Exited || result.Exit != 17) {
					t.Fatal("known workload exit lost precedence")
				}
				if count := len(registry.Records); count != 1 {
					t.Fatal("registered run lost its public session")
				}
			})
		}
	}
}

func TestCredentialRegisteredRunTerminationGrace(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runner, durable, registry, engine, _ := freshEgressRunner()
		fixture := f.MemoryCredentialFixture(map[string][]byte{"claude-token": []byte("synthetic\n")})
		codec := registry.Store.Codec.(*f.Codec)
		codec.Value.Egress, codec.Value.Credentials = port.EgressGrant{}, fixture.Sources.Kinds()
		runner.Sources, runner.Credentials, runner.StagingEntropy = fixture.Sources, fixture.Stager, credentialEntropy{}
		engine.CredentialStaging = fixture.Stager.(*f.CredentialStager)
		engine.WaitUntilCancel, engine.IgnoreSignal = true, true
		ctx, cancel := context.WithCancelCause(t.Context())
		defer cancel(nil)
		var terminated, killed time.Time
		engine.After = func(event string) {
			switch event {
			case "wait":
				cancel(port.Interrupt{Signal: "SIGTERM"})
			case "SIGTERM":
				terminated = time.Now()
			case "SIGKILL":
				killed = time.Now()
			}
		}
		result := runner.Run(ctx, durable, []string{"probe"}, port.Streams{})
		if terminated.IsZero() || killed.IsZero() || killed.Sub(terminated) != 10*time.Second {
			t.Fatal("credential run did not escalate at the original ten-second grace", killed.Sub(terminated))
		}
		if result.Cleanup != port.Complete || len(engine.Resources) != 0 || len(engine.CredentialAcquired) != 0 || len(engine.Anchors) != 0 {
			t.Fatal("credential run did not remove all owned resources after escalation", result.Cleanup)
		}
	})
}
func TestCredentialUnregisteredRunRefusesBeforeEngine(t *testing.T) {
	b, store, e := f.Fixture()
	b.Credentials = port.CredentialKinds{Claude: true}
	store.Codec.(*f.Codec).Value = b
	engine := &f.RunEngine{Engine: e}
	sources := &f.CredentialSourceFixture{}
	runner := Runner{Store: store, Engine: engine, Entropy: f.Entropy{Value: strings.Repeat("1", 64)}, Workspace: f.MountWorkspace(b), Sources: sources}
	result := runner.Run(t.Context(), ap.Durable{Frozen: b.Frozen}, []string{"probe"}, port.Streams{})
	if result.Outcome != port.Refused || len(engine.Events) != 0 {
		t.Fatal("unregistered credential run reached engine")
	}
	if sources.Closes != 1 {
		t.Fatal("refused run retained its checked source capability")
	}
}
func TestCredentialControllerOutputAbsence(t *testing.T) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal("marker setup failed")
	}
	marker := []byte("itos-synthetic-credential-" + hex.EncodeToString(random[:]))
	runner, d, registry, engine, _ := freshEgressRunner()
	registry.Store.Codec.(*f.Codec).Value.Egress = port.EgressGrant{}
	fixture := f.MemoryCredentialFixture(map[string][]byte{"claude-token": marker})
	registry.Store.Codec.(*f.Codec).Value.Credentials = fixture.Sources.Kinds()
	runner.Sources, runner.Credentials, runner.StagingEntropy = fixture.Sources, fixture.Stager, credentialEntropy{}
	engine.CredentialStaging = fixture.Stager.(*f.CredentialStager)
	engine.AfterStart = func() {
		if !f.CredentialMarkerPresent(engine.CredentialAcquired, marker) {
			t.Error("acquired reference detector positive control failed")
		}
	}
	result := runner.Run(t.Context(), d, []string{"probe"}, port.Streams{})
	surfaces := []string{fmt.Sprintf("%#v", result), fmt.Sprintf("%#v", registry.Store.Events), fmt.Sprintf("%#v", engine.Engine.Events)}
	for _, surface := range surfaces {
		if f.CredentialMarkerPresent([]byte(surface), marker) {
			t.Fatal("credential marker in controller result or recorded metadata")
		}
	}
	if len(fixture.Stager.(*f.CredentialStager).Files) != 0 || len(engine.CredentialAcquired) != 0 {
		t.Fatal("confirmed cleanup retained credential paths or references")
	}
}
func TestCredentialPrimaryFailureSurvivesCleanupFailure(t *testing.T) {
	runner, d, registry, engine, _ := freshEgressRunner()
	registry.Store.Codec.(*f.Codec).Value.Egress = port.EgressGrant{}
	fixture := f.MemoryCredentialFixture(map[string][]byte{"claude-token": []byte("synthetic\n")})
	registry.Store.Codec.(*f.Codec).Value.Credentials = fixture.Sources.Kinds()
	stager := fixture.Stager.(*f.CredentialStager)
	stager.Fault = "unlink"
	runner.Credentials, runner.Sources, runner.StagingEntropy = stager, fixture.Sources, credentialEntropy{}
	engine.CredentialStaging = stager
	engine.StartFailure = port.ExecutionFailed
	result := runner.Run(t.Context(), d, []string{"probe"}, port.Streams{})
	if result.Outcome != port.ExecutionFailed || result.Cleanup == port.Complete || result.Exited || !port.HexID(result.SessionID) {
		t.Fatal("cleanup hid the primary failure or invented a workload outcome")
	}
	record, fail := registry.Store.Codec.Decode(registry.Store.Records[registry.Records[result.SessionID].Intent])
	if fail != "" || record.ContainerID != engine.CreateID || record.Completed() {
		t.Fatal("incomplete credential cleanup discarded original worker authority")
	}
	stager.Fault = ""
	retry := (Recovery{Registry: registry, Engine: engine, Credentials: stager}).Stop(t.Context(), result.SessionID)
	if len(retry.Failures) != 0 || retry.Entries[0].Cleanup != "complete" {
		t.Fatal("original session could not retry credential cleanup")
	}
}
