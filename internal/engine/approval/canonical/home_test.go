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
