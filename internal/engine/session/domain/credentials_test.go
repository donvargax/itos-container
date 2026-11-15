package domain

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"slices"
	"strings"
	"testing"
)

type credentialEntropy struct{}

func (credentialEntropy) StagingToken() (string, port.Outcome) { return strings.Repeat("a", 32), "" }
func credentialRunner(t *testing.T) (Runner, *f.Registry, *f.RunEngine, string, f.CredentialFixture) {
	runner, registry, engine, _, id := registeredSessionFixture(t)
	fixture := f.MemoryCredentialFixture(map[string][]byte{"claude-token": []byte("synthetic\n")})
	registry.Store.Codec.(*f.Codec).Value.Credentials = fixture.Sources.Kinds()
	runner.Credentials, runner.Sources, runner.StagingEntropy = fixture.Stager, fixture.Sources, credentialEntropy{}
	engine.CredentialStaging = fixture.Stager.(*f.CredentialStager)
	return runner, registry, engine, id, fixture
}
func TestCredentialLifecycle(t *testing.T) {
	runner, registry, engine, id, fixture := credentialRunner(t)
	engine.After = func(event string) {
		if event == "create" || event == "start" {
			if !fixture.Present(f.CredentialIntent(fixture.Sources.Kinds()).Credentials.Directory) {
				t.Error("worker effect preceded staged copies")
			}
		}
	}
	c, a, _, fail := runner.registeredLaunch(t.Context(), id, []string{"probe", "literal;$x"}, port.Streams{})
	if c != nil {
		defer c.Close()
	}
	if a != nil {
		defer a.Close()
	}
	if fail != "" {
		t.Fatal("credential launch refused", fail)
	}
	r, decode := registry.Store.Codec.Decode(registry.Store.Records[registry.Records[id].Intent])
	if decode != "" || r.Schema != 4 || r.Credentials.Stage != "absent" || fixture.Present(r.Credentials.Directory) || !slices.Contains(engine.Events, "start") {
		t.Fatal("acknowledged startup did not remove staging")
	}
	result := (Recovery{Registry: registry, Engine: engine, Credentials: fixture.Stager}).Stop(t.Context(), id)
	if len(result.Failures) != 0 || result.Entries[0].Cleanup != "complete" {
		t.Fatal("credential recovery did not complete")
	}
}
func TestCredentialBudgets(t *testing.T) {
	runner, _, engine, id, fixture := credentialRunner(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stage := fixture.Stager.(*f.CredentialStager)
	stage.Before = func(event string) {
		if event == "copy" {
			cancel()
		}
	}
	c, a, _, fail := runner.registeredLaunch(ctx, id, []string{"probe"}, port.Streams{})
	if c != nil {
		c.Close()
	}
	if a != nil {
		a.Close()
	}
	if fail == "" || slices.Contains(engine.Events, "create") || slices.Contains(engine.Events, "start") {
		t.Fatal("cancelled credential operation resumed launch")
	}
}
