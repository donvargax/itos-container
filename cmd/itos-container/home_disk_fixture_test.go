package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	ad "github.com/donvargax/itos-container/internal/engine/approval/disk"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	sd "github.com/donvargax/itos-container/internal/engine/session/disk"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

// Synthetic UID/GID cases map filesystem ownership to the test process only at
// the filesystem port boundary. Protected records, scope keys, archive headers
// and worker users retain the synthetic approved IDs. The adapter still applies
// real no-follow, link, mode, inode, lock and durable-publication checks.
type homeFixtureStorage struct{ ap.Storage }

func (s homeFixtureStorage) Open(ctx context.Context, c ap.Candidate, k string, create bool) (ap.Directory, ap.Failure) {
	c.UID = strconv.Itoa(os.Geteuid())
	return s.Storage.Open(ctx, c, k, create)
}

type homeFixtureRegistry struct{ ap.RegistryStorage }

func (s homeFixtureRegistry) Admission(ctx context.Context, c ap.Candidate) (ap.AdmissionDirectory, ap.Failure) {
	c.UID = strconv.Itoa(os.Geteuid())
	return s.RegistryStorage.Admission(ctx, c)
}
func (s homeFixtureRegistry) List(ctx context.Context, c ap.Candidate) ([]string, ap.Failure) {
	c.UID = strconv.Itoa(os.Geteuid())
	return s.RegistryStorage.List(ctx, c)
}
func (s homeFixtureRegistry) Register(ctx context.Context, c ap.Candidate, id string, data []byte) (ap.ControllerLease, ap.Failure) {
	c.UID = strconv.Itoa(os.Geteuid())
	return s.RegistryStorage.Register(ctx, c, id, data)
}
func (s homeFixtureRegistry) Open(ctx context.Context, c ap.Candidate, id string, read bool) (ap.SessionDirectory, ap.Failure) {
	c.UID = strconv.Itoa(os.Geteuid())
	return s.RegistryStorage.Open(ctx, c, id, read)
}

type homeFixtureCatalog struct{ ap.CatalogStorage }

func (s homeFixtureCatalog) List(ctx context.Context, _ string) ([]string, ap.Failure) {
	return s.CatalogStorage.List(ctx, strconv.Itoa(os.Geteuid()))
}
func (s homeFixtureCatalog) Open(ctx context.Context, _, key string, create bool) (ap.Directory, ap.Failure) {
	return s.CatalogStorage.Open(ctx, strconv.Itoa(os.Geteuid()), key, create)
}

type homeFixtureAccount string

func (s homeFixtureAccount) AccountHome() (string, ap.Failure) { return string(s), "" }

type homeFixtureLocations struct{ ad.Locations }

func (s homeFixtureLocations) At(root string) ap.Storage {
	return homeFixtureStorage{s.Locations.At(root)}
}
func (s homeFixtureLocations) Inspect(ctx context.Context, c ap.Candidate, root string) (ap.AdmissionDirectory, ap.Failure) {
	c.UID = strconv.Itoa(os.Geteuid())
	return s.Locations.Inspect(ctx, c, root)
}

type homeDiskFixture struct {
	t                      *testing.T
	f                      *homeLocalFixture
	root, account, catalog string
	copied                 map[string]string
	store                  sd.Homes
}

func homeDisk(t *testing.T, f *homeLocalFixture) *homeDiskFixture {
	t.Helper()
	root := t.TempDir()
	account := filepath.Join(root, "account")
	if err := os.Mkdir(account, 0700); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(root, "state")
	if err := os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	protected := ad.Store{StateHome: state, Home: account}
	d := &homeDiskFixture{t: t, f: f, root: filepath.Join(state, "itos-container"), account: account, catalog: filepath.Join(account, ".local", "state", "itos-container", "homes"), copied: map[string]string{}}
	f.m.registry.Store.Storage = homeFixtureStorage{ad.Storage{Store: protected}}
	f.m.registry.Storage = homeFixtureRegistry{ad.Registry{Store: protected}}
	d.store = sd.Homes{Storage: homeFixtureCatalog{ad.Catalog{Account: homeFixtureAccount(account), Store: protected}}, Codec: f.m.registry.Store.Codec.(sp.HomeCodec), Host: f.m.host, StateRoot: d.root, Locations: homeFixtureLocations{ad.Locations{Store: protected}}}
	return d
}
func (d *homeDiskFixture) write(path string, data []byte) {
	d.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		d.t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		d.t.Fatal(err)
	}
}
func (d *homeDiskFixture) copy(path string, data []byte) {
	if previous, ok := d.copied[path]; ok && previous == string(data) {
		return
	}
	d.write(path, data)
	d.copied[path] = string(data)
}
func (d *homeDiskFixture) seed() {
	for key, dir := range d.f.m.storage.Directories {
		for name, data := range dir.Files {
			d.copy(filepath.Join(d.root, key, name), data)
		}
	}
	for key, file := range d.f.m.files.Sessions {
		d.copy(filepath.Join(d.root, "sessions", key, "record.json"), file.Data)
		d.copy(filepath.Join(d.root, "sessions", key, "controller.lock"), []byte{})
		d.copy(filepath.Join(d.root, "sessions", key, "operation.lock"), []byte{})
	}
	for name, data := range d.f.m.files.Admissions {
		d.copy(filepath.Join(d.root, "admission", name), data)
	}
	for key, s := range d.f.homes.Scopes {
		d.copy(filepath.Join(d.catalog, key, key+".json"), homeFixtureCodec{}.EncodeHome(s))
	}
}
func (d *homeDiskFixture) scope(key string) sp.HomeScope {
	data, err := os.ReadFile(filepath.Join(d.catalog, key, key+".json"))
	if os.IsNotExist(err) {
		return d.f.homes.Scopes[key]
	}
	if err != nil {
		d.t.Fatal(err)
	}
	var s sp.HomeScope
	if err = json.Unmarshal(data, &s); err != nil {
		d.t.Fatal(err)
	}
	return s
}
func (d *homeDiskFixture) put(s sp.HomeScope) {
	d.copy(filepath.Join(d.catalog, s.Scope, s.Scope+".json"), homeFixtureCodec{}.EncodeHome(s))
}
func (d *homeDiskFixture) Scope(b sp.Binding) string { return d.store.Scope(b) }
func (d *homeDiskFixture) List(ctx context.Context) ([]string, sp.Outcome) {
	if d.f.homes.Failure != "" {
		return nil, d.f.homes.Failure
	}
	return d.store.List(ctx)
}
func (d *homeDiskFixture) Open(ctx context.Context, key string, create bool) (sp.HomeTransaction, sp.Outcome) {
	if d.f.homes.Failure != "" {
		return nil, d.f.homes.Failure
	}
	t, f := d.store.Open(ctx, key, create)
	if t == nil || f != "" {
		return t, f
	}
	return homeFixtureTransaction{HomeTransaction: t, d: d}, f
}
func (d *homeDiskFixture) Claim(r sp.Record, id string) (sp.HomeClaim, sp.Outcome) {
	return d.store.Claim(r, id)
}
func (d *homeDiskFixture) CheckClaims(ctx context.Context, s sp.HomeScope, g sp.HomeGeneration) sp.Outcome {
	if d.f.homes.ClaimsFailure != "" {
		return d.f.homes.ClaimsFailure
	}
	return d.store.CheckClaims(ctx, s, g)
}

type homeFixtureTransaction struct {
	sp.HomeTransaction
	d *homeDiskFixture
}

func (t homeFixtureTransaction) Read() (sp.HomeScope, sp.Outcome) {
	if t.d.f.homes.Failure != "" {
		return sp.HomeScope{}, t.d.f.homes.Failure
	}
	return t.HomeTransaction.Read()
}
func (t homeFixtureTransaction) Save(s sp.HomeScope) sp.Outcome {
	if t.d.f.homes.BeforeSave != nil {
		if f := t.d.f.homes.BeforeSave(s); f != "" {
			return f
		}
	}
	return t.HomeTransaction.Save(s)
}

func (s homeFixtureStorage) ReadOriginal(ctx context.Context, c ap.Candidate, collection, name string) ([]byte, ap.Failure) {
	c.UID = strconv.Itoa(os.Geteuid())
	return s.Storage.(ap.OriginalStorage).ReadOriginal(ctx, c, collection, name)
}
