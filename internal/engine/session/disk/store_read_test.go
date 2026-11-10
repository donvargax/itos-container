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
	t.Helper()
	b, _, _ := sf.Fixture()
	// The selected platform is Linux even on a Windows test host. Keep the
	// fixture's absolute Linux path; no checkout filesystem is used here.
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

// lockProbe reports whether the intent collection is held while a checkout scan
// runs: the real storage holds its account-wide control lock from the creating
// open until publication.
type lockProbe struct {
	*rf.Checkout
	held   func() bool
	scans  int
	locked int
}

func (p *lockProbe) observe() {
	p.scans++
	if p.held() {
		p.locked++
	}
}
func (p *lockProbe) Inspect() (rp.CheckoutIdentity, rp.Failure) {
	p.observe()
	return p.Checkout.Inspect()
}
func (p *lockProbe) Unchanged() bool {
	p.observe()
	return p.Checkout.Unchanged()
}

func TestNewIntentScansCheckoutBeforeTakingTheControlLock(t *testing.T) {
	for _, operation := range []string{"intent", "reserved"} {
		t.Run(operation, func(t *testing.T) {
			store, b, storage, checkout := intentReadFixture(t)
			directory := storage.Directories["intents/"+b.Frozen.Project]
			probe := &lockProbe{Checkout: checkout, held: func() bool { return directory.Locked }}
			store.Checkout = probe
			create := store.New
			if operation == "reserved" {
				create = store.Prepare
			}
			entry, failure := create(t.Context(), ap.Durable{Frozen: b.Frozen}, strings.Repeat("1", 64))
			if failure != "" || entry == nil {
				t.Fatal("valid creation refused", failure)
			}
			defer entry.Close()
			if probe.scans != 2 {
				t.Fatalf("creation ran %d checkout scans, want inspection and revalidation", probe.scans)
			}
			if probe.locked != 0 {
				t.Fatalf("%d checkout scans ran while the control lock was held", probe.locked)
			}
			if !directory.Locked || directory.Files[strings.Repeat("1", 64)+".json"] == nil {
				t.Fatal("creation did not publish its intent under the lock")
			}
		})
	}
}

func TestRefusedCheckoutScanNeverOpensStorageForCreation(t *testing.T) {
	for _, fault := range []string{"identity differs", "changed during scan", "inspection failure"} {
		t.Run(fault, func(t *testing.T) {
			store, b, storage, checkout := intentReadFixture(t)
			delete(storage.Directories, "intents/"+b.Frozen.Project)
			switch fault {
			case "identity differs":
				checkout.Identity.Inode = "replaced"
			case "changed during scan":
				checkout.Changed = true
			case "inspection failure":
				checkout.InspectFailure = rp.CheckoutUnavailable
			}
			entry, failure := store.New(t.Context(), ap.Durable{Frozen: b.Frozen}, strings.Repeat("1", 64))
			if entry != nil {
				entry.Close()
			}
			if entry != nil || failure != sp.Refused {
				t.Fatal("refused checkout was accepted", entry, failure)
			}
			if _, created := storage.Directories["intents/"+b.Frozen.Project]; created || checkout.Opened {
				t.Fatal("refused checkout created intent storage or kept its pin")
			}
		})
	}
}
