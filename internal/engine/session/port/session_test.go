package port_test

import (
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestParseReference(t *testing.T) {
	project := strings.Repeat("a", 64)
	token := strings.Repeat("b", 64)
	reference := "intents/" + project + "/" + token + ".json"
	t.Run("valid reference preserves both identities", func(t *testing.T) {
		gotProject, gotToken, ok := port.ParseReference(reference)
		if !ok || gotProject != project || gotToken != token {
			t.Fatalf("ParseReference = (%q, %q, %v), want (%q, %q, true)", gotProject, gotToken, ok, project, token)
		}
	})
	for _, tc := range []struct {
		name, reference string
	}{
		{"empty", ""},
		{"traversal", "../" + token + ".json"},
		{"absolute path", "/" + reference},
		{"different namespace", "snapshots/" + project + "/" + token + ".json"},
		{"missing suffix", "intents/" + project + "/" + token},
		{"temporary file", reference + ".tmp"},
		{"extra component", "intents/" + project + "/nested/" + token + ".json"},
		{"empty project", "intents//" + token + ".json"},
		{"short project", "intents/" + project[:63] + "/" + token + ".json"},
		{"uppercase project", "intents/" + strings.ToUpper(project) + "/" + token + ".json"},
		{"empty token", "intents/" + project + "/.json"},
		{"short token", "intents/" + project + "/" + token[:63] + ".json"},
		{"long token", "intents/" + project + "/" + token + "b.json"},
		{"nonhex token", "intents/" + project + "/" + strings.Repeat("g", 64) + ".json"},
		{"uppercase token", "intents/" + project + "/" + strings.ToUpper(token) + ".json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, ok := port.ParseReference(tc.reference); ok {
				t.Fatalf("accepted invalid intent reference %q", tc.reference)
			}
		})
	}
}

func TestRecordNameToken(t *testing.T) {
	token := strings.Repeat("b", 64)
	for _, tc := range []struct {
		name, worker, want string
	}{
		{"owned name", "itos-container-" + token, token},
		{"empty name", "", ""},
		{"bare token", token, ""},
		{"different prefix", "foreign-" + token, ""},
		{"embedded prefix", "foreign-itos-container-" + token, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := port.Record{Name: tc.worker}
			if got := record.NameToken(); got != tc.want {
				t.Fatalf("NameToken(%q) = %q, want %q", tc.worker, got, tc.want)
			}
		})
	}
}

func TestRecordCompletionRequiresTerminalEvidence(t *testing.T) {
	id := strings.Repeat("a", 64)
	for _, tc := range []struct {
		name, stage, id string
		complete        bool
	}{
		{"reserved before create", "reserved", "", false},
		{"create reply unknown", "intent", "", false},
		{"owned worker remains", "owned", id, false},
		{"confirmed full-ID absence", "absent", id, true},
		{"absence without identity", "absent", "", false},
		{"absence with short identity", "absent", id[:12], false},
		{"proven pre-create cancellation", "cancelled", "", true},
		{"cancellation with a worker", "cancelled", id, false},
		{"unknown stage with identity", "unknown", id, false},
		{"empty record", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := port.Record{Schema: 1, Stage: tc.stage, ContainerID: tc.id}
			if got := r.Completed(); got != tc.complete {
				t.Fatalf("completion = %v, want %v for %#v", got, tc.complete, r)
			}
		})
	}
}

func TestRecordTransitionPreservesOriginalAuthority(t *testing.T) {
	current := port.Record{
		Schema: 1, Project: strings.Repeat("a", 64), Generation: strings.Repeat("b", 64),
		Fingerprint: strings.Repeat("c", 64), Snapshot: "snapshots/original.json",
		UID: "1000", GID: "1000", Endpoint: "unix:///original.sock", EngineID: "original-engine",
		Name: "itos-container-" + strings.Repeat("d", 64), ImageReference: "original-image",
		ImageID: "sha256:" + strings.Repeat("e", 64), ContainerID: strings.Repeat("f", 64), Stage: "owned",
	}
	completed := current
	completed.Stage = "absent"
	if !current.ValidTransition(completed) {
		t.Fatal("refused completion preserving original authority")
	}
	for name, change := range map[string]func(*port.Record){
		"schema":          func(r *port.Record) { r.Schema++ },
		"project":         func(r *port.Record) { r.Project = strings.Repeat("1", 64) },
		"generation":      func(r *port.Record) { r.Generation = strings.Repeat("2", 64) },
		"fingerprint":     func(r *port.Record) { r.Fingerprint = strings.Repeat("3", 64) },
		"snapshot":        func(r *port.Record) { r.Snapshot = "snapshots/other.json" },
		"user":            func(r *port.Record) { r.UID = "1001" },
		"group":           func(r *port.Record) { r.GID = "1001" },
		"endpoint":        func(r *port.Record) { r.Endpoint = "unix:///other.sock" },
		"engine":          func(r *port.Record) { r.EngineID = "other-engine" },
		"name":            func(r *port.Record) { r.Name = "itos-container-" + strings.Repeat("4", 64) },
		"image reference": func(r *port.Record) { r.ImageReference = "other-image" },
		"image identity":  func(r *port.Record) { r.ImageID = "sha256:" + strings.Repeat("5", 64) },
		"worker changed":  func(r *port.Record) { r.ContainerID = strings.Repeat("6", 64) },
		"worker cleared":  func(r *port.Record) { r.ContainerID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			next := completed
			change(&next)
			if current.ValidTransition(next) {
				t.Fatal("completion accepted substituted or lost authority", next)
			}
		})
	}
}
