package disk

import (
	"bytes"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	rf "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"strings"
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
	if b.Egress != (sp.EgressGrant{}) {
		directory.Files[r.NameToken()+".egress.json"] = executionBytes(b.Egress.ImageID)
	}
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

func TestEgressExecutionEvidenceIsRequiredAndImmutable(t *testing.T) {
	for _, defect := range []string{"missing", "schema", "bad id", "conflict", "extra", "duplicate", "oversized", "read failure"} {
		t.Run(defect, func(t *testing.T) {
			b, _, _ := sf.Fixture()
			b = sf.EgressBinding(b)
			record := sf.EgressRecords(b)[6]
			store, directory := egressStore(b, record)
			name := record.NameToken() + ".egress.json"
			ref := sp.Reference(b.Frozen.Project, record.NameToken())
			opened, failure := store.Open(t.Context(), ref)
			if failure != "" {
				t.Fatal(failure)
			}
			switch defect {
			case "missing":
				delete(directory.Files, name)
			case "schema":
				directory.Files[name] = []byte(`{"proxy_image_id":"` + b.Egress.ImageID + `","schema":2}`)
			case "bad id":
				directory.Files[name] = executionBytes("sha256:bad")
			case "conflict":
				directory.Files[name] = executionBytes("sha256:" + strings.Repeat("a", 64))
			case "extra":
				directory.Files[name] = []byte(`{"extra":1,"proxy_image_id":"` + b.Egress.ImageID + `","schema":1}`)
			case "duplicate":
				directory.Files[name] = []byte(`{"proxy_image_id":"` + b.Egress.ImageID + `","schema":1,"schema":1}`)
			case "oversized":
				directory.Files[name] = bytes.Repeat([]byte(" "), 1025)
			case "read failure":
				directory.ReadFailures = map[string]ap.Failure{name: "state.path"}
			}
			if opened.Validate() == "" {
				t.Fatal("changed execution evidence accepted")
			}
			opened.Close()
			if recovered, f := store.OpenCleanup(t.Context(), ref); recovered != nil || f == "" {
				if recovered != nil {
					recovered.Close()
				}
				t.Fatal("recovery inferred missing/conflicting original image", f)
			}
		})
	}
}

func TestEgressExecutionPublicationPrecedesIntentAndRefusesMissingObservation(t *testing.T) {
	for _, id := range []string{"", "sha256:bad", "sha256:" + strings.Repeat("a", 64), "sha256:" + strings.Repeat("e", 64)} {
		b, _, _ := sf.Fixture()
		b = sf.EgressBinding(b)
		r := sf.EgressRecords(b)[0]
		store, d := egressStore(b, r)
		delete(d.Files, r.NameToken()+".json")
		delete(d.Files, r.NameToken()+".egress.json")
		opened, f := store.Prepare(t.Context(), ap.Durable{Frozen: b.Frozen, ProxyImageID: id}, r.NameToken())
		if id == b.Egress.ImageID {
			if f != "" || opened == nil || opened.Binding().Egress.ImageID != id || !bytes.Equal(d.Files[r.NameToken()+".egress.json"], executionBytes(id)) {
				t.Fatal("original image not durable", f)
			}
			opened.Close()
		} else if opened != nil || f != sp.Invalid || len(d.Files) != 0 {
			if opened != nil {
				opened.Close()
			}
			t.Fatal("unobserved/conflicting image granted intent", f)
		}
	}
}

func TestExecutionPublicationRefusesConflictsAndStorageFailures(t *testing.T) {
	for _, defect := range []string{"read", "write", "existing", "offline observation", "incomplete grant"} {
		t.Run(defect, func(t *testing.T) {
			b, _, _ := sf.Fixture()
			b = sf.EgressBinding(b)
			r := sf.EgressRecords(b)[0]
			store, d := egressStore(b, r)
			name := r.NameToken() + ".egress.json"
			delete(d.Files, r.NameToken()+".json")
			id := b.Egress.ImageID
			want := sp.Refused
			switch defect {
			case "read":
				d.ReadFailures = map[string]ap.Failure{name: "state.path"}
				want = sp.Unsafe
			case "write":
				delete(d.Files, name)
				d.WriteFailure = "state.write"
				want = sp.IntentWrite
			case "offline observation":
				b.Egress = sp.EgressGrant{}
				store.Codec = &sf.Codec{Value: b}
				want = sp.Invalid
			case "incomplete grant":
				b.Egress.Names = ""
				store.Codec = &sf.Codec{Value: b}
				want = sp.Invalid
			}
			opened, f := store.Prepare(t.Context(), ap.Durable{Frozen: b.Frozen, ProxyImageID: id}, r.NameToken())
			if opened != nil || f != want {
				if opened != nil {
					opened.Close()
				}
				t.Fatalf("unusable evidence granted intent: %s, want %s", f, want)
			}
			if d.Files[r.NameToken()+".json"] != nil {
				t.Fatal("published an intent after refused execution evidence")
			}
		})
	}
}

func TestExecutionObservationFillsOnlyTheOriginalSnapshotGrant(t *testing.T) {
	for _, id := range []string{"", "sha256:bad", "sha256:" + strings.Repeat("e", 64)} {
		b, _, _ := sf.Fixture()
		b = sf.EgressBinding(b)
		b.Egress.ImageID = ""
		r := sf.EgressRecords(b)[0]
		store, d := egressStore(b, r)
		name := r.NameToken() + ".egress.json"
		d.Files[name] = executionBytes(id)
		ref := sp.Reference(b.Frozen.Project, r.NameToken())
		opened, f := store.OpenCleanup(t.Context(), ref)
		if id == "" || id == "sha256:bad" {
			if opened != nil || f != sp.Invalid {
				if opened != nil {
					opened.Close()
				}
				t.Fatal("unusable original image accepted", f)
			}
		} else {
			if f != "" || opened == nil || opened.Binding().Egress.ImageID != id || opened.Binding().Egress.Names != b.Egress.Names {
				t.Fatal("original evidence mapping lost", f)
			}
			opened.Close()
		}
		delete(d.Files, r.NameToken()+".json")
		delete(d.Files, name)
		created, f := store.Prepare(t.Context(), ap.Durable{Frozen: b.Frozen, ProxyImageID: id}, r.NameToken())
		if id == "" || id == "sha256:bad" {
			if created != nil || f != sp.Invalid {
				if created != nil {
					created.Close()
				}
				t.Fatal("unusable fresh image accepted", f)
			}
		} else {
			if f != "" || created == nil || created.Binding().Egress.ImageID != id {
				t.Fatal("fresh evidence mapping lost", f)
			}
			created.Close()
		}
	}
}
