package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	canonical "github.com/donvargax/itos-container/internal/engine/approval/canonical"
	approvaldisk "github.com/donvargax/itos-container/internal/engine/approval/disk"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	sessioncodec "github.com/donvargax/itos-container/internal/engine/session/codec"
	sessiondisk "github.com/donvargax/itos-container/internal/engine/session/disk"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestPublicRegistryContracts(t *testing.T) {
	for _, backend := range []string{"memory", "disk"} {
		t.Run(backend, func(t *testing.T) {
			sf.RegistryContract(t, func() (sp.Registry, sp.Store, ap.Durable) {
				if backend == "memory" {
					b, store, _ := sf.Fixture()
					return &sf.Registry{Store: store, Project: b.Frozen.Project}, store, ap.Durable{Frozen: b.Frozen}
				}
				store, d, storage := sessionStoreFixture(t)
				return sessiondisk.Registry{Store: store, Storage: approvaldisk.Registry{Store: storage.Store}, Codec: store.Codec.(sessioncodec.JSON), Identity: canonical.JSON{}}, store, d
			})
		})
	}
}

func TestPublicRegistryDiscoveryPreservesReadFailureCause(t *testing.T) {
	m := publicMemoryFixture(t)
	id, _ := m.seed("a")
	m.files.Failure = "state.unavailable"
	if ids, failure := m.registry.List(context.Background()); ids != nil || failure != sp.Incomplete {
		t.Fatal("discovery failure became a write failure or an empty success", ids, failure)
	}
	if len(m.files.Events) != 0 {
		t.Fatal("failed read wrote state")
	}
	m.files.Failure = ""
	if ids, failure := m.registry.List(context.Background()); failure != "" || !reflect.DeepEqual(ids, []string{id}) {
		t.Fatal("retry lost the registered session", ids, failure)
	}
}

func TestPublicRegistryRejectsInvalidCandidateBeforeStorage(t *testing.T) {
	for _, fault := range []string{"host", "invalid frozen", "different frozen user"} {
		t.Run(fault, func(t *testing.T) {
			m := publicMemoryFixture(t)
			var registry sp.Registry = m.registry
			want := sp.Refused
			switch fault {
			case "host":
				m.host.UID = "0"
			case "invalid frozen":
				d := ap.Durable{Frozen: m.b.Frozen}
				d.Frozen.Settings = "{}"
				registry, want = m.registry.ForRun(d), sp.Invalid
			case "different frozen user":
				registry = m.registry.ForRun(ap.Durable{Frozen: m.b.Frozen})
				m.host.UID = "1001"
			}
			if ids, failure := registry.List(context.Background()); ids != nil || failure != want {
				t.Fatal(ids, failure)
			}
			lease, failure := registry.Register(context.Background(), ap.Durable{Frozen: m.b.Frozen}, strings.Repeat("a", 64), strings.Repeat("b", 64))
			if lease != nil {
				lease.Close()
			}
			if lease != nil || failure != want || len(m.files.Events) != 0 || m.checkout.Opened {
				t.Fatal("candidate refusal leaked authority", failure)
			}
		})
	}
}

func TestPublicRegistryRequiresCheckoutAccessOnlyForRegistration(t *testing.T) {
	for _, fault := range []string{"pin", "inspect", "changed checkout"} {
		t.Run(fault, func(t *testing.T) {
			m := publicMemoryFixture(t)
			id, _ := m.seed("a")
			switch fault {
			case "pin":
				m.checkout.Failure = rp.CheckoutUnavailable
			case "inspect":
				m.checkout.InspectFailure = rp.CheckoutUnavailable
			default:
				m.checkout.Changed = true
			}
			// Recovery discovers the original binding through the protected
			// checkout index; launch still requires fresh checkout access.
			if ids, failure := m.registry.List(t.Context()); failure != "" || !reflect.DeepEqual(ids, []string{id}) {
				t.Fatal("mutable checkout vetoed discovery", ids, failure)
			}
			entry, failure := m.registry.Open(t.Context(), id, false)
			if failure != "" {
				t.Fatal("mutable checkout vetoed recovery authority", failure)
			}
			entry.Close()
			lease, failure := m.registry.Register(t.Context(), ap.Durable{Frozen: m.b.Frozen}, strings.Repeat("b", 64), strings.Repeat("c", 64))
			if lease != nil {
				lease.Close()
			}
			if lease != nil || failure != sp.Refused || len(m.files.Events) != 0 || m.checkout.Opened {
				t.Fatal("invalid launch identity reached registration", failure)
			}
		})
	}
}

func TestPublicRegistryRejectsNonabsoluteDiscoveryRoots(t *testing.T) {
	for _, root := range []string{"", ".", "relative/checkout"} {
		t.Run(root, func(t *testing.T) {
			m := publicMemoryFixture(t)
			id, _ := m.seed("a")
			m.registry.Store.Input.Root = root
			if ids, failure := m.registry.List(t.Context()); ids != nil || failure != sp.Refused {
				t.Fatal("nonabsolute root reached index discovery", ids, failure)
			}
			if entry, failure := m.registry.Open(t.Context(), id, true); entry != nil || failure != sp.Refused {
				t.Fatal("nonabsolute root opened protected authority", entry, failure)
			}
			if len(m.files.Events) != 0 || m.checkout.Opened {
				t.Fatal("invalid root consulted the checkout or wrote state")
			}
		})
	}
}

func TestPublicRegistryRejectsInvalidRegistrationBinding(t *testing.T) {
	for _, fault := range []string{"id", "token", "project", "frozen", "user"} {
		t.Run(fault, func(t *testing.T) {
			m := publicMemoryFixture(t)
			id, token := strings.Repeat("a", 64), strings.Repeat("b", 64)
			d := ap.Durable{Frozen: m.b.Frozen}
			want := sp.Refused
			switch fault {
			case "id":
				id = "../foreign"
			case "token":
				token = "invalid"
			case "project":
				d.Frozen.Project = strings.Repeat("f", 64)
			case "frozen":
				d.Frozen.Settings, want = "{}", sp.Invalid
			case "user":
				m.host.GID = "1001"
			}
			lease, failure := m.registry.Register(context.Background(), d, id, token)
			if lease != nil {
				lease.Close()
			}
			if lease != nil || failure != want || len(m.files.Events) != 0 {
				t.Fatal("invalid binding reached publication", failure)
			}
		})
	}
}

func TestPublicRegistryRetainsEvidenceThroughUpdateFailures(t *testing.T) {
	for _, fault := range []string{"write", "parent fsync", "sync", "changed directory", "changed record", "invalid next", "changed intent", "unavailable intent"} {
		t.Run(fault, func(t *testing.T) {
			m := publicMemoryFixture(t)
			id, _ := m.seed("a")
			files := m.files.Sessions[m.index().Project+"/"+id]
			entry, failure := m.registry.Open(context.Background(), id, false)
			if failure != "" {
				t.Fatal(failure)
			}
			defer entry.Close()
			original := entry.Record()
			before := string(files.Data)
			next := original
			next.State = "running"
			want := sp.Refused
			switch fault {
			case "write", "parent fsync":
				m.files.Fault, want = fault, sp.IntentWrite
			case "sync":
				m.files.Fault, want = "file fsync", sp.IntentWrite
			case "changed directory":
				files.Changed = true
			case "changed record":
				files.Data = append(files.Data, ' ')
			case "invalid next":
				next.State, want = "future", sp.Invalid
			case "changed intent":
				directory := m.storage.Directories["intents/"+m.c.Project]
				for name, data := range directory.Files {
					directory.Files[name] = append(data, ' ')
				}
			case "unavailable intent":
				m.storage.Failure, want = "state.unavailable", sp.Incomplete
			}
			if fault == "sync" {
				failure = entry.Sync()
			} else {
				failure = entry.Save(next)
			}
			if failure != want {
				t.Fatalf("update failure = %s, want %s", failure, want)
			}
			if fault == "parent fsync" {
				if entry.Record() != next || string(files.Data) == before {
					t.Fatal("visible uncertain publication lost its retry representation")
				}
			} else if entry.Record() != original {
				t.Fatal("failed update changed the held record")
			}
			if fault == "changed directory" || fault == "changed record" || fault == "changed intent" || fault == "unavailable intent" {
				if failure := entry.Sync(); failure != want {
					t.Fatal("sync accepted changed authority", failure)
				}
				intent := entry.Intent()
				if fault == "changed intent" || fault == "unavailable intent" {
					if failure := intent.Save(intent.Record()); failure != want {
						t.Fatal("intent update accepted changed authority", failure)
					}
				}
				return
			}
			m.files.Fault = ""
			if failure := entry.Save(entry.Record()); failure != "" {
				t.Fatal("unchanged retry did not restore durability", failure)
			}
		})
	}
}

func TestPublicRegistryOwnsProblemStorageAndRejectsChangedRead(t *testing.T) {
	m := publicMemoryFixture(t)
	id, _ := m.seed("a")
	files := m.files.Sessions[m.index().Project+"/"+id]
	record, failure := m.registry.Codec.DecodePublic(files.Data)
	if failure != "" {
		t.Fatal(failure)
	}
	problem := sp.Incomplete
	record.LastProblem = &problem
	files.Data = m.registry.Codec.EncodePublic(record)
	entry, failure := m.registry.Open(context.Background(), id, false)
	if failure != "" {
		t.Fatal(failure)
	}
	copy := entry.Record()
	*copy.LastProblem = sp.Internal
	if *entry.Record().LastProblem != sp.Incomplete {
		t.Fatal("caller changed held diagnostic")
	}
	entry.Close()
	files.Changed = true
	if changed, failure := m.registry.Open(context.Background(), id, true); changed != nil || failure != sp.Unsafe {
		t.Fatal("read trusted changed public evidence", changed, failure)
	}
}

func TestPublicRegistryRevalidationReadFailureReleasesItsLock(t *testing.T) {
	m := publicMemoryFixture(t)
	id, _ := m.seed("a")
	before := string(m.files.Sessions[m.index().Project+"/"+id].Data)
	m.files.AfterRead = func() { m.files.ReadFailure = "state.unavailable" }
	if entry, failure := m.registry.Open(context.Background(), id, false); entry != nil || failure != sp.Incomplete {
		t.Fatal("unavailable revalidation acquired a session", entry, failure)
	}
	m.files.AfterRead, m.files.ReadFailure = nil, ""
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	entry, failure := m.registry.Open(ctx, id, false)
	if failure != "" {
		t.Fatal("failed open retained its lock", failure)
	}
	defer entry.Close()
	entry.Intent().Close()
	if failure := entry.Validate(); failure != "" {
		t.Fatal("transaction intent close invalidated its session", failure)
	}
	if len(m.files.Events) != 0 || string(m.files.Sessions[m.index().Project+"/"+id].Data) != before {
		t.Fatal("failed revalidation changed evidence")
	}
}

func TestPublicRegistryRetainsOriginalForegroundAuthority(t *testing.T) {
	store, d, storage := sessionStoreFixture(t)
	r := sessiondisk.Registry{Store: store, Storage: approvaldisk.Registry{Store: storage.Store}, Codec: store.Codec.(sessioncodec.JSON), Identity: canonical.JSON{}}
	id, token := strings.Repeat("b", 64), strings.Repeat("c", 64)
	ctx := context.Background()
	lease, f := r.Register(ctx, d, id, token)
	if f != "" {
		t.Fatal(f)
	}
	defer lease.Close()
	i, f := store.New(ctx, d, token)
	if f != "" {
		t.Fatal(f)
	}
	i.Close()
	original := r.ForRun(d)
	git := filepath.Join(store.Input.Root, ".git")
	if err := os.Rename(git, git+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(git, 0700); err != nil {
		t.Fatal(err)
	}
	current, f := r.Open(ctx, id, true)
	if f != "" {
		t.Fatal("writable Git identity vetoed fresh recovery", f)
	}
	if current.Intent().Binding().Frozen != d.Frozen {
		t.Fatal("fresh recovery lost its original protected binding")
	}
	current.Close()
	owned, f := original.Open(ctx, id, false)
	if f != "" {
		t.Fatal("checkout edits vetoed original foreground cleanup", f)
	}
	defer owned.Close()
	if f = owned.Validate(); f != "" {
		t.Fatal(f)
	}
	record := owned.Record()
	record.State = "stopped"
	if f = owned.Save(record); f != "" {
		t.Fatal(f)
	}
	exit := 7
	record.WorkloadExit = &exit
	if f = owned.Save(record); f != "" {
		t.Fatal("explicit later exit lost", f)
	}
	changed := record
	changed.State = "running"
	if f = owned.Save(changed); f != sp.Refused {
		t.Fatal("completed session reopened", f)
	}
	exit = 0
	if f = owned.Save(record); f != sp.Refused {
		t.Fatal("known result changed", f)
	}
	// The original-binding route still checks the protected bytes on every use.
	path := filepath.Join(storage.Store.StateHome, "itos-container", "sessions", publicIndexKey(store.Input.Root, owned.Intent().Binding().UID), id, "record.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, append(data, ' '), 0600); err != nil {
		t.Fatal(err)
	}
	if f = owned.Validate(); f != sp.Refused {
		t.Fatal("changed public evidence accepted", f)
	}
}
