package domain

import (
	"context"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

type credentialIdentityReply struct {
	token   string
	failure port.Outcome
}

func TestCredentialPreparationCancelledPublication(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runner, _, engine, entry, fixture := credentialPreparationFixture(t)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		stager := fixture.Stager.(*f.CredentialStager)
		stager.Before = func(event string) {
			if event == "mkdir" {
				cancel()
			}
		}
		if failure := runner.prepareCredentials(ctx, entry.Intent()); failure == "" {
			t.Fatal("cancelled identity publication allowed a copy")
		}
		synctest.Wait()
		if entry.Intent().Record().Credentials.Stage != "intent" || len(stager.Directories) != 0 || slices.Contains(engine.Events, "create") {
			t.Fatal("cancelled publication advanced launch or lost unpublished cleanup")
		}
	})
}

func TestCredentialSourcesWithoutGrantRefuse(t *testing.T) {
	runner, registry, engine, _, id := registeredSessionFixture(t)
	entry, failure := registry.Open(t.Context(), id, false)
	if failure != "" {
		t.Fatal(failure)
	}
	defer entry.Close()
	runner.Sources = &f.CredentialSourceFixture{}
	if failure := runner.prepareCredentials(t.Context(), entry.Intent()); failure != port.CredentialRefused || len(engine.Events) != 0 {
		t.Fatal("ungranted source capability reached provisioning", failure)
	}
}

func TestCredentialCallRetainsStartupFailure(t *testing.T) {
	_, _, _, entry, _ := credentialPreparationFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	state := &startupState{}
	ctx = context.WithValue(ctx, startupKey{}, state)
	failure := credentialCall(ctx, entry.Intent(), "cancelled credential publication", func(context.Context, port.Intent) port.Outcome {
		cancel()
		return ""
	})
	if failure == "" || failure != state.failure || state.operation != "cancelled credential publication" || budget.Cause(ctx) == nil {
		t.Fatal("credential operation lost its startup cancellation cause")
	}
}

func TestCredentialCreateWithoutPreparationRefuses(t *testing.T) {
	runner, registry, engine, sessionID, _ := credentialRunner(t)
	entry, failure := registry.Open(t.Context(), sessionID, false)
	if failure != "" {
		t.Fatal(failure)
	}
	defer entry.Close()
	if failure := runner.Workspace.Pin(entry.Intent().Binding()); failure != "" {
		t.Fatal(failure)
	}
	c, failure := engine.OpenRun(t.Context(), entry.Intent().Binding())
	if failure != "" {
		t.Fatal(failure)
	}
	defer c.Close()
	if id, failure := createRegistered(t.Context(), entry, c, runner.Workspace, []string{"probe"}); id != "" || failure != port.CredentialRefused {
		t.Fatal("registered create without credential preparation was admitted", failure)
	}
	if slices.Contains(engine.Events, "create") || slices.Contains(engine.Events, "start") {
		t.Fatal("missing preparation caused worker effects")
	}
}

func TestCredentialCreateRequiresUncancelledIntent(t *testing.T) {
	for _, stage := range []string{"intent", "cancelled"} {
		t.Run(stage, func(t *testing.T) {
			runner, _, engine, entry, _ := credentialPreparationFixture(t)
			if failure := runner.prepareCredentials(t.Context(), entry.Intent()); failure != "" {
				t.Fatal("credential preparation positive control failed", failure)
			}
			r := entry.Intent().Record()
			r.Stage = stage
			if failure := entry.Intent().Save(r); failure != "" {
				t.Fatal("could not publish worker stage", failure)
			}
			c, failure := engine.OpenRun(t.Context(), entry.Intent().Binding())
			if failure != "" {
				t.Fatal(failure)
			}
			defer c.Close()
			id, failure := createRegistered(t.Context(), entry, c, runner.Workspace, []string{"probe"})
			if stage == "cancelled" {
				if failure != port.Refused || id != "" || slices.Contains(engine.Events, "create") || entry.Intent().Record() != r {
					t.Fatal("cancelled credential intent reached worker creation", failure)
				}
			} else if failure != "" || id == "" || !slices.Contains(engine.Events, "create") {
				t.Fatal("prepared credential intent could not create a worker", failure)
			}
			if slices.Contains(engine.Events, "start") {
				t.Fatal("worker creation started the workload")
			}
		})
	}
}

func TestCredentialStopFinalObservationFailure(t *testing.T) {
	runner, registry, engine, id, fixture := credentialRunner(t)
	if failure := launchEgress(t, runner, id); failure != "" {
		t.Fatal("credential launch positive control failed", failure)
	}
	stager := fixture.Stager.(*f.CredentialStager)
	observation := credentialObserved{stager, func() { stager.Fault = "observe" }}
	result := (Recovery{Registry: registry, Engine: engine, Credentials: observation}).Stop(t.Context(), id)
	if len(result.Failures) != 1 || result.Failures[0].Outcome != port.CredentialUnavailable || result.Entries[0].Cleanup != "unknown" || result.Entries[0].Staging != "unknown" {
		t.Fatal("final staging observation failure was hidden")
	}
	if !slices.Equal(result.Entries[0].Retry, []string{"itos", "container", "stop", id}) {
		t.Fatal("failed final observation lost the original stop retry")
	}
}

func (r credentialIdentityReply) StagingToken() (string, port.Outcome) { return r.token, r.failure }

func credentialPreparationFixture(t *testing.T) (Runner, *f.Registry, *f.RunEngine, port.Session, f.CredentialFixture) {
	t.Helper()
	runner, registry, engine, id, fixture := credentialRunner(t)
	entry, failure := registry.Open(t.Context(), id, false)
	if failure != "" {
		t.Fatal(failure)
	}
	t.Cleanup(entry.Close)
	c, failure := engine.OpenRun(t.Context(), entry.Intent().Binding())
	if failure != "" {
		t.Fatal(failure)
	}
	defer c.Close()
	if failure = runner.Workspace.Pin(entry.Intent().Binding()); failure != "" {
		t.Fatal(failure)
	}
	if failure = prepareAnchor(t.Context(), entry.Intent(), c, runner.Workspace, id); failure != "" {
		t.Fatal(failure)
	}
	return runner, registry, engine, entry, fixture
}

func TestCredentialPreparationMissingAuthority(t *testing.T) {
	for _, fault := range []string{"stager", "sources", "entropy", "kinds", "identity failure", "identity shape", "cancelled identity", "intent write", "source owned elsewhere"} {
		t.Run(fault, func(t *testing.T) {
			runner, registry, engine, entry, fixture := credentialPreparationFixture(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			want := port.CredentialRefused
			switch fault {
			case "stager":
				runner.Credentials = nil
			case "sources":
				runner.Sources = nil
			case "entropy":
				runner.StagingEntropy = nil
			case "kinds":
				fixture.Sources.Entries[0].Kind = "github-token"
			case "identity failure":
				runner.StagingEntropy = credentialIdentityReply{failure: port.IdentityFailed}
				want = port.IdentityFailed
			case "identity shape":
				runner.StagingEntropy = credentialIdentityReply{token: strings.Repeat("G", 32)}
				want = port.IdentityFailed
			case "cancelled identity":
				cancel()
				want = port.IdentityFailed
			case "intent write":
				registry.Store.Fault = "reserved-record write"
				want = port.IntentWrite
			case "source owned elsewhere":
				runner.sourceOwner = make(chan struct{}, 1)
				want = port.Incomplete
			}
			if failure := runner.prepareCredentials(ctx, entry.Intent()); failure != want {
				t.Fatal("missing credential authority was admitted", failure)
			}
			if slices.Contains(engine.Events, "create") || slices.Contains(engine.Events, "start") || len(fixture.Stager.(*f.CredentialStager).Directories) != 0 {
				t.Fatal("credential authority refusal caused provisioning effects")
			}
		})
	}
}

func TestCredentialStagingIdentity(t *testing.T) {
	valid := "0123456789abcdef0123456789abcdef"
	for _, tc := range []struct {
		name  string
		reply credentialIdentityReply
		want  port.Outcome
	}{
		{"valid", credentialIdentityReply{token: valid}, ""},
		{"failure with valid token", credentialIdentityReply{token: valid, failure: port.IdentityFailed}, port.IdentityFailed},
		{"empty", credentialIdentityReply{}, port.IdentityFailed},
		{"short hexadecimal", credentialIdentityReply{token: valid[:31]}, port.IdentityFailed},
		{"long hexadecimal", credentialIdentityReply{token: valid + "0"}, port.IdentityFailed},
		{"uppercase", credentialIdentityReply{token: strings.ToUpper(valid)}, port.IdentityFailed},
		{"non hexadecimal", credentialIdentityReply{token: valid[:31] + "g"}, port.IdentityFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner, _, engine, entry, fixture := credentialPreparationFixture(t)
			runner.StagingEntropy = tc.reply
			before := entry.Intent().Record()
			if failure := runner.prepareCredentials(t.Context(), entry.Intent()); failure != tc.want {
				t.Fatalf("staging identity result = %q, want %q", failure, tc.want)
			}
			stager := fixture.Stager.(*f.CredentialStager)
			after := entry.Intent().Record()
			if tc.want != "" {
				if after.Schema != before.Schema || after.Credentials != before.Credentials || len(stager.Events) != 0 {
					t.Fatal("refused staging identity published intent or reached the stager")
				}
			} else if after.Credentials.Stage != "owned" || after.Credentials.Directory != "/dev/shm/itos-container-"+valid || !fixture.Present(after.Credentials.Directory) {
				t.Fatal("valid staging identity did not stage credentials at the intended path")
			}
			if slices.Contains(engine.Events, "create") || slices.Contains(engine.Events, "start") {
				t.Fatal("credential preparation launched a worker")
			}
		})
	}
}

type credentialObserved struct {
	port.CredentialStager
	after func()
}

func (s credentialObserved) Observe(ctx context.Context, r port.Record) (string, port.Outcome) {
	presence, failure := s.CredentialStager.Observe(ctx, r)
	s.after()
	return presence, failure
}

func TestCredentialCleanupRequiresFreshAbsence(t *testing.T) {
	for _, fault := range []string{"healthy", "missing stager", "invalid record", "observation unavailable", "paths still present", "cancelled observation", "completion write"} {
		t.Run(fault, func(t *testing.T) {
			runner, registry, _, entry, fixture := credentialPreparationFixture(t)
			if failure := runner.prepareCredentials(t.Context(), entry.Intent()); failure != "" {
				t.Fatal("staging positive control failed", failure)
			}
			original := entry.Intent().Record()
			stager := fixture.Stager.(*f.CredentialStager)
			var adapter port.CredentialStager = stager
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			want := port.Outcome("")
			switch fault {
			case "missing stager":
				adapter, want = nil, port.Internal
			case "invalid record":
				registry.Store.UnsafeRecord = true
				want = port.Unsafe
			case "observation unavailable", "paths still present":
				stager.Before = func(event string) {
					if event == "observe" && len(stager.Directories) == 0 {
						if fault == "paths still present" {
							stager.Directories[original.Credentials.Directory] = original.Credentials
						} else {
							stager.Fault = "observe"
						}
					}
				}
				want = port.Incomplete
				if fault == "observation unavailable" {
					want = port.CredentialUnavailable
				}
			case "cancelled observation":
				adapter = credentialObserved{stager, cancel}
				want = port.Incomplete
			case "completion write":
				registry.Store.Fault = "reserved-record write"
				want = port.IntentWrite
			}
			if failure := cleanupCredentials(ctx, entry.Intent(), adapter); failure != want {
				t.Fatal("cleanup lost its observation or publication failure", failure)
			}
			if want != "" && entry.Intent().Record().Credentials.Stage != "owned" {
				t.Fatal("unconfirmed cleanup published staging absence")
			}
		})
	}
}

func TestCredentialPresenceFailuresRemainUnknown(t *testing.T) {
	for _, fault := range []string{"missing stager", "invalid record", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			runner, registry, _, entry, fixture := credentialPreparationFixture(t)
			if failure := runner.prepareCredentials(t.Context(), entry.Intent()); failure != "" {
				t.Fatal(failure)
			}
			recovery := Recovery{Credentials: fixture.Stager}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			want := port.Internal
			switch fault {
			case "missing stager":
				recovery.Credentials = nil
			case "invalid record":
				registry.Store.UnsafeRecord = true
				want = port.Unsafe
			case "cancelled":
				cancel()
				want = port.Incomplete
			}
			var observation port.RecoveryEntry
			if failure := recovery.credentialPresence(ctx, entry, &observation); failure != want || observation.Staging != "unknown" {
				t.Fatal("failed observation invented staging absence", failure, observation.Staging)
			}
		})
	}
}

func TestCredentialQueuedCallCancellation(t *testing.T) {
	for _, handoff := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			gate := make(chan struct{}, 1)
			i := &finiteIntent{gate: gate}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan struct{})
			go func() {
				defer close(done)
				_, _ = credentialHeld(ctx, i, "queued credential call", func(context.Context, port.Intent) bool { return true })
			}()
			synctest.Wait()
			if handoff {
				gate <- struct{}{}
			}
			cancel()
			<-done
			synctest.Wait()
			if handoff && len(gate) != 1 {
				t.Fatal("cancelled queued operation lost serialization authority")
			}
		})
	}
}
