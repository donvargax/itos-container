package codec

import (
	"bytes"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func homeScopeFixture() port.HomeScope {
	b, _, _ := f.Fixture()
	id := strings.Repeat("a", 64)
	return port.HomeScope{Schema: 1, Scope: (JSON{}).Scope(b), Project: b.Frozen.Project, UID: b.UID, GID: b.GID,
		Engine: port.HomeEngineIdentity{Endpoint: b.Endpoint, ID: b.EngineID}, Current: id,
		Generations: []port.HomeGeneration{{ID: id, Name: "itos-container-home-" + id, Created: "original", Stage: "retained", Initialized: true, Claims: []port.HomeClaim{}}}}
}

func TestHomeCodecValidatesIdentityEvenWhenScopeHashMatches(t *testing.T) {
	j := JSON{}
	for _, tc := range []struct {
		name   string
		change func(*port.HomeScope)
		want   port.Outcome
	}{
		{"valid", func(*port.HomeScope) {}, ""},
		{"numeric limits", func(s *port.HomeScope) { s.UID, s.GID = "4294967295", "4294967295" }, ""},
		{"group zero", func(s *port.HomeScope) { s.GID = "0" }, ""},
		{"invalid project", func(s *port.HomeScope) { s.Project = "invalid" }, port.Invalid},
		{"root uid", func(s *port.HomeScope) { s.UID = "0" }, port.Invalid},
		{"noncanonical uid", func(s *port.HomeScope) { s.UID = "01000" }, port.Invalid},
		{"invalid uid", func(s *port.HomeScope) { s.UID = "invalid" }, port.Invalid},
		{"uid overflow", func(s *port.HomeScope) { s.UID = "4294967296" }, port.Invalid},
		{"noncanonical gid", func(s *port.HomeScope) { s.GID = "01000" }, port.Invalid},
		{"invalid gid", func(s *port.HomeScope) { s.GID = "-1" }, port.Invalid},
		{"gid overflow", func(s *port.HomeScope) { s.GID = "4294967296" }, port.Invalid},
		{"absent endpoint", func(s *port.HomeScope) { s.Engine.Endpoint = "" }, port.Invalid},
		{"absent daemon", func(s *port.HomeScope) { s.Engine.ID = "" }, port.Invalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := homeScopeFixture()
			tc.change(&s)
			b := s.Binding()
			b.Frozen.Project = s.Project
			s.Scope = j.Scope(b)
			if _, failure := j.DecodeHome(j.EncodeHome(s)); failure != tc.want {
				t.Fatalf("self-consistent identity returned %s, want %s", failure, tc.want)
			}
		})
	}
}

func TestHomeCodecRejectsIncompleteAuthority(t *testing.T) {
	j := JSON{}
	for _, change := range []func(*port.HomeScope){
		func(s *port.HomeScope) { s.Schema = 0 },
		func(s *port.HomeScope) { s.Project = "bad" },
		func(s *port.HomeScope) { s.Scope = strings.Repeat("b", 64) },
		func(s *port.HomeScope) { s.UID = "0" },
		func(s *port.HomeScope) { s.UID = "01000" },
		func(s *port.HomeScope) { s.UID = "invalid" },
		func(s *port.HomeScope) { s.GID = "-1" },
		func(s *port.HomeScope) { s.GID = "01" },
		func(s *port.HomeScope) { s.Engine.Endpoint = "" },
		func(s *port.HomeScope) { s.Engine.ID = "" },
		func(s *port.HomeScope) { s.Current = "bad" },
		func(s *port.HomeScope) { s.Current = strings.Repeat("b", 64) },
		func(s *port.HomeScope) { s.Generations = nil },
		func(s *port.HomeScope) { s.Generations = append(s.Generations, s.Generations[0]) },
		func(s *port.HomeScope) { s.Generations[0].Name = "foreign" },
		func(s *port.HomeScope) { s.Generations[0].Stage = "unknown" },
		func(s *port.HomeScope) { s.Generations[0].Created = "" },
		func(s *port.HomeScope) { s.Generations[0].Successor = strings.Repeat("b", 64) },
	} {
		s := homeScopeFixture()
		change(&s)
		if _, failure := j.DecodeHome(j.EncodeHome(s)); failure != port.Invalid {
			t.Fatalf("accepted incomplete authority: %+v", s)
		}
	}
	valid := j.EncodeHome(homeScopeFixture())
	for _, raw := range [][]byte{nil, []byte("{"), append(bytes.Clone(valid), '\n'), bytes.Replace(valid, []byte(`"schema":1`), []byte(`"schema":1,"extra":true`), 1), make([]byte, 1024*1024+1)} {
		if _, failure := j.DecodeHome(raw); failure != port.Invalid {
			t.Fatal("accepted noncanonical record")
		}
	}
}

func TestHomeCodecRejectsRecycledGenerationIdentities(t *testing.T) {
	j := JSON{}
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for _, tc := range []struct {
		name   string
		reason string
		change func(*port.HomeScope)
	}{
		{"pending successor names a retired generation", "a pending reset must reserve a fresh token, not an identity the record already holds", func(s *port.HomeScope) {
			s.Generations = []port.HomeGeneration{
				{ID: b, Name: "itos-container-home-" + b, Created: "first", Stage: "reset", Successor: a, Claims: []port.HomeClaim{}},
				{ID: a, Name: "itos-container-home-" + a, Created: "second", Stage: "reset-pending", Successor: b, Claims: []port.HomeClaim{}},
			}
			s.Current = a
		}},
		{"reset chain returns to its own generation", "one generation ID can bind only one home identity", func(s *port.HomeScope) {
			s.Generations = []port.HomeGeneration{
				{ID: a, Name: "itos-container-home-" + a, Created: "first", Stage: "reset", Successor: b, Claims: []port.HomeClaim{}},
				{ID: b, Name: "itos-container-home-" + b, Created: "second", Stage: "reset", Successor: a, Claims: []port.HomeClaim{}},
				{ID: a, Name: "itos-container-home-" + a, Created: "third", Stage: "retained", Claims: []port.HomeClaim{}},
			}
			s.Current = a
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := homeScopeFixture()
			tc.change(&s)
			if _, failure := j.DecodeHome(j.EncodeHome(s)); failure != port.Invalid {
				t.Fatalf("%s: %s", tc.reason, failure)
			}
		})
	}
}

func TestHomeCodecGenerationChainAndClaims(t *testing.T) {
	j := JSON{}
	for _, stage := range []string{"awaiting-allocation", "intent", "retained", "reset-pending", "delete-pending", "deleted", "reset"} {
		t.Run(stage, func(t *testing.T) {
			s := homeScopeFixture()
			g := &s.Generations[0]
			g.Stage = stage
			if stage == "awaiting-allocation" || stage == "intent" {
				g.Created, g.Initialized = "", false
			}
			if strings.HasPrefix(stage, "reset") {
				g.Successor = strings.Repeat("b", 64)
			}
			if stage == "reset" {
				s.Current = g.Successor
				s.Generations = append(s.Generations, port.HomeGeneration{ID: s.Current, Name: "itos-container-home-" + s.Current, Stage: "awaiting-allocation", Claims: []port.HomeClaim{}})
			}
			assert := func(value port.HomeScope, want port.Outcome) {
				t.Helper()
				data := j.EncodeHome(value)
				got, failure := j.DecodeHome(data)
				if failure != want || want == "" && !bytes.Equal(data, j.EncodeHome(got)) {
					t.Fatalf("stage %s: %+v / %s, want %s", stage, value, failure, want)
				}
			}
			assert(s, "")
			clone := func() port.HomeScope { result, _ := j.DecodeHome(j.EncodeHome(s)); return result }
			for _, change := range []func(*port.HomeGeneration){
				func(g *port.HomeGeneration) { g.Successor = g.ID },
				func(g *port.HomeGeneration) { g.Created = ""; g.Initialized = true },
			} {
				bad := clone()
				change(&bad.Generations[0])
				assert(bad, port.Invalid)
			}
			claim := port.HomeClaim{Session: strings.Repeat("c", 64), StateRoot: "/original/state/itos-container", Intent: port.Reference(s.Project, strings.Repeat("d", 64))}
			claimed := clone()
			claimed.Generations[0].Claims = []port.HomeClaim{claim}
			if strings.HasPrefix(stage, "reset") || strings.HasPrefix(stage, "delete") {
				assert(claimed, port.Invalid)
			} else {
				assert(claimed, "")
				for _, badClaim := range []port.HomeClaim{{Session: "bad", StateRoot: claim.StateRoot, Intent: claim.Intent}, {Session: claim.Session, Intent: claim.Intent}, {Session: claim.Session, StateRoot: claim.StateRoot, Intent: "bad"}, {Session: claim.Session, StateRoot: claim.StateRoot, Intent: port.Reference(strings.Repeat("e", 64), strings.Repeat("d", 64))}} {
					bad := clone()
					bad.Generations[0].Claims = []port.HomeClaim{badClaim}
					assert(bad, port.Invalid)
				}
				claimed.Generations[0].Claims = append(claimed.Generations[0].Claims, claim)
				assert(claimed, port.Invalid)
			}
			if stage == "reset" {
				for _, change := range []func(*port.HomeScope){
					func(s *port.HomeScope) { s.Generations[0].Stage = "retained"; s.Generations[0].Successor = "" },
					func(s *port.HomeScope) { s.Generations[0].Successor = strings.Repeat("e", 64) },
					func(s *port.HomeScope) { s.Generations = s.Generations[:1]; s.Current = s.Generations[0].ID },
				} {
					bad := clone()
					change(&bad)
					assert(bad, port.Invalid)
				}
			}
		})
	}
}
