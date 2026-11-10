package disk

import (
	"context"
	"errors"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// Registry holds project locks only for directory discovery/publication. Engine
// calls happen with a session operation lock, never with a project storage lock.
type Registry struct {
	Store     Store
	IO        SnapshotIO
	Stat      func(*os.File) (os.FileInfo, error)
	Exclusive func(*os.Root, string, string) error
}

func (s Registry) store(ctx context.Context) Store {
	store := s.Store
	ops := store.operations()
	if s.IO.Create != nil {
		ops.create = s.IO.Create
	}
	if s.IO.Write != nil {
		ops.write = s.IO.Write
	}
	if s.IO.FileSync != nil {
		ops.syncFile = s.IO.FileSync
	}
	if s.IO.Rename != nil {
		ops.rename = s.IO.Rename
	}
	if s.IO.ParentSync != nil {
		ops.syncParent = s.IO.ParentSync
	}
	if s.IO.DirectorySync != nil {
		ops.syncDir = s.IO.DirectorySync
	}
	if s.Stat != nil {
		ops.statFile = s.Stat
	}
	ops.lock = func(f *os.File, _ bool) error { return lockContext(ctx, f) }
	store.ops = &ops
	return store
}
func publicID(id string) bool { return len(id) == 64 && strings.Trim(id, "0123456789abcdef") == "" }

func (s Registry) List(ctx context.Context, c port.Candidate) ([]string, port.Failure) {
	p, failure := s.store(ctx).pinAt(c, false, "sessions")
	if failure != "" || p == nil {
		return nil, failure
	}
	defer p.close()
	entries, err := p.dir.ReadDir(-1)
	if err != nil {
		return nil, "state.unavailable"
	}
	if !p.unchanged() {
		return nil, "state.path"
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names, ""
}

func (s Registry) Register(ctx context.Context, c port.Candidate, id string, data []byte) (port.ControllerLease, port.Failure) {
	if !publicID(id) {
		return nil, "state.invalid"
	}
	store := s.store(ctx)
	project, failure := store.pinAt(c, true, "sessions")
	if failure != "" {
		return nil, failure
	}
	defer project.close()
	if _, err := project.ops.lstat(project.root(), id); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return nil, "state.unavailable"
		}
		return nil, "state.conflict"
	}
	// Parent is locked across exclusive directory creation. A retained directory
	// is never reused, even when its preceding registration was interrupted.
	p, failure := store.pinPath(c, true, filepath.Join(project.path, id), 4)
	if failure != "" {
		return nil, failure
	}
	defer p.close()
	exclusive := s.Exclusive
	if exclusive == nil {
		exclusive = (*os.Root).Link
	}
	d := &sessionDirectory{directoryStore: directoryStore{pin: p, ctx: budget.Lifetime(ctx), public: true, exclusive: exclusive}}
	controller, failure := d.openLock("controller.lock", true)
	if failure != "" {
		return nil, failure
	}
	success := false
	defer func() {
		if !success {
			_ = controller.Close()
		}
	}()
	if tryLock(controller) != nil {
		return nil, "state.conflict"
	}
	operation, failure := d.openLock("operation.lock", true)
	if failure != "" {
		return nil, failure
	}
	d.controller, d.operation = controller, operation
	defer func() { _ = operation.Close() }()
	if !d.Unchanged() {
		return nil, "state.path"
	}
	if failure = d.Publish("record.json", nil, data); failure != "" {
		return nil, failure
	}
	if !d.Unchanged() || ctx.Err() != nil {
		return nil, "state.unavailable"
	}
	success = true
	return &controllerLease{controller}, ""
}

type controllerLease struct{ file *os.File }

func (l *controllerLease) Close() { _ = unlock(l.file); _ = l.file.Close() }

func (s Registry) Open(ctx context.Context, c port.Candidate, id string, readOnly bool) (port.SessionDirectory, port.Failure) {
	if !publicID(id) {
		return nil, "state.invalid"
	}
	store := s.store(ctx)
	path, failure := store.locationAt(c, "sessions")
	if failure != "" {
		return nil, failure
	}
	// No directory lock spans the operation. Only the persistent operation file
	// serializes readers and writers; all ancestors remain pinned and checked.
	ops := *store.ops
	ops.lock = func(*os.File, bool) error { return nil }
	store.ops = &ops
	p, failure := store.pinPath(c, false, filepath.Join(path, id), 4)
	if failure != "" || p == nil {
		return nil, failure
	}
	d := &sessionDirectory{directoryStore: directoryStore{pin: p, ctx: budget.Lifetime(ctx), public: true, exclusive: (*os.Root).Link}, readOnly: readOnly}
	success := false
	defer func() {
		if !success {
			d.Close()
		}
	}()
	d.controller, failure = d.openLock("controller.lock", false)
	if failure != "" {
		return nil, failure
	}
	d.operation, failure = d.openLock("operation.lock", false)
	if failure != "" {
		return nil, failure
	}
	if err := sessionLock(ctx, d.operation, readOnly); err != nil {
		return nil, "state.unavailable"
	}
	if !d.Unchanged() {
		return nil, "state.path"
	}
	success = true
	return d, ""
}

type sessionDirectory struct {
	directoryStore
	controller, operation *os.File
	readOnly              bool
}

func (d *sessionDirectory) Publish(name string, previous, next []byte) port.Failure {
	if d.readOnly {
		return "state.conflict"
	}
	if !d.Unchanged() {
		return "state.path"
	}
	return d.directoryStore.Publish(name, previous, next)
}
func (d *sessionDirectory) Sync(name string, expected []byte) port.Failure {
	if d.readOnly {
		return "state.invalid"
	}
	if !d.Unchanged() {
		return "state.path"
	}
	return d.directoryStore.Sync(name, expected)
}

func (d *sessionDirectory) openLock(name string, create bool) (*os.File, port.Failure) {
	p := d.pin
	flags := os.O_RDONLY | noFollow
	if create {
		flags = os.O_RDWR | os.O_CREATE | os.O_EXCL | noFollow
	}
	file, err := p.ops.openRecord(p.root(), name, flags, 0600)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "state.invalid"
		}
		return nil, "state.path"
	}
	success := false
	defer func() {
		if !success {
			_ = file.Close()
		}
	}()
	if create && p.ops.chmod(file, 0600) != nil {
		return nil, "state.write"
	}
	info, err := p.ops.statFile(file)
	if err != nil || recordSafety(info, p.uid) != "" || info.Size() != 0 {
		return nil, "state.unsafe"
	}
	latest, err := p.ops.lstat(p.root(), name)
	if err != nil || !os.SameFile(info, latest) || recordSafety(latest, p.uid) != "" {
		return nil, "state.path"
	}
	if create && (p.ops.syncFile(file) != nil || p.ops.syncParent(p.dir) != nil) {
		return nil, "state.write"
	}
	success = true
	return file, ""
}
func (d *sessionDirectory) Unchanged() bool {
	if !d.directoryStore.Unchanged() {
		return false
	}
	for name, file := range map[string]*os.File{"controller.lock": d.controller, "operation.lock": d.operation} {
		if file == nil {
			continue
		}
		info, err := d.pin.ops.statFile(file)
		if err != nil || recordSafety(info, d.pin.uid) != "" || info.Size() != 0 {
			return false
		}
		latest, err := d.pin.ops.lstat(d.pin.root(), name)
		if err != nil || !os.SameFile(info, latest) || recordSafety(latest, d.pin.uid) != "" {
			return false
		}
	}
	return true
}
func (d *sessionDirectory) Controller() (string, port.Failure) {
	if !d.Unchanged() {
		return "unknown", "state.path"
	}
	err := trySharedLock(d.controller)
	if lockBusy(err) {
		return "present", ""
	}
	if err != nil {
		return "unknown", "state.unavailable"
	}
	if unlock(d.controller) != nil || !d.Unchanged() {
		return "unknown", "state.unavailable"
	}
	return "absent", ""
}
func (d *sessionDirectory) Close() {
	if d.operation != nil {
		_ = unlock(d.operation)
		_ = d.operation.Close()
	}
	if d.controller != nil {
		_ = d.controller.Close()
	}
	d.directoryStore.Close()
}
func sessionLock(ctx context.Context, file *os.File, shared bool) error {
	ctx, cancel := context.WithTimeout(ctx, budget.CallDuration)
	defer cancel()
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var err error
		if shared {
			err = trySharedLock(file)
		} else {
			err = tryLock(file)
		}
		if err == nil || !lockBusy(err) {
			return err
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
