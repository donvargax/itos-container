package disk

import (
	"context"
	"reflect"
	"strings"
	"testing"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	rf "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestStoreAcquisitionRefusesUnusableEvidence(t *testing.T) {
	for _, tc := range []struct {
		operation, fault string
		want             sp.Outcome
	}{
		{"new", "valid", ""},
		{"new", "invalid token", sp.Invalid},
		{"new", "invalid project", sp.Invalid},
		{"new", "invalid frozen handoff", sp.Invalid},
		{"new", "unsupported host", sp.Refused},
		{"new", "checkout pin failure", sp.Refused},
		{"new", "record read failure", sp.IntentWrite},
		{"new", "checkout identity differs", sp.Refused},
		{"new", "snapshot locked", sp.Incomplete},
		{"new", "snapshot directory missing", sp.Invalid},
		{"open", "valid", ""},
		{"open", "invalid reference", sp.Invalid},
		{"open", "unsupported host", sp.Refused},
		{"open", "checkout pin failure", sp.Refused},
		{"open", "malformed record", sp.Invalid},
		{"open", "record missing", sp.Refused},
		{"open", "snapshot locked", sp.Incomplete},
		{"open", "snapshot directory missing", sp.Invalid},
	} {
		t.Run(tc.operation+"/"+tc.fault, func(t *testing.T) {
			b, _, _ := sf.Fixture()
			codec := &sf.Codec{Value: b}
			token := strings.Repeat("1", 64)
			reference := sp.Reference(b.Frozen.Project, token)
			record := b.Intent(token)
			if tc.operation == "open" {
				record.Stage, record.ContainerID = "owned", strings.Repeat("2", 64)
			}
			other := strings.Repeat("3", 64) + ".json"
			directory := &af.Directory{Files: map[string][]byte{other: []byte("unrelated ownership")}}
			if tc.operation == "open" {
				directory.Files[token+".json"] = codec.Encode(record)
			}
			snapshots := &af.Directory{Files: map[string][]byte{"1.json": []byte("snapshot")}}
			storage := &af.Storage{Directories: map[string]*af.Directory{
				"intents/" + b.Frozen.Project:   directory,
				"snapshots/" + b.Frozen.Project: snapshots,
			}}
			checkout := &rf.Checkout{Identity: b.Checkout}
			store := Store{Storage: storage, Codec: codec, Host: &af.Host{UID: b.UID, GID: b.GID}, Checkout: checkout, Input: rp.Input{Root: b.Checkout.Root, OS: "linux", Architecture: "amd64"}}
			durable := ap.Durable{Frozen: b.Frozen}
			switch tc.fault {
			case "invalid token":
				token = "../escape"
			case "invalid project":
				durable.Frozen.Project = "../escape"
				codec.Value.Frozen = durable.Frozen
			case "invalid frozen handoff":
				durable.Frozen.Settings = "unrecognized frozen settings"
			case "unsupported host":
				store.Input.OS = "windows"
			case "checkout pin failure":
				checkout.Failure = rp.CheckoutUnavailable
			case "record read failure":
				directory.ReadFailure = "state.unavailable"
			case "checkout identity differs":
				checkout.Identity.Inode = "replaced"
			case "snapshot locked":
				snapshots.Locked = true
			case "snapshot directory missing":
				delete(storage.Directories, "snapshots/"+b.Frozen.Project)
			case "record missing":
				delete(directory.Files, token+".json")
			case "invalid reference":
				reference = "../escape"
			case "malformed record":
				directory.Files[token+".json"] = []byte("unrecognized ownership bytes")
			}
			// Checkpoint independent bytes, including malformed evidence, before
			// the attempt. Refusal must neither publish nor repair any record.
			expected := map[string][]byte{}
			for name, data := range directory.Files {
				expected[name] = append([]byte{}, data...)
			}
			var intent sp.Intent
			var failure sp.Outcome
			if tc.operation == "new" {
				intent, failure = store.New(context.Background(), durable, token)
			} else {
				intent, failure = store.Open(context.Background(), reference)
			}
			if failure != tc.want || (intent == nil) != (tc.want != "") {
				if intent != nil {
					intent.Close()
				}
				t.Fatalf("acquisition = %v, %s; want %s", intent, failure, tc.want)
			}
			if intent != nil {
				defer intent.Close()
				if intent.Record() != record || intent.Binding() != b || intent.Validate() != "" {
					t.Fatal("valid acquisition lost its independent ownership binding")
				}
				if tc.operation == "new" {
					expected[token+".json"] = codec.Encode(record)
				}
			} else if directory.Locked || checkout.Opened {
				t.Fatal("refused acquisition retained its storage lock or checkout pin")
			}
			if !reflect.DeepEqual(directory.Files, expected) {
				t.Fatal("acquisition changed unexpected ownership evidence")
			}
			if snapshots.Locked != (tc.fault == "snapshot locked") {
				t.Fatal("acquisition leaked a snapshot lock or released another caller's lock")
			}
			if len(snapshots.Files) != 1 || string(snapshots.Files["1.json"]) != "snapshot" {
				t.Fatal("acquisition changed the original frozen snapshot")
			}
		})
	}
}

func TestStoreContractWithProtectedStorageFake(t *testing.T) {
	sf.StoreContract(t, func() (sp.Store, ap.Durable) {
		b, _, _ := sf.Fixture()
		storage := &af.Storage{Directories: map[string]*af.Directory{
			"snapshots/" + b.Frozen.Project: {Files: map[string][]byte{"1.json": []byte("snapshot")}},
		}}
		store := Store{Storage: storage, Codec: &sf.Codec{Value: b}, Host: &af.Host{UID: b.UID, GID: b.GID}, Checkout: &rf.Checkout{Identity: b.Checkout}, Input: rp.Input{Root: b.Checkout.Root, OS: "linux", Architecture: "amd64"}}
		return store, ap.Durable{Frozen: b.Frozen}
	})
}

func TestIntentRevalidatesEvidenceBeforeSave(t *testing.T) {
	for _, tc := range []struct {
		fault string
		want  sp.Outcome
	}{
		{"record unreadable", sp.Incomplete},
		{"record unsafe", sp.Unsafe},
		{"record missing", sp.Refused},
		{"record replaced", sp.Refused},
		{"snapshot binding replaced", sp.Invalid},
	} {
		t.Run(tc.fault, func(t *testing.T) {
			b, _, _ := sf.Fixture()
			codec := &sf.Codec{Value: b}
			token := strings.Repeat("1", 64)
			record := b.Intent(token)
			record.Stage, record.ContainerID = "owned", strings.Repeat("2", 64)
			data := codec.Encode(record)
			name := token + ".json"
			directory := &af.Directory{Files: map[string][]byte{name: data}}
			snapshots := &af.Directory{Files: map[string][]byte{"1.json": []byte("snapshot")}}
			storage := &af.Storage{Directories: map[string]*af.Directory{
				"intents/" + b.Frozen.Project:   directory,
				"snapshots/" + b.Frozen.Project: snapshots,
			}}
			checkout := &rf.Checkout{Identity: b.Checkout}
			store := Store{Storage: storage, Codec: codec, Host: &af.Host{UID: b.UID, GID: b.GID}, Checkout: checkout, Input: rp.Input{Root: b.Checkout.Root, OS: "linux", Architecture: "amd64"}}
			intent, failure := store.Open(context.Background(), sp.Reference(b.Frozen.Project, token))
			if failure != "" || intent == nil {
				t.Fatalf("open = %v, %s", intent, failure)
			}
			defer intent.Close()
			if failure = intent.Validate(); failure != "" {
				t.Fatal("healthy evidence refused", failure)
			}
			switch tc.fault {
			case "record unreadable":
				directory.ReadFailure = "state.unavailable"
			case "record unsafe":
				directory.ReadFailure = "state.unsafe"
			case "record missing":
				delete(directory.Files, name)
			case "record replaced":
				replacement := record
				replacement.ContainerID = strings.Repeat("3", 64)
				directory.Files[name] = codec.Encode(replacement)
			case "snapshot binding replaced":
				// The codec fake now resolves the snapshot to another valid
				// binding while the open intent retains its original binding.
				codec.Value.ImageID = "sha256:" + strings.Repeat("e", 64)
			}
			expected := append([]byte(nil), directory.Files[name]...)
			if failure = intent.Validate(); failure != tc.want {
				t.Fatalf("changed evidence validation = %s, want %s", failure, tc.want)
			}
			next := record
			next.Stage = "absent"
			if failure = intent.Save(next); failure != tc.want {
				t.Fatalf("save with changed evidence = %s, want %s", failure, tc.want)
			}
			if intent.Record() != record || intent.Binding() != b || !reflect.DeepEqual(directory.Files[name], expected) {
				t.Fatal("refused save changed the pinned identity or stored evidence")
			}
			if len(snapshots.Files) != 1 || string(snapshots.Files["1.json"]) != "snapshot" || snapshots.Locked {
				t.Fatal("refused save changed the frozen snapshot or retained its lock")
			}
			// Restore only the fixture fault; the same open intent must still
			// be able to advance its original identity once evidence is valid.
			directory.ReadFailure = ""
			directory.Files[name] = data
			codec.Value = b
			if failure = intent.Save(next); failure != "" {
				t.Fatal("recovered save refused", failure)
			}
			if intent.Record() != next || string(directory.Files[name]) != string(codec.Encode(next)) {
				t.Fatal("recovered save lost the original container identity")
			}
			intent.Close()
			if directory.Locked || checkout.Opened {
				t.Fatal("close retained the ownership lock or checkout pin")
			}
		})
	}
}

func TestUnsafeRecordMetadataRefusalReachesSessionStore(t *testing.T) {
	b, _, _ := sf.Fixture()
	codec := &sf.Codec{Value: b}
	token := strings.Repeat("1", 64)
	directory := &af.Directory{Files: map[string][]byte{token + ".json": codec.Encode(b.Intent(token))}, ReadFailure: "state.unsafe"}
	storage := &af.Storage{Directories: map[string]*af.Directory{"intents/" + b.Frozen.Project: directory}}
	host := &af.Host{UID: b.UID, GID: b.GID}
	store := Store{Storage: storage, Codec: codec, Host: host, Checkout: &sf.Checkout{Identity: b.Checkout}, Input: rp.Input{Root: b.Checkout.Root, OS: "linux", Architecture: "amd64"}}
	if intent, f := store.Open(context.Background(), sp.Reference(b.Frozen.Project, token)); intent != nil || f != sp.Unsafe {
		t.Fatal(intent, f)
	}
	if directory.Locked || host.UID != b.UID {
		t.Fatal("unsafe record leaked its lock or altered host identity")
	}
}

func TestSyncPreservesIndependentlyEncodedOwnership(t *testing.T) {
	b, _, _ := sf.Fixture()
	token := strings.Repeat("1", 64)
	record := b.Intent(token)
	record.Stage, record.ContainerID = "owned", strings.Repeat("2", 64)
	const original = "independently encoded ownership"
	codec := &sf.Codec{Value: b, Records: map[string]sp.Record{original: record}}
	directory := &af.Directory{Files: map[string][]byte{token + ".json": []byte(original)}}
	storage := &af.Storage{Directories: map[string]*af.Directory{
		"intents/" + b.Frozen.Project:   directory,
		"snapshots/" + b.Frozen.Project: {Files: map[string][]byte{"1.json": []byte("snapshot")}},
	}}
	store := Store{Storage: storage, Codec: codec, Host: &af.Host{UID: b.UID, GID: b.GID}, Checkout: &sf.Checkout{Identity: b.Checkout}, Input: rp.Input{Root: b.Checkout.Root, OS: "linux", Architecture: "amd64"}}
	intent, failure := store.Open(context.Background(), sp.Reference(b.Frozen.Project, token))
	if failure != "" {
		t.Fatal(failure)
	}
	defer intent.Close()
	if failure = intent.Save(record); failure != "" {
		t.Fatal(failure)
	}
	if string(directory.Files[token+".json"]) != original {
		t.Fatal("sync rewrote the existing representation")
	}
	if failure = intent.Validate(); failure != "" {
		t.Fatal("sync lost the exact stored ownership bytes", failure)
	}
	record.Stage = "absent"
	if failure = intent.Save(record); failure != "" {
		t.Fatal("could not advance independently encoded ownership", failure)
	}
}

func TestSnapshotReadFailurePreservesRecovery(t *testing.T) {
	b, _, _ := sf.Fixture()
	token := strings.Repeat("1", 64)
	record := b.Intent(token)
	record.Stage, record.ContainerID = "owned", strings.Repeat("2", 64)
	codec := &sf.Codec{Value: b}
	data := codec.Encode(record)
	directory := &af.Directory{Files: map[string][]byte{token + ".json": data}}
	snapshots := &af.Directory{Files: map[string][]byte{"1.json": []byte("snapshot")}, ReadFailure: "state.unavailable"}
	storage := &af.Storage{Directories: map[string]*af.Directory{
		"intents/" + b.Frozen.Project:   directory,
		"snapshots/" + b.Frozen.Project: snapshots,
	}}
	store := Store{Storage: storage, Codec: codec, Host: &af.Host{UID: b.UID, GID: b.GID}, Checkout: &sf.Checkout{Identity: b.Checkout}, Input: rp.Input{Root: b.Checkout.Root, OS: "linux", Architecture: "amd64"}}
	ref := sp.Reference(b.Frozen.Project, token)
	if intent, failure := store.Open(context.Background(), ref); intent != nil || failure != sp.Incomplete {
		t.Fatalf("unavailable snapshot read = %v, %s; want no intent and cleanup-incomplete", intent, failure)
	}
	if directory.Locked || snapshots.Locked {
		t.Fatal("failed snapshot read retained storage locks")
	}
	if string(directory.Files[token+".json"]) != string(data) {
		t.Fatal("failed snapshot read changed the ownership record")
	}
	snapshots.ReadFailure = ""
	intent, failure := store.Open(context.Background(), ref)
	if failure != "" || intent == nil {
		t.Fatalf("recovery after snapshot read becomes available = %v, %s", intent, failure)
	}
	defer intent.Close()
	if intent.Record() != record || intent.Binding() != b {
		t.Fatal("recovery lost the original ownership binding")
	}
}

func TestHostAndStorageFailureClassification(t *testing.T) {
	for _, tc := range []struct{ uid, gid, os, arch string }{{"0", "1000", "linux", "amd64"}, {"-1", "1000", "linux", "amd64"}, {"01", "1000", "linux", "amd64"}, {"1000", "bad", "linux", "amd64"}, {"1000", "1000", "darwin", "amd64"}, {"1000", "1000", "linux", "arm64"}} {
		store := Store{Host: &af.Host{UID: tc.uid, GID: tc.gid}, Input: rp.Input{OS: tc.os, Architecture: tc.arch}}
		if _, _, f := store.identity(); f != sp.Refused {
			t.Fatal(tc, f)
		}
	}
	for _, tc := range []struct {
		f     ap.Failure
		write bool
		want  sp.Outcome
	}{{"state.path", false, sp.Unsafe}, {"state.unsafe", false, sp.Unsafe}, {"state.invalid", false, sp.Invalid}, {"state.conflict", true, sp.Refused}, {"state.write", true, sp.IntentWrite}, {"state.unavailable", false, sp.Incomplete}} {
		if f := storageFailure(tc.f, tc.write); f != tc.want {
			t.Fatal(tc, f)
		}
	}
}
