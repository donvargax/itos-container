package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	canonical "github.com/donvargax/itos-container/internal/engine/approval/canonical"
	approvaldisk "github.com/donvargax/itos-container/internal/engine/approval/disk"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	sessiondisk "github.com/donvargax/itos-container/internal/engine/session/disk"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestCheckoutAdmissionContracts(t *testing.T) {
	for _, backend := range []string{"memory", "disk"} {
		t.Run(backend, func(t *testing.T) {
			sf.AdmissionContract(t, func() sf.AdmissionFixture {
				if backend == "memory" {
					return memoryAdmissionFixture()
				}
				return diskAdmissionFixture(t)
			})
		})
	}
}
func memoryAdmissionFixture() sf.AdmissionFixture {
	f := sf.AdmissionFixture{}
	admissions := &sf.Admissions{}
	for index, root := range []string{"/work", "/work/child", "/work-more"} {
		b, store, _ := sf.Fixture()
		b.Checkout.Root = root
		b.Frozen.Project = strings.Repeat(string(rune('a'+index)), 64)
		store.Codec.(*sf.Codec).Value, store.Root = b, root
		store.Snapshots = map[string][]byte{b.Intent(strings.Repeat("1", 64)).Snapshot: []byte("snapshot")}
		f.Registries[index] = &sf.Registry{Store: store, Project: b.Frozen.Project, Admissions: admissions}
		f.Stores[index], f.Frozen[index] = store, ap.Durable{Frozen: b.Frozen}
	}
	f.WriteFailure = func(fail bool) {
		admissions.Failure = ""
		if fail {
			admissions.Failure = sp.IntentWrite
		}
	}
	return f
}
func diskAdmissionFixture(t *testing.T) sf.AdmissionFixture {
	t.Helper()
	f := sf.AdmissionFixture{}
	base := t.TempDir()
	state := filepath.Join(base, "state")
	failed := false
	for index, name := range []string{"work", "work/child", "work-more"} {
		store, durable, storage := sessionStoreFixture(t)
		root := filepath.Join(base, name)
		if err := os.Rename(store.Input.Root, root); err != nil {
			t.Fatal(err)
		}
		store.Input.Root = root
		b, failure := store.Codec.Frozen(durable.Frozen)
		if failure != "" {
			t.Fatal(failure)
		}
		b.Checkout.Root = root
		ready := af.Ready()
		ready.Checkout = &b.Checkout
		candidate := canonical.JSON{}.Candidate(ready, b.UID, b.GID)
		record := ap.Record{Candidate: candidate, Generation: "1"}
		snapshot := filepath.Join(state, "itos-container", "snapshots", candidate.Project, "1.json")
		if err := os.MkdirAll(filepath.Dir(snapshot), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(snapshot, canonical.JSON{}.Encode(record), 0600); err != nil {
			t.Fatal(err)
		}
		durable = ap.Durable{Frozen: ap.Frozen{Project: candidate.Project, Generation: "1", Fingerprint: candidate.Fingerprint, Settings: candidate.Canonical}, Reference: snapshot}
		storage.Store.StateHome = state
		store.Storage = storage
		registry := sessiondisk.Registry{Store: store, Storage: approvaldisk.Registry{Store: storage.Store, IO: approvaldisk.SnapshotIO{FileSync: func(file *os.File) error {
			if failed {
				return os.ErrPermission
			}
			return file.Sync()
		}}}, Codec: store.Codec.(sp.PublicCodec), Identity: canonical.JSON{}}
		f.Registries[index], f.Stores[index], f.Frozen[index] = registry, store, durable
	}
	f.WriteFailure = func(fail bool) { failed = fail }
	return f
}
