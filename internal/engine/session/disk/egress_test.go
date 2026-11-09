package disk

import (
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	rf "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"testing"
)

func TestStoreEgressRecordRoundTrip(t *testing.T) {
	b, _, _ := sf.Fixture()
	b = sf.EgressBinding(b)
	rs := sf.EgressRecords(b)
	codec := &sf.Codec{Value: b}
	directory := &af.Directory{Files: map[string][]byte{rs[0].NameToken() + ".json": codec.Encode(rs[0])}}
	storage := &af.Storage{Directories: map[string]*af.Directory{
		"intents/" + b.Frozen.Project:   directory,
		"snapshots/" + b.Frozen.Project: {Files: map[string][]byte{"1.json": []byte("snapshot")}},
	}}
	store := Store{Storage: storage, Codec: codec, Host: &af.Host{UID: b.UID, GID: b.GID}, Checkout: &rf.Checkout{Identity: b.Checkout}, Input: rp.Input{Root: b.Checkout.Root, OS: "linux", Architecture: "amd64"}}
	ref := sp.Reference(b.Frozen.Project, rs[0].NameToken())
	var intent sp.Intent
	for n, r := range rs[1:] {
		var failure sp.Outcome
		intent, failure = store.Open(t.Context(), ref)
		if failure != "" {
			t.Fatalf("reopen %d: %s", n, failure)
		}
		if failure = intent.Save(r); failure != "" {
			intent.Close()
			t.Fatalf("save boundary %d: %s", n, failure)
		}
		if intent.Record() != r || intent.Validate() != "" {
			t.Error("saved boundary lost authority")
		}
		intent.Close()
	}
	intent, failure := store.Open(t.Context(), ref)
	if failure != "" {
		t.Fatal(failure)
	}
	defer intent.Close()
	if failure = intent.Save(rs[6]); failure != sp.Invalid {
		t.Error("terminal regression accepted", failure)
	}
	bad := rs[len(rs)-1]
	bad.Egress.Proxy.ID = rs[1].NameToken()
	directory.Files[bad.NameToken()+".json"] = codec.Encode(bad)
	if failure = intent.Validate(); failure != sp.Refused {
		t.Error("tampered egress accepted", failure)
	}
}
