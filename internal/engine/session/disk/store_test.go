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
