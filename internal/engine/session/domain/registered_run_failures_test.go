package domain_test

import (
	"reflect"
	"strings"
	"testing"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestRegisteredRunRequiresBothIndependentTokensBeforePublication(t *testing.T) {
	id, token := strings.Repeat("1", 64), strings.Repeat("2", 64)
	for _, tc := range []struct {
		name   string
		values []f.Entropy
		left   int
	}{
		{"public entropy unavailable", []f.Entropy{{Value: id, Failure: port.IdentityFailed}, {Value: token}}, 1},
		{"invalid public ID", []f.Entropy{{Value: strings.Repeat("A", 64)}, {Value: token}}, 1},
		{"operation entropy exhausted", []f.Entropy{{Value: id}}, 0},
		{"operation entropy unavailable", []f.Entropy{{Value: id}, {Value: token, Failure: port.IdentityFailed}, {Value: token}}, 1},
		{"invalid operation token", []f.Entropy{{Value: id}, {Value: strings.Repeat("2", 63)}, {Value: token}}, 1},
		{"valid independent tokens", []f.Entropy{{Value: id}, {Value: token}}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, store, engine := f.Fixture()
			registry := &f.Registry{Store: store, Project: b.Frozen.Project}
			entropy := &f.EntropySequence{Values: tc.values}
			runEngine := &f.RunEngine{Engine: engine, Exit: 7}
			runner := domain.Runner{Registry: registry, Store: store, Engine: runEngine, Entropy: entropy, Workspace: &f.Workspace{}}

			result := runner.Run(t.Context(), ap.Durable{Frozen: b.Frozen}, []string{"probe"}, port.Streams{})
			if len(entropy.Values) != tc.left {
				t.Fatal("run drew a replacement identity after entropy refusal", entropy.Values)
			}
			if tc.name == "valid independent tokens" {
				if result.SessionID != id || result.Reference != port.Reference(b.Frozen.Project, token) || result.Outcome != "" || !result.Exited || result.Exit != 7 || result.Cleanup != port.Complete {
					t.Fatal("run lost independent registration or workload result", result)
				}
				if registry.Records[id].Intent != result.Reference || registry.Leases[id] || registry.Locks[id] || len(engine.Resources) != 0 {
					t.Fatal("completed run lost its intent binding or retained owned resources")
				}
				return
			}
			if result != (port.RunResult{Outcome: port.RegistrationInternal}) {
				t.Fatal("entropy refusal exposed a retry or invented a workload result", result)
			}
			if len(registry.Records) != 0 || len(registry.Leases) != 0 || registry.Writes != 0 || len(store.Events) != 0 || len(store.Records) != 0 || len(engine.Events) != 0 || len(runEngine.Events) != 0 {
				t.Fatal("entropy refusal reached publication or the engine")
			}
		})
	}
}

func TestRegisteredRunRetainsRegistrationWhenIntentPublicationFails(t *testing.T) {
	for _, fault := range []string{"intent file fsync", "intent parent fsync"} {
		t.Run(fault, func(t *testing.T) {
			b, store, engine := f.Fixture()
			id, token := strings.Repeat("1", 64), strings.Repeat("2", 64)
			registry := &f.Registry{Store: store, Project: b.Frozen.Project}
			store.Fault = fault
			runEngine := &f.RunEngine{Engine: engine}
			runner := domain.Runner{Registry: registry, Store: store, Engine: runEngine, Entropy: &f.EntropySequence{Values: []f.Entropy{{Value: id}, {Value: token}}}, Workspace: &f.Workspace{}}

			result := runner.Run(t.Context(), ap.Durable{Frozen: b.Frozen}, []string{"probe"}, port.Streams{})
			if result != (port.RunResult{Outcome: port.IntentWrite}) {
				t.Fatal("failed intent publication exposed a retry or claimed workload progress", result)
			}
			ref := port.Reference(b.Frozen.Project, token)
			want := port.PublicRecord{Schema: 1, ID: id, Project: b.Frozen.Project, Intent: ref, State: "preparing"}
			if !reflect.DeepEqual(registry.Records, map[string]port.PublicRecord{id: want}) || registry.Writes != 1 || registry.Leases[id] || registry.Locks[id] || store.Locked {
				t.Fatal("intent publication failure lost registration or retained a lease", registry.Records)
			}
			if len(engine.Events) != 0 || len(runEngine.Events) != 0 || len(engine.Resources) != 0 {
				t.Fatal("undurable intent authorized engine access")
			}
			if fault == "intent parent fsync" {
				i, failure := store.Codec.Decode(store.Records[ref])
				if failure != "" || i != b.Intent(token) {
					t.Fatal("uncertain publication discarded the visible original intent", i, failure)
				}
			} else if len(store.Records) != 0 {
				t.Fatal("failed file fsync published an intent")
			}
		})
	}
}
