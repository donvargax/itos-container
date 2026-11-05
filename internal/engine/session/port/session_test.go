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
