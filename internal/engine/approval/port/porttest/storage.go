package porttest

import (
	"bytes"
	"context"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// Storage models locked protected collections with exact compare/publication.
// ReadFailure may describe unsafe injected metadata; the fake does no I/O.
type Storage struct {
	Directories map[string]*Directory
	Failure     port.Failure
}
type Directory struct {
	Files                                  map[string][]byte
	ReadFailure, WriteFailure, SyncFailure port.Failure
	Changed, Locked                        bool
}

func (s *Storage) Open(ctx context.Context, c port.Candidate, collection string, create bool) (port.Directory, port.Failure) {
	if ctx.Err() != nil {
		return nil, "state.unavailable"
	}
	if s.Failure != "" {
		return nil, s.Failure
	}
	if (collection != "intents" && collection != "snapshots") || (create && collection != "intents") {
		return nil, "state.invalid"
	}
	if s.Directories == nil {
		s.Directories = map[string]*Directory{}
	}
	key := collection + "/" + c.Project
	d := s.Directories[key]
	if d == nil && create {
		d = &Directory{Files: map[string][]byte{}}
		s.Directories[key] = d
	}
	if d == nil {
		return nil, ""
	}
	if d.Locked {
		return nil, "state.unavailable"
	}
	d.Locked = true
	return d, ""
}
func (d *Directory) Close()          { d.Locked = false }
func (d *Directory) Unchanged() bool { return !d.Changed }
func (d *Directory) Read(name string) ([]byte, port.Failure) {
	if !strings.HasSuffix(name, ".json") || len(name) <= 5 || strings.Trim(strings.TrimSuffix(name, ".json"), "0123456789abcdef") != "" {
		return nil, "state.invalid"
	}
	if d.Changed {
		return nil, "state.path"
	}
	if d.ReadFailure != "" {
		return nil, d.ReadFailure
	}
	if d.Files[name] == nil {
		return nil, ""
	}
	return append([]byte{}, d.Files[name]...), ""
}
func (d *Directory) Publish(name string, previous, next []byte) port.Failure {
	if len(next) == 0 || len(next) > 1024*1024 {
		return "state.invalid"
	}
	old, f := d.Read(name)
	if f != "" {
		return f
	}
	if !bytes.Equal(old, previous) || ((previous == nil) != (old == nil)) {
		return "state.conflict"
	}
	if d.WriteFailure != "" {
		return d.WriteFailure
	}
	d.Files[name] = append([]byte{}, next...)
	return ""
}
func (d *Directory) Sync(name string, expected []byte) port.Failure {
	old, f := d.Read(name)
	if f != "" {
		return f
	}
	if old == nil || !bytes.Equal(old, expected) {
		return "state.invalid"
	}
	return d.SyncFailure
}

// StorageContract verifies exclusive publish, exact compare and independent bytes.
func StorageContract(t Reporter, open func() (port.Storage, port.Candidate)) {
	t.Helper()
	storage, c := open()
	ctx := context.Background()
	d, f := storage.Open(ctx, c, "intents", true)
	if f != "" || d == nil {
		t.Errorf("open %s", f)
		return
	}
	defer d.Close()
	if _, f = d.Read("../escape"); f != "state.invalid" {
		t.Errorf("path accepted %s", f)
	}
	if data, f := d.Read("1.json"); f != "" || data != nil {
		t.Errorf("missing read %s", f)
	}
	if f = d.Publish("1.json", nil, []byte("first")); f != "" {
		t.Errorf("publish %s", f)
	}
	if f = d.Publish("1.json", nil, []byte("replacement")); f != "state.conflict" {
		t.Errorf("exclusive publication %s", f)
	}
	data, f := d.Read("1.json")
	if f != "" || string(data) != "first" {
		t.Errorf("read %s", f)
	}
	if len(data) > 0 {
		data[0] = 'X'
	}
	if f = d.Publish("1.json", []byte("first"), []byte("second")); f != "" {
		t.Errorf("advance %s", f)
	}
	if f = d.Sync("1.json", []byte("wrong")); f != "state.invalid" {
		t.Errorf("sync wrong bytes %s", f)
	}
	if f = d.Sync("1.json", []byte("second")); f != "" {
		t.Errorf("sync %s", f)
	}
}
