package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	canonical "github.com/donvargax/itos-container/internal/engine/approval/canonical"
	disk "github.com/donvargax/itos-container/internal/engine/approval/disk"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	aptest "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	manifestjson "github.com/donvargax/itos-container/internal/engine/manifest/jsonc"
	readinessdisk "github.com/donvargax/itos-container/internal/engine/readiness/disk"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	sc "github.com/donvargax/itos-container/internal/engine/session/codec"
	sd "github.com/donvargax/itos-container/internal/engine/session/disk"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	st "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func sessionStoreFixture(t *testing.T) (sd.Store, ap.Durable, disk.Storage) {
	t.Helper()
	if runtime.GOOS != "linux" || os.Geteuid() == 0 {
		t.Skip("native non-root protected-storage fixture")
	}
	dir := t.TempDir()
	root := filepath.Join(dir, "checkout")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"objects", "refs"} {
		if err := os.Mkdir(filepath.Join(root, ".git", dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"HEAD", "config"} {
		if err := os.WriteFile(filepath.Join(root, ".git", name), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	identity := rp.CheckoutIdentity{Root: root}
	for _, entry := range []struct {
		path          string
		device, inode *string
	}{{root, &identity.Device, &identity.Inode}, {filepath.Join(root, ".git"), &identity.GitDevice, &identity.GitInode}} {
		info, err := os.Stat(entry.path)
		if err != nil {
			t.Fatal(err)
		}
		stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
		*entry.device = strconv.FormatUint(stat.FieldByName("Dev").Uint(), 10)
		*entry.inode = strconv.FormatUint(stat.FieldByName("Ino").Uint(), 10)
	}
	uid, gid := strconv.Itoa(os.Geteuid()), strconv.Itoa(os.Getegid())
	ready := aptest.Ready()
	ready.Checkout = &identity
	candidate := canonical.JSON{}.Candidate(ready, uid, gid)
	record := ap.Record{Candidate: candidate, Generation: "1"}
	state := filepath.Join(dir, "state")
	snapshot := filepath.Join(state, "itos-container", "snapshots", candidate.Project, "1.json")
	if err := os.MkdirAll(filepath.Dir(snapshot), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshot, canonical.JSON{}.Encode(record), 0600); err != nil {
		t.Fatal(err)
	}
	storage := disk.Storage{Store: disk.Store{StateHome: state}}
	codec := sc.JSON{Approval: canonical.JSON{}, Parser: manifestjson.Parser{}}
	store := sd.Store{Storage: storage, Codec: codec, Host: &aptest.Host{UID: uid, GID: gid}, Checkout: &readinessdisk.Checkout{}, Input: rp.Input{Root: root, OS: "linux", Architecture: "amd64"}}
	return store, ap.Durable{Frozen: ap.Frozen{Project: candidate.Project, Generation: "1", Fingerprint: candidate.Fingerprint, Settings: candidate.Canonical}, Reference: snapshot}, storage
}
func TestSessionStoreContracts(t *testing.T) {
	t.Run("disk", func(t *testing.T) {
		st.StoreContract(t, func() (sp.Store, ap.Durable) { store, d, _ := sessionStoreFixture(t); return store, d })
	})
	t.Run("memory", func(t *testing.T) {
		st.StoreContract(t, func() (sp.Store, ap.Durable) { b, store, _ := st.Fixture(); return store, ap.Durable{Frozen: b.Frozen} })
	})
}

type sessionOwnerInfo struct {
	os.FileInfo
	metadata any
}

func (i sessionOwnerInfo) Sys() any { return i.metadata }

// Only cmd composes concrete adapters. This unit test exercises the session disk
// store with the real protected file operations, injecting ONLY record metadata.
func TestSessionDiskRecordOwnerMetadata(t *testing.T) {
	store, d, storage := sessionStoreFixture(t)
	token := strings.Repeat("1", 64)
	intent, f := store.New(context.Background(), d, token)
	if f != "" {
		t.Fatal(f)
	}
	intent.Close()
	ref := sp.Reference(d.Frozen.Project, token)
	baseline, f := store.Open(context.Background(), ref)
	if f != "" {
		t.Fatal(f)
	}
	baseline.Close()
	uidBefore, gidBefore := store.Host.Identity()
	recordReached := false
	storage.Stat = func(file *os.File) (os.FileInfo, error) {
		info, err := file.Stat()
		if err != nil {
			return nil, err
		}
		if filepath.Base(file.Name()) != token+".json" {
			return info, nil
		}
		recordReached = true
		original := reflect.Indirect(reflect.ValueOf(info.Sys()))
		metadata := reflect.New(original.Type())
		metadata.Elem().Set(original)
		metadata.Elem().FieldByName("Uid").SetUint(uint64(os.Geteuid() + 1))
		return sessionOwnerInfo{FileInfo: info, metadata: metadata.Interface()}, nil
	}
	store.Storage = storage
	refused, f := store.Open(context.Background(), ref)
	if refused != nil || f != sp.Unsafe || !recordReached {
		t.Fatalf("record owner validation = %v %s (reached %v)", refused, f, recordReached)
	}
	uidAfter, gidAfter := store.Host.Identity()
	if uidBefore != uidAfter || gidBefore != gidAfter {
		t.Fatal("host identity changed")
	}
	// The same actual record and original snapshot remain valid without the seam.
	storage.Stat = nil
	store.Storage = storage
	valid, f := store.Open(context.Background(), ref)
	if f != "" {
		t.Fatal(f)
	}
	valid.Close()
}
func TestSessionDiskRejectsInvalidLookupsAndBinding(t *testing.T) {
	store, d, _ := sessionStoreFixture(t)
	for _, ref := range []string{"../x", "intents/../a.json", "intents/" + d.Frozen.Project + "/../a.json", "intents/" + d.Frozen.Project + "/short.json"} {
		if _, f := store.Open(context.Background(), ref); f != sp.Invalid {
			t.Fatal(ref, f)
		}
	}
	bad := d
	bad.Frozen.Settings = "{}"
	if _, f := store.New(context.Background(), bad, strings.Repeat("1", 64)); f != sp.Invalid {
		t.Fatal(f)
	}
	if _, f := store.New(context.Background(), d, "bad"); f != sp.Invalid {
		t.Fatal(f)
	}
	store.Input.OS = "windows"
	if _, f := store.New(context.Background(), d, strings.Repeat("1", 64)); f != sp.Refused {
		t.Fatal(f)
	}
}
