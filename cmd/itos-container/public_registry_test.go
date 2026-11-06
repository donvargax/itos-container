package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	canonical "github.com/donvargax/itos-container/internal/engine/approval/canonical"
	approvaldisk "github.com/donvargax/itos-container/internal/engine/approval/disk"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
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
