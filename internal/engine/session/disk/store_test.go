package disk

import (
	"context"
	"strings"
	"testing"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

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
