package main

import (
	"bytes"
	"strings"
	"testing"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestPublicRegistryAcquisitionFailures(t *testing.T) {
	for _, fault := range []string{"unsupported host", "unavailable root", "unavailable storage", "invalid frozen", "foreign frozen host", "invalid public ID", "invalid token", "foreign project", "different root"} {
		t.Run(fault, func(t *testing.T) {
			m := publicMemoryFixture(t)
			id, _ := m.seed("a")
			frozen := ap.Durable{Frozen: m.b.Frozen}
			token := strings.Repeat("b", 64)
			want := sp.Refused
			registry := sp.Registry(m.registry)
			switch fault {
			case "unsupported host":
				m.host.UID = "0"
			case "unavailable root":
				m.checkout.Failure = rp.CheckoutUnavailable
			case "unavailable storage":
				m.files.Failure = "state.unavailable"
				want = sp.IntentWrite
			case "invalid frozen":
				frozen.Frozen.Settings = "broken"
				registry = m.registry.ForRun(frozen)
				want = sp.Invalid
			case "foreign frozen host":
				m.host.GID = "1001"
				registry = m.registry.ForRun(frozen)
			case "invalid public ID":
				id = "../outside"
			case "invalid token":
				token = "../outside"
			case "foreign project":
				frozen.Frozen.Project = strings.Repeat("e", 64)
			case "different root":
				m.checkout.Identity.Root = "/different-checkout"
			}
			if lease, f := registry.Register(t.Context(), frozen, id, token); lease != nil || f != want {
				t.Fatalf("registration = %v, %s; want %s", lease, f, want)
			}
			if len(m.files.Events) != 0 {
				t.Fatal("invalid registration wrote protected state")
			}
			if fault == "unavailable storage" {
				want = sp.Incomplete
			}
			if fault == "unsupported host" || fault == "unavailable root" || fault == "unavailable storage" || fault == "invalid frozen" || fault == "foreign frozen host" {
				if ids, f := registry.List(t.Context()); ids != nil || f != want {
					t.Fatal("discovery accepted unusable authority", ids, f)
				}
				if entry, f := registry.Open(t.Context(), id, true); entry != nil || f != want {
					t.Fatal("open accepted unusable authority", entry, f)
				}
			}
		})
	}
}

func TestPublicRegistryRetainsBytesAndAuthorityAfterWriteFailures(t *testing.T) {
	for _, fault := range []string{"read only", "unsafe locks", "replaced record", "invalid next", "file fsync", "parent fsync", "sync failure", "changed intent"} {
		t.Run(fault, func(t *testing.T) {
			m := publicMemoryFixture(t)
			id, _ := m.seed("a")
			entry, f := m.registry.Open(t.Context(), id, fault == "read only")
			if f != "" {
				t.Fatal(f)
			}
			defer entry.Close()
			files := m.files.Sessions[m.index().Project+"/"+id]
			before := bytes.Clone(files.Data)
			next := entry.Record()
			next.State = "stopping"
			want := sp.Refused
			switch fault {
			case "unsafe locks":
				files.Changed = true
				if state, f := entry.Controller(); state != "unknown" || f != sp.Unsafe {
					t.Fatal(state, f)
				}
			case "replaced record":
				files.Data = append(files.Data, ' ')
				before = bytes.Clone(files.Data)
			case "invalid next":
				next.State = "unknown state"
				want = sp.Invalid
			case "file fsync", "parent fsync", "sync failure":
				m.files.Fault = fault
				want = sp.IntentWrite
				if fault == "sync failure" {
					m.files.Fault = "file fsync"
					next = entry.Record()
				}
			case "changed intent":
				r := entry.Intent().Record()
				r.ContainerID = strings.Repeat("f", 64)
				m.storage.Directories["intents/"+m.c.Project].Files[r.NameToken()+".json"] = m.registry.Store.Codec.Encode(r)
			}
			if f := entry.Save(next); f != want {
				t.Fatalf("save with %s = %s; want %s", fault, f, want)
			}
			if fault == "parent fsync" {
				if entry.Record().State != "stopping" || !bytes.Equal(files.Data, m.registry.Codec.EncodePublic(next)) {
					t.Fatal("visible uncertain publication lost its complete representation")
				}
				m.files.Fault = ""
				if f := entry.Sync(); f != "" {
					t.Fatal("could not re-establish uncertain durability", f)
				}
			} else if !bytes.Equal(files.Data, before) {
				t.Fatal("failed update changed existing public evidence")
			}
			if fault == "read only" || fault == "unsafe locks" || fault == "replaced record" || fault == "changed intent" {
				if f := entry.Sync(); f != sp.Refused {
					t.Fatal("invalid or read-only session repaired durability", f)
				}
			}
		})
	}
}
