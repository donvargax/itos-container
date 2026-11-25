package canonical

import (
	"encoding/json"
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	policyport "github.com/donvargax/itos-container/internal/engine/policy/port"
	"strings"
	"testing"
)

func TestHomeApprovalProfiles(t *testing.T) {
	for _, mode := range []string{"offline", "allowlist"} {
		for _, credentials := range []bool{false, true} {
			r := porttest.Ready()
			r.Policy.Policy.Version = 3
			r.Policy.Policy.Network.Mode = mode
			if mode == "allowlist" {
				r.Policy.Policy.Network.Allow = []string{"api.example.com"}
			}
			if credentials {
				r.Policy.Policy.Credentials = []policyport.Credential{{Kind: "github-token", Source: "/private/token"}}
			}
			codec := JSON{}
			before := codec.Candidate(r, "2001", "3001")
			r.Policy.Policy.Home = &policyport.Home{Storage: "volume"}
			got := codec.Candidate(r, "2001", "3001")
			var plain, home map[string]any
			_ = json.Unmarshal([]byte(before.Canonical), &plain)
			_ = json.Unmarshal([]byte(got.Canonical), &home)
			want := strings.TrimSuffix(plain["profile"].(string), "-v1") + "-home-v1"
			if home["profile"] != want || home["home"].(map[string]any)["mode"] != "0700" || got.Project != before.Project || got.Fingerprint == before.Fingerprint {
				t.Fatal(before, got)
			}
			record := port.Record{Candidate: got, Generation: "2"}
			if decoded, f := codec.Decode(before, codec.Encode(record)); f != "" || decoded != record {
				t.Fatal(decoded, f)
			}
			delete(home, "home")
			home["profile"] = plain["profile"]
			data, _ := json.Marshal(home)
			if string(data) != before.Canonical {
				t.Fatal("home changed legacy settings", string(data))
			}
		}
	}
}

func TestInvalidHomeSettingsCannotAuthorize(t *testing.T) {
	for _, change := range []func(*settings){
		func(s *settings) { s.Profile = "offline-home-v1" },
		func(s *settings) { h := volumeHome(); s.Home = &h },
		func(s *settings) { h := volumeHome(); h.Storage = "tmpfs"; s.Home = &h; s.Profile = "offline-home-v1" },
		func(s *settings) {
			h := volumeHome()
			h.Target = "/outside"
			s.Home = &h
			s.Profile = "offline-home-v1"
		},
		func(s *settings) {
			h := volumeHome()
			h.Lifetime = "session"
			s.Home = &h
			s.Profile = "offline-home-v1"
		},
		func(s *settings) { h := volumeHome(); h.Mode = "0777"; s.Home = &h; s.Profile = "offline-home-v1" },
		func(s *settings) { h := volumeHome(); s.Home = &h; s.Profile = "other-home-v1" },
	} {
		testRejectsSelfConsistentInvalidSettings(t, change)
	}
}

func TestOpenCodeHarnessIsBoundByProfile(t *testing.T) {
	r := porttest.Ready()
	codec := JSON{}
	r.Policy.Policy.Home = &policyport.Home{Storage: "volume"}
	plain := codec.Candidate(r, "2001", "3001")
	r.Policy.Policy.Home = &policyport.Home{Storage: "volume", Harnesses: []string{"opencode"}}
	got := codec.Candidate(r, "2001", "3001")
	if got.Fingerprint == plain.Fingerprint || got.Project != plain.Project {
		t.Fatal("the harness must change the fingerprint and keep the project", plain, got)
	}
	var settings map[string]any
	_ = json.Unmarshal([]byte(got.Canonical), &settings)
	if settings["profile"] != "offline-home-v1" {
		t.Fatal("profile names stay as slice-34 named them", settings["profile"])
	}
	want := `"harnesses":[{"name":"opencode","profile":"opencode-v1"}]`
	if !strings.Contains(got.Canonical, want) {
		t.Fatal("the home object omits the harness binding", got.Canonical)
	}
	record := port.Record{Candidate: got, Generation: "2"}
	if decoded, f := codec.Decode(plain, codec.Encode(record)); f != "" || decoded != record {
		t.Fatal(decoded, f)
	}
	frozen, f := codec.FrozenRecord(port.Frozen{Project: got.Project, Generation: "2", Fingerprint: got.Fingerprint, Settings: got.Canonical})
	if f != "" || frozen != record {
		t.Fatal("the frozen settings must keep the harness", f)
	}
	paths := codec.ChangedPaths(got, port.Record{Candidate: plain, Generation: "1"}, true)
	if len(paths) != 1 || paths[0] != "/home/harnesses" {
		t.Fatal(paths)
	}
}

func TestInvalidHarnessSettingsCannotAuthorize(t *testing.T) {
	for _, harnesses := range [][]harness{
		{{"codex", "codex-v1"}},
		{{"opencode", "opencode-v2"}},
		{{"opencode", "opencode-v1"}, {"opencode", "opencode-v1"}},
		{{"OpenCode", "opencode-v1"}},
	} {
		testRejectsSelfConsistentInvalidSettings(t, func(s *settings) {
			h := volumeHome()
			h.Harnesses = harnesses
			s.Home = &h
			s.Profile = "offline-home-v1"
		})
	}
}

func TestCodexHarnessIsBoundByItsSeed(t *testing.T) {
	r := porttest.Ready()
	codec := JSON{}
	r.Policy.Policy.Home = &policyport.Home{Storage: "volume"}
	plain := codec.Candidate(r, "2001", "3001")
	r.Policy.Policy.Home = &policyport.Home{Storage: "volume", Harness: "codex"}
	got := codec.Candidate(r, "2001", "3001")
	if got.Fingerprint == plain.Fingerprint || got.Project != plain.Project {
		t.Fatal("the harness must change the fingerprint and keep the project", plain, got)
	}
	want := `"home":{"harness":{"kind":"codex","seed":"codex-v1","target":"/home/itos/.codex"},"lifetime":"project","mode":"0700","storage":"volume","target":"/home/itos"}`
	if !strings.Contains(got.Canonical, want) || !strings.Contains(got.Canonical, `"profile":"offline-home-v1"`) {
		t.Fatal("the home object omits the Codex binding", got.Canonical)
	}
	// Beside the OpenCode list the harness sits before it, as the sorted keys put it.
	r.Policy.Policy.Home = &policyport.Home{Storage: "volume", Harness: "codex", Harnesses: []string{"opencode"}}
	both := codec.Candidate(r, "2001", "3001")
	if !strings.Contains(both.Canonical, `"harness":{"kind":"codex","seed":"codex-v1","target":"/home/itos/.codex"},"harnesses":[{"name":"opencode","profile":"opencode-v1"}],"lifetime"`) {
		t.Fatal(both.Canonical)
	}
	record := port.Record{Candidate: got, Generation: "2"}
	if decoded, f := codec.Decode(plain, codec.Encode(record)); f != "" || decoded != record {
		t.Fatal(decoded, f)
	}
	paths := codec.ChangedPaths(got, port.Record{Candidate: plain, Generation: "1"}, true)
	if len(paths) != 1 || paths[0] != "/home/harness" {
		t.Fatal(paths)
	}
	// No frozen input may carry the harness until run can seed it.
	frozen := port.Frozen{Project: got.Project, Generation: "2", Fingerprint: got.Fingerprint, Settings: got.Canonical}
	if r, f := codec.FrozenRecord(frozen); f != "state.invalid" || r != (port.Record{}) {
		t.Fatal("a frozen Codex harness was accepted", r, f)
	}
}

func TestInvalidCodexHarnessSettingsCannotAuthorize(t *testing.T) {
	for _, harness := range []codexHarness{
		{"codex", "codex-v2", "/home/itos/.codex"},
		{"codex", "codex-v1", "/home/itos/.other"},
		{"opencode", "codex-v1", "/home/itos/.codex"},
		{"", "", ""},
	} {
		testRejectsSelfConsistentInvalidSettings(t, func(s *settings) {
			h := volumeHome()
			h.Harness = &harness
			s.Home = &h
			s.Profile = "offline-home-v1"
		})
	}
}
