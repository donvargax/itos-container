package porttest

import (
	"bytes"
	"context"
	budget "github.com/donvargax/itos-container/internal/engine/budget/port"
	"strings"
	"sync"
	"time"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// Registry is a stateful in-memory protected-storage fake. Filesystem faults
// are supplied observations; no process, clock or filesystem is consulted.
type Registry struct {
	mu               sync.Mutex
	admissionGate    chan struct{}
	Admissions       map[string][]byte
	AdmissionFailure port.Failure
	Sessions         map[string]*SessionFiles
	Failure          port.Failure
	Fault            string
	Events           []RegistryEvent
	// A read can become unavailable after a preceding successful observation.
	ReadFailure port.Failure
	AfterRead   func()
}
type RegistryEvent struct {
	ID, Operation string
	Data          []byte
	Failed        bool
}
type SessionFiles struct {
	Data            []byte
	Controller      bool
	Unsafe, Changed bool
	// Log is the published egress-refusals.jsonl, nil until one is published.
	// Staged counts the handles staged and not yet released by Publish or Close.
	Log     []byte
	Staged  int
	readers int
	writer  bool
	changed chan struct{}
}

func registryID(id string) bool { return len(id) == 64 && strings.Trim(id, "0123456789abcdef") == "" }
func (s *Registry) List(ctx context.Context, c port.Candidate) ([]string, port.Failure) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return nil, "state.unavailable"
	}
	if s.Failure != "" {
		return nil, s.Failure
	}
	var ids []string
	for key := range s.Sessions {
		if id, ok := strings.CutPrefix(key, c.Project+"/"); ok {
			ids = append(ids, id)
		}
	}
	return ids, ""
}
func (s *Registry) Register(ctx context.Context, c port.Candidate, id string, data []byte) (port.ControllerLease, port.Failure) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !registryID(id) {
		return nil, "state.invalid"
	}
	if ctx.Err() != nil {
		return nil, "state.unavailable"
	}
	if s.Failure != "" {
		return nil, s.Failure
	}
	if s.Sessions == nil {
		s.Sessions = map[string]*SessionFiles{}
	}
	key := c.Project + "/" + id
	if s.Sessions[key] != nil {
		return nil, "state.conflict"
	}
	f := &SessionFiles{Controller: true}
	s.Sessions[key] = f
	for _, operation := range []string{"lock object durability", "write", "file fsync", "atomic publication", "parent fsync"} {
		failed := s.Fault == operation
		s.Events = append(s.Events, RegistryEvent{ID: id, Operation: operation, Data: bytes.Clone(data), Failed: failed})
		if failed {
			f.Controller = false
			return nil, "state.write"
		}
		if operation == "atomic publication" {
			f.Data = bytes.Clone(data)
		}
	}
	return &registryLease{s: s, files: f}, ""
}

type registryLease struct {
	s     *Registry
	files *SessionFiles
}

func (l *registryLease) Close() { l.s.mu.Lock(); defer l.s.mu.Unlock(); l.files.Controller = false }
func (s *Registry) Open(ctx context.Context, c port.Candidate, id string, readOnly bool) (port.SessionDirectory, port.Failure) {
	if !registryID(id) {
		return nil, "state.invalid"
	}
	s.mu.Lock()
	if s.Failure != "" {
		failure := s.Failure
		s.mu.Unlock()
		return nil, failure
	}
	f := s.Sessions[c.Project+"/"+id]
	if f == nil {
		s.mu.Unlock()
		return nil, ""
	}
	if f.Unsafe {
		s.mu.Unlock()
		return nil, "state.unsafe"
	}
	for {
		if ctx.Err() != nil {
			s.mu.Unlock()
			return nil, "state.unavailable"
		}
		if !f.writer && (readOnly || f.readers == 0) {
			if readOnly {
				f.readers++
			} else {
				f.writer = true
			}
			s.mu.Unlock()
			break
		}
		if f.changed == nil {
			f.changed = make(chan struct{})
		}
		changed := f.changed
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, "state.unavailable"
		case <-changed:
		}
		s.mu.Lock()
	}
	return &registryDirectory{s: s, files: f, id: id, ctx: budget.Lifetime(ctx), readOnly: readOnly}, ""
}

type registryDirectory struct {
	s        *Registry
	files    *SessionFiles
	id       string
	ctx      context.Context
	readOnly bool
}

func (d *registryDirectory) Unchanged() bool {
	return d.ctx.Err() == nil && !d.files.Changed && !d.files.Unsafe
}
func (d *registryDirectory) Close() {
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	if d.readOnly {
		d.files.readers--
	} else {
		d.files.writer = false
	}
	if d.files.changed != nil {
		close(d.files.changed)
		d.files.changed = nil
	}
}
func (d *registryDirectory) Controller() (string, port.Failure) {
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	if !d.Unchanged() {
		return "unknown", "state.path"
	}
	if d.files.Controller {
		return "present", ""
	}
	return "absent", ""
}
func (d *registryDirectory) Read(name string) ([]byte, port.Failure) {
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	if name != "record.json" {
		return nil, "state.invalid"
	}
	if !d.Unchanged() {
		return nil, "state.path"
	}
	if d.s.ReadFailure != "" {
		return nil, d.s.ReadFailure
	}
	if d.s.AfterRead != nil {
		d.s.AfterRead()
	}
	return bytes.Clone(d.files.Data), ""
}
func (d *registryDirectory) Publish(name string, previous, next []byte) port.Failure {
	data, f := d.Read(name)
	if f != "" {
		return f
	}
	if d.readOnly {
		return "state.conflict"
	}
	if len(next) == 0 || len(next) > 1024*1024 {
		return "state.invalid"
	}
	if !bytes.Equal(data, previous) {
		return "state.conflict"
	}
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	for _, operation := range []string{"write", "file fsync", "atomic publication", "parent fsync"} {
		failed := d.s.Fault == operation
		d.s.Events = append(d.s.Events, RegistryEvent{ID: d.id, Operation: operation, Data: bytes.Clone(next), Failed: failed})
		if failed {
			return "state.write"
		}
		if operation == "atomic publication" {
			d.files.Data = bytes.Clone(next)
		}
	}
	return ""
}
func (d *registryDirectory) Sync(name string, expected []byte) port.Failure {
	data, f := d.Read(name)
	if f != "" {
		return f
	}
	if d.readOnly || !bytes.Equal(data, expected) || data == nil {
		return "state.invalid"
	}
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	for _, operation := range []string{"file fsync", "parent fsync"} {
		failed := d.s.Fault == operation
		d.s.Events = append(d.s.Events, RegistryEvent{ID: d.id, Operation: operation, Data: bytes.Clone(data), Failed: failed})
		if failed {
			return "state.write"
		}
	}
	return ""
}

func RegistryStorageContract(t Reporter, open func() (port.RegistryStorage, port.Candidate)) {
	t.Helper()
	s, c := open()
	ctx := context.Background()
	id := strings.Repeat("a", 64)
	if ids, f := s.List(ctx, c); f != "" || len(ids) != 0 {
		t.Errorf("empty discovery: %v %s", ids, f)
		return
	}
	lease, f := s.Register(ctx, c, id, []byte("first"))
	if f != "" {
		t.Errorf("registration: %s", f)
		return
	}
	defer lease.Close()
	if _, f = s.Register(ctx, c, id, []byte("replacement")); f != "state.conflict" {
		t.Errorf("occupied ID: %s", f)
	}
	d, f := s.Open(ctx, c, id, false)
	if f != "" || d == nil {
		t.Errorf("open: %s", f)
		return
	}
	defer d.Close()
	if state, f := d.Controller(); f != "" || state != "present" {
		t.Errorf("live lease: %s %s", state, f)
	}
	lease.Close()
	if state, f := d.Controller(); f != "" || state != "absent" {
		t.Errorf("released lease: %s %s", state, f)
	}
	data, f := d.Read("record.json")
	if f != "" || string(data) != "first" {
		t.Errorf("record read: %q %s", data, f)
	}
	if len(data) > 0 {
		data[0] = 'X'
	}
	if f = d.Publish("record.json", []byte("wrong"), []byte("second")); f != "state.conflict" {
		t.Errorf("stale update: %s", f)
	}
	if f = d.Publish("record.json", []byte("first"), []byte("second")); f != "" {
		t.Errorf("update: %s", f)
	}
	if f = d.Sync("record.json", []byte("second")); f != "" {
		t.Errorf("resync: %s", f)
	}
	other := c
	other.Project = strings.Repeat("b", 64)
	if ids, f := s.List(ctx, other); f != "" || len(ids) != 0 {
		t.Errorf("project separation: %v %s", ids, f)
	}
}

func RegistryReadersContract(t Reporter, open func() (port.RegistryStorage, port.Candidate)) {
	t.Helper()
	s, c := open()
	id := strings.Repeat("a", 64)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	lease, f := s.Register(ctx, c, id, []byte("record"))
	if f != "" {
		t.Errorf("register: %s", f)
		return
	}
	lease.Close()
	a, f := s.Open(ctx, c, id, true)
	if f != "" || a == nil {
		t.Errorf("first reader: %s", f)
		return
	}
	defer a.Close()
	b, f := s.Open(ctx, c, id, true)
	if f != "" || b == nil {
		t.Errorf("second reader: %s", f)
		return
	}
	defer b.Close()
	for _, d := range []port.SessionDirectory{a, b} {
		if state, f := d.Controller(); f != "" || state != "absent" {
			t.Errorf("status reader looked like a controller: %s %s", state, f)
		}
		if f = d.Publish("record.json", []byte("record"), []byte("changed")); f != "state.conflict" {
			t.Errorf("read-only publication: %s", f)
		}
		if f = d.Sync("record.json", []byte("record")); f != "state.invalid" {
			t.Errorf("read-only durability repair: %s", f)
		}
	}
}
