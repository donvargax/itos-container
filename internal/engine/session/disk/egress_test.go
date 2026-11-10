package disk

import (
	"bytes"
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
	store, directory := egressStore(b, rs[0])
	codec := store.Codec
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

func egressStore(b sp.Binding, r sp.Record) (Store, *af.Directory) {
	codec := &sf.Codec{Value: b}
	directory := &af.Directory{Files: map[string][]byte{r.NameToken() + ".json": codec.Encode(r)}}
	storage := &af.Storage{Directories: map[string]*af.Directory{
		"intents/" + b.Frozen.Project:   directory,
		"snapshots/" + b.Frozen.Project: {Files: map[string][]byte{"1.json": []byte("snapshot")}},
	}}
	return Store{Storage: storage, Codec: codec, Host: &af.Host{UID: b.UID, GID: b.GID}, Checkout: &rf.Checkout{Identity: b.Checkout}, Input: rp.Input{Root: b.Checkout.Root, OS: "linux", Architecture: "amd64"}}, directory
}

func TestStoreEgressRequiresSnapshotGrant(t *testing.T) {
	for _, grant := range []bool{false, true} {
		b, _, _ := sf.Fixture()
		r := sf.EgressRecords(b)[6]
		want := sp.Invalid
		if grant {
			b = sf.EgressBinding(b)
			want = ""
		}
		store, _ := egressStore(b, r)
		i, failure := store.Open(t.Context(), sp.Reference(b.Frozen.Project, r.NameToken()))
		if i != nil {
			defer i.Close()
		}
		if failure != want {
			t.Fatalf("grant %t: open = %s, want %s", grant, failure, want)
		}
		if grant && (i == nil || i.Record() != r || i.Validate() != "") {
			t.Fatal("granted schema 3 record lost authority")
		}
	}
}

func TestStoreEgressRefusesWorkerIntentWithoutNetworks(t *testing.T) {
	for _, grant := range []bool{false, true} {
		b, _, _ := sf.Fixture()
		want := sp.Outcome("")
		if grant {
			b = sf.EgressBinding(b)
			want = sp.Invalid
		}
		rs := sf.EgressRecords(b)
		before := rs[0]
		next := before
		next.Stage = "intent"
		if !before.ValidTransition(next) {
			t.Fatal("fixture must reach the binding-dependent egress check")
		}
		store, directory := egressStore(b, before)
		name := before.NameToken() + ".json"
		original := bytes.Clone(directory.Files[name])
		i, failure := store.Open(t.Context(), sp.Reference(b.Frozen.Project, before.NameToken()))
		if failure != "" {
			t.Fatal(failure)
		}
		defer i.Close()
		if failure = i.Save(next); failure != want {
			t.Fatalf("grant %t: save = %s, want %s", grant, failure, want)
		}
		if grant {
			if i.Record() != before || !bytes.Equal(directory.Files[name], original) || i.Validate() != "" {
				t.Fatal("refused worker intent changed stored authority")
			}
			if failure = i.Save(rs[1]); failure != "" || i.Record() != rs[1] {
				t.Fatal("valid schema 3 network intent refused", failure)
			}
		} else if i.Record() != next || i.Validate() != "" {
			t.Fatal("offline worker intent lost authority")
		}
	}
}
