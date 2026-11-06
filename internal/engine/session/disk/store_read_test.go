package disk

import (
	"bytes"
	"strings"
	"testing"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	rf "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func intentReadFixture(t *testing.T) (Store, sp.Binding, *af.Storage, *rf.Checkout) {
	b, _, _ := sf.Fixture()
	b.Checkout.Root = t.TempDir() // The disk adapter requires a host-native absolute path.
	storage := &af.Storage{Directories: map[string]*af.Directory{
		"intents/" + b.Frozen.Project:   {Files: map[string][]byte{}},
		"snapshots/" + b.Frozen.Project: {Files: map[string][]byte{"1.json": []byte("snapshot")}},
	}}
	checkout := &rf.Checkout{Identity: b.Checkout}
	store := Store{
		Storage: storage, Codec: &sf.Codec{Value: b},
		Host: &af.Host{UID: b.UID, GID: b.GID}, Checkout: checkout,
		Input: rp.Input{Root: b.Checkout.Root, OS: "linux", Architecture: "amd64"},
	}
	return store, b, storage, checkout
}

func TestOpeningMissingIntentDoesNotCreateStorage(t *testing.T) {
	for _, operation := range []string{"launch", "cleanup"} {
		for _, missing := range []string{"collection", "record"} {
			t.Run(operation+"/"+missing, func(t *testing.T) {
				store, b, storage, checkout := intentReadFixture(t)
				key := "intents/" + b.Frozen.Project
				if missing == "collection" {
					delete(storage.Directories, key)
				}
				before := len(storage.Directories)
				open := store.Open
				if operation == "cleanup" {
					open = store.OpenCleanup
				}
				entry, failure := open(t.Context(), sp.Reference(b.Frozen.Project, strings.Repeat("1", 64)))
				if entry != nil {
					entry.Close()
				}
				if entry != nil || failure != sp.Refused {
					t.Fatal("missing intent granted authority", entry, failure)
				}
				if len(storage.Directories) != before || checkout.Opened {
					t.Fatal("missing read created storage or retained checkout authority")
				}
				if directory := storage.Directories[key]; directory != nil && (directory.Locked || len(directory.Files) != 0) {
					t.Fatal("missing read retained its lock or published a record")
				}
			})
		}
	}
}

func TestIntentReadFailurePreservesCauseEvidenceAndRetry(t *testing.T) {
	for _, operation := range []string{"launch", "cleanup"} {
		for _, tc := range []struct {
			failure ap.Failure
			want    sp.Outcome
		}{
			{"state.unavailable", sp.Incomplete},
			{"state.path", sp.Unsafe},
			{"state.invalid", sp.Invalid},
			{"state.conflict", sp.Refused},
		} {
			t.Run(operation+"/"+string(tc.failure), func(t *testing.T) {
				store, b, storage, checkout := intentReadFixture(t)
				token := strings.Repeat("1", 64)
				record := b.Intent(token)
				directory := storage.Directories["intents/"+b.Frozen.Project]
				data := store.Codec.Encode(record)
				directory.Files[token+".json"] = bytes.Clone(data)
				directory.ReadFailure = tc.failure
				open := store.Open
				if operation == "cleanup" {
					open = store.OpenCleanup
				}
				ref := sp.Reference(b.Frozen.Project, token)
				entry, failure := open(t.Context(), ref)
				if entry != nil {
					entry.Close()
				}
				if entry != nil || failure != tc.want {
					t.Fatalf("read failure = %v, %s; want no intent and %s", entry, failure, tc.want)
				}
				if checkout.Opened || !bytes.Equal(directory.Files[token+".json"], data) {
					t.Fatal("failed read changed evidence or retained checkout authority")
				}
				directory.ReadFailure = ""
				entry, failure = open(t.Context(), ref)
				if failure != "" || entry == nil {
					t.Fatal("read failure prevented retry", failure)
				}
				defer entry.Close()
				if entry.Record() != record || entry.Binding() != b {
					t.Fatal("retry lost original ownership")
				}
			})
		}
	}
}
