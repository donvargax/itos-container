package disk

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	rf "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestHomeOriginalStateMustBeOutsideTheCheckout(t *testing.T) {
	root, err := filepath.Abs("original-home-fixture")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, state, checkout string
		unsafe                bool
	}{
		{"same directory", root, root, true},
		{"state inside checkout", filepath.Join(root, "state"), root, true},
		{"checkout inside state", root, filepath.Join(root, "checkout"), true},
		{"prefix sibling", root + "-state", root, false},
		{"reverse prefix sibling", root, root + "-checkout", false},
		{"separate roots", filepath.Join(root, "state"), filepath.Join(root, "checkout"), false},
		{"relative checkout", root, "checkout", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := originalOverlap(tc.state, tc.checkout); got != tc.unsafe {
				t.Fatalf("original state boundary unsafe=%v, want %v", got, tc.unsafe)
			}
		})
	}
}

type homeTestCodecs struct {
	*sf.Codec
	*sf.HomeCodec
	*sf.PublicRecords
}

func homeDiskState() (Homes, sp.HomeScope, *af.Catalog, *homeTestCodecs) {
	b, _, _ := sf.Fixture()
	b.Home = true
	codec := &homeTestCodecs{Codec: &sf.Codec{Value: b}, HomeCodec: &sf.HomeCodec{}, PublicRecords: &sf.PublicRecords{}}
	id := strings.Repeat("d", 64)
	s := sp.HomeScope{Schema: 1, Scope: codec.Scope(b), Project: b.Frozen.Project, UID: b.UID, GID: b.GID, Engine: sp.HomeEngineIdentity{Endpoint: b.Endpoint, ID: b.EngineID}, Current: id, Generations: []sp.HomeGeneration{{ID: id, Name: "itos-container-home-" + id, Created: "original", Stage: "retained", Initialized: true}}}
	catalog := &af.Catalog{Directories: map[string]map[string]*af.Directory{b.UID: {s.Scope: {Files: map[string][]byte{s.Scope + ".json": codec.EncodeHome(s)}}}}}
	return Homes{Storage: catalog, Codec: codec, Host: &af.Host{UID: b.UID, GID: b.GID}, StateRoot: "/state/itos-container"}, s, catalog, codec
}

func TestHomeAbsentReadDoesNotCreateCatalogState(t *testing.T) {
	h, s, catalog, _ := homeDiskState()
	delete(catalog.Directories[s.UID], s.Scope)
	if tx, failure := h.Open(t.Context(), s.Scope, false); tx != nil || failure != "" {
		t.Fatal(tx, failure)
	}
	if len(catalog.Directories[s.UID]) != 0 {
		t.Fatal("read created protected state")
	}
}

func TestHomeStorageRefusesChangedAndInvalidRecords(t *testing.T) {
	for _, fault := range []string{"list unavailable", "read unavailable", "wrong scope", "wrong uid", "malformed", "save binding", "save invalid", "read replacement", "resync"} {
		t.Run(fault, func(t *testing.T) {
			h, s, c, codec := homeDiskState()
			d := c.Directories[s.UID][s.Scope]
			want := sp.Refused
			switch fault {
			case "list unavailable":
				c.Failure = "state.unavailable"
				if _, f := h.List(t.Context()); f != sp.Incomplete {
					t.Fatal(f)
				}
				return
			case "read unavailable":
				d.ReadFailure = "state.unavailable"
				want = sp.Incomplete
			case "wrong scope":
				other := s
				other.Scope = strings.Repeat("e", 64)
				d.Files[s.Scope+".json"] = codec.EncodeHome(other)
			case "wrong uid":
				other := s
				other.UID = "1001"
				d.Files[s.Scope+".json"] = codec.EncodeHome(other)
			case "malformed":
				d.Files[s.Scope+".json"] = []byte("invalid")
				want = sp.Invalid
			}
			tx, f := h.Open(t.Context(), s.Scope, false)
			if fault == "read unavailable" || fault == "wrong scope" || fault == "wrong uid" || fault == "malformed" {
				if tx != nil || f != want || d.Locked {
					t.Fatal(tx, f, want, d.Locked)
				}
				return
			}
			if f != "" {
				t.Fatal(f)
			}
			defer tx.Close()
			switch fault {
			case "save binding":
				s.Scope = "other"
				f = tx.Save(s)
			case "save invalid":
				codec.Failure = sp.Invalid
				f = tx.Save(s)
				want = sp.Invalid
			case "read replacement":
				d.Files[s.Scope+".json"] = []byte("replacement")
				_, f = tx.Read()
			case "resync":
				d.SyncFailure = "state.write"
				f = tx.Save(s)
				want = sp.IntentWrite
			}
			if f != want {
				t.Fatal(f, want)
			}
		})
	}
}

func TestHomeCompletionFailureKeepsPendingAuthority(t *testing.T) {
	for _, stage := range []string{"reset-pending", "delete-pending"} {
		t.Run(stage, func(t *testing.T) {
			h, s, c, codec := homeDiskState()
			s.Generations[0].Stage = stage
			d := c.Directories[s.UID][s.Scope]
			original := codec.EncodeHome(s)
			d.Files[s.Scope+".json"] = original
			tx, f := h.Open(t.Context(), s.Scope, false)
			if f != "" {
				t.Fatal(f)
			}
			defer tx.Close()
			next := s
			next.Generations = append([]sp.HomeGeneration{}, s.Generations...)
			next.Generations[0].Stage = "deleted"
			d.PublishFailure = "state.write"
			if f = tx.Save(next); f != sp.IntentWrite {
				t.Fatal(f)
			}
			if !bytes.Equal(d.Files[s.Scope+".json"], original) {
				t.Fatal("unconfirmed completion replaced pending authority")
			}
			d.PublishFailure = ""
			if f = tx.Save(next); f != "" {
				t.Fatal("fresh retry", f)
			}
		})
	}
	_, s, _, codec := homeDiskState()
	if completingHome(codec, []byte("invalid"), s) {
		t.Fatal("malformed authority accepted")
	}
	previous := codec.EncodeHome(s)
	if completingHome(codec, previous, s) {
		t.Fatal("ordinary save classified as retirement")
	}
}

func TestHomeClaimsReadOriginalAuthorityWithoutReleasingIt(t *testing.T) {
	for _, fault := range []string{"valid", "bad claim", "no codecs", "no locations", "no original reader", "read failure", "intent missing", "intent malformed", "snapshot missing", "snapshot malformed", "wrong binding", "overlap", "inspect failure", "registry absent", "sessions failure", "public malformed", "public changed", "wrong session", "replaced directory"} {
		t.Run(fault, func(t *testing.T) {
			h, s, _, codec := homeDiskState()
			b := codec.Value
			id, token := strings.Repeat("b", 64), strings.Repeat("c", 64)
			r := b.Intent(token)
			r.Home = s.Generations[0].Reference(s.Scope)
			p := sp.PublicRecord{Schema: 1, ID: id, Project: b.Frozen.Project, Intent: sp.Reference(r.Project, token), State: "failed"}
			codec.PublicRecords.Records = map[string]sp.PublicRecord{"public": p}
			claim := sp.HomeClaim{Session: id, StateRoot: h.StateRoot, Intent: p.Intent}
			intents := &af.Directory{Files: map[string][]byte{token + ".json": codec.Encode(r)}}
			snapshots := &af.Directory{Files: map[string][]byte{"1.json": []byte("snapshot")}}
			storage := &af.Storage{Directories: map[string]*af.Directory{"intents/" + r.Project: intents, "snapshots/" + r.Project: snapshots}}
			index := registryKey(b.Checkout.Root, b.UID) + "/" + id
			registry := &af.Registry{Sessions: map[string]*af.SessionFiles{index: {Data: []byte("public")}}}
			locations := &af.Locations{Stores: map[string]ap.Storage{h.StateRoot: storage}, Registries: map[string]*af.Registry{h.StateRoot: registry}}
			h.Locations = locations
			want := sp.Incomplete
			switch fault {
			case "valid":
				want = sp.HomeInUse
			case "bad claim":
				claim.Intent = "not a reference"
				want = sp.Invalid
			case "no codecs":
				h.Codec = &sf.HomeCodec{}
			case "no locations":
				h.Locations = nil
			case "no original reader":
				delete(locations.Stores, h.StateRoot)
			case "read failure":
				intents.ReadFailure = "state.path"
				want = sp.Unsafe
			case "intent missing":
				delete(intents.Files, token+".json")
			case "intent malformed":
				intents.Files[token+".json"] = []byte("malformed")
				want = sp.Invalid
			case "snapshot missing":
				delete(snapshots.Files, "1.json")
			case "snapshot malformed":
				snapshots.Files["1.json"] = []byte("malformed")
				want = sp.Invalid
			case "wrong binding":
				r.GID = "42"
				intents.Files[token+".json"] = codec.Encode(r)
				want = sp.Invalid
			case "overlap":
				claim.StateRoot = b.Checkout.Root
				locations.Stores[claim.StateRoot] = storage
				want = sp.Unsafe
			case "inspect failure":
				locations.Failure = "state.path"
				want = sp.Unsafe
			case "registry absent":
				delete(locations.Registries, h.StateRoot)
			case "sessions failure":
				registry.ReadFailure = "state.path"
				want = sp.Unsafe
			case "public malformed":
				registry.Sessions[index].Data = []byte("malformed")
				want = sp.Invalid
			case "public changed":
				p.Intent = "changed"
				codec.PublicRecords.Records["public"] = p
				want = sp.Invalid
			case "wrong session":
				delete(registry.Sessions, index)
				registry.Sessions[registryKey(b.Checkout.Root, b.UID)+"/"+token] = &af.SessionFiles{Data: []byte("public")}
			case "replaced directory":
				locations.Changed = true
				want = sp.Unsafe
			}
			s.Generations[0].Claims = []sp.HomeClaim{claim}
			if got := h.CheckClaims(t.Context(), s, s.Generations[0]); got != want {
				t.Fatal(got, want)
			}
			if len(s.Generations[0].Claims) != 1 || intents.Locked || snapshots.Locked {
				t.Fatal("claim inspection changed ownership or retained a lock")
			}
		})
	}
	h, _, _, _ := homeDiskState()
	if _, f := h.Claim(sp.Record{}, "invalid"); f != sp.Invalid {
		t.Fatal(f)
	}
}

func TestHomeSessionScopeCannotBeSubstitutedOrInferred(t *testing.T) {
	for _, fault := range []string{"wrong scope", "missing codec"} {
		t.Run(fault, func(t *testing.T) {
			h, s, _, codec := homeDiskState()
			b := codec.Value
			token := strings.Repeat("b", 64)
			r := b.Intent(token)
			r.Home = s.Generations[0].Reference(s.Scope)
			var records sp.Codec = codec
			if fault == "wrong scope" {
				r.Home.Scope = strings.Repeat("f", 64)
			} else {
				records = codec.Codec
			}
			storage := &af.Storage{Directories: map[string]*af.Directory{"intents/" + r.Project: {Files: map[string][]byte{token + ".json": codec.Encode(r)}}, "snapshots/" + r.Project: {Files: map[string][]byte{"1.json": []byte("snapshot")}}}}
			store := Store{Storage: storage, Codec: records, Host: h.Host, Checkout: &rf.Checkout{Identity: b.Checkout}, Input: rp.Input{Root: b.Checkout.Root, OS: "linux", Architecture: "amd64"}}
			entry, failure := store.Open(t.Context(), sp.Reference(r.Project, token))
			if failure == "" {
				failure = entry.Validate()
				entry.Close()
			}
			if failure != sp.Invalid {
				t.Fatal(failure)
			}
		})
	}
}
