package disk

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
)

// Storage exposes the existing protected directory machinery through a port.
// File operations remain injectable; callers cannot disable validation.
type Storage struct {
	Store     Store
	IO        SnapshotIO
	Stat      func(*os.File) (os.FileInfo, error)
	Exclusive func(*os.Root, string, string) error
}

func (s Storage) Open(ctx context.Context, c port.Candidate, collection string, create bool) (port.Directory, port.Failure) {
	if (collection != "intents" && collection != "snapshots") || (create && collection != "intents") {
		return nil, "state.invalid"
	}
	if ctx.Err() != nil {
		return nil, "state.unavailable"
	}
	transaction := port.ControlTransaction(localTransaction{})
	if create {
		var failure port.Failure
		transaction, failure = s.Store.control(ctx, c, true)
		if failure != "" {
			return nil, failure
		}
	}
	success := false
	defer func() {
		if !success {
			transaction.Close()
		}
	}()
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
	p, failure := store.pinAt(c, create, collection)
	if failure != "" || p == nil {
		return nil, failure
	}
	exclusive := s.Exclusive
	if exclusive == nil {
		exclusive = (*os.Root).Link
	}
	success = true
	return &directoryStore{pin: p, ctx: budget.Lifetime(ctx), exclusive: exclusive, control: transaction, releaseControl: create}, ""
}
func lockBusy(err error) bool { return errors.Is(err, syscall.EWOULDBLOCK) }
func lockContext(ctx context.Context, file *os.File) error {
	return sessionLock(ctx, file, false)
}

type directoryStore struct {
	pin            *pin
	ctx            context.Context
	exclusive      func(*os.Root, string, string) error
	public         bool
	control        port.ControlTransaction
	releaseControl bool
}

func (d *directoryStore) Close() {
	d.pin.close()
	if d.control != nil {
		d.control.Close()
	}
}
func (d *directoryStore) Unchanged() bool {
	return d.ctx.Err() == nil && d.pin.unchanged() && (d.control == nil || d.control.Unchanged())
}
func recordName(name string) bool {
	return strings.HasSuffix(name, ".json") && len(name) > 5 && strings.Trim(strings.TrimSuffix(name, ".json"), "0123456789abcdef") == ""
}
func (d *directoryStore) Read(name string) ([]byte, port.Failure) {
	p := d.pin
	if !recordName(name) && (!d.public || name != "record.json") {
		return nil, "state.invalid"
	}
	if !d.Unchanged() {
		return nil, "state.path"
	}
	info, err := p.ops.lstat(p.root(), name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ""
	}
	if err != nil {
		return nil, "state.unavailable"
	}
	links := p.recordLinks(info)
	if failure := linkedRecordSafety(info, p.uid, links); failure != "" {
		return nil, failure
	}
	file, err := p.ops.openRecord(p.root(), name, os.O_RDONLY|noFollow, 0)
	if err != nil {
		return nil, "state.path"
	}
	defer func() { _ = file.Close() }()
	actual, err := p.ops.statFile(file)
	if err != nil {
		return nil, "state.path"
	}
	if failure := linkedRecordSafety(actual, p.uid, links); failure != "" {
		return nil, failure
	}
	if !os.SameFile(info, actual) {
		return nil, "state.path"
	}
	data, err := p.ops.readAll(io.LimitReader(file, 1024*1024+1))
	if err != nil {
		return nil, "state.unavailable"
	}
	if len(data) > 1024*1024 {
		return nil, "state.invalid"
	}
	latest, err := p.ops.lstat(p.root(), name)
	if err != nil || !os.SameFile(actual, latest) || linkedRecordSafety(latest, p.uid, links) != "" || !d.Unchanged() {
		return nil, "state.path"
	}
	return data, ""
}
func (d *directoryStore) Publish(name string, previous, next []byte) port.Failure {
	// Initial intent publication ends the common transaction before its caller
	// can perform engine I/O. The selected directory remains independently pinned.
	if d.releaseControl && d.control != nil {
		defer func() { d.control.Close(); d.control = nil }()
	}
	if len(next) == 0 || len(next) > 1024*1024 {
		return "state.invalid"
	}
	old, failure := d.Read(name)
	if failure != "" {
		return failure
	}
	if !bytes.Equal(old, previous) || ((previous == nil) != (old == nil)) {
		return "state.conflict"
	}
	p := d.pin
	// Re-establish directory durability after any earlier uncertain mkdir/fsync.
	for _, root := range p.roots {
		file, err := p.ops.openDir(root, ".")
		if err != nil {
			return "state.write"
		}
		err = p.ops.syncDir(file)
		_ = file.Close()
		if err != nil {
			return "state.write"
		}
	}
	var nonce [16]byte
	if _, err := p.ops.nonce(nonce[:]); err != nil {
		return "state.write"
	}
	temporary := ".intent-" + hex.EncodeToString(nonce[:]) + ".tmp"
	file, err := p.ops.create(p.root(), temporary)
	if err != nil {
		return "state.write"
	}
	defer func() { _ = file.Close(); _ = p.root().Remove(temporary) }()
	if p.ops.chmod(file, 0600) != nil {
		return "state.write"
	}
	n, err := p.ops.write(file, next)
	if err != nil || n != len(next) {
		return "state.write"
	}
	if p.ops.syncFile(file) != nil || p.ops.closeFile(file) != nil {
		return "state.write"
	}
	if !d.Unchanged() {
		return "state.path"
	}
	old, failure = d.Read(name)
	if failure != "" {
		return failure
	}
	if !bytes.Equal(old, previous) || ((previous == nil) != (old == nil)) {
		return "state.conflict"
	}
	if previous == nil {
		// Atomic exclusive linking cannot overwrite a name that appeared after Read.
		// Drop the temporary link before any observer can accept the final record.
		if err = d.exclusive(p.root(), temporary, name); err != nil {
			if errors.Is(err, os.ErrExist) {
				return "state.conflict"
			}
			return "state.write"
		}
		if p.root().Remove(temporary) != nil {
			return "state.write"
		}
	} else if p.ops.rename(p.root(), temporary, name) != nil {
		return "state.write"
	}
	if p.ops.syncParent(p.dir) != nil {
		return "state.write"
	}
	if !d.Unchanged() {
		return "state.path"
	}
	return ""
}
func (d *directoryStore) Sync(name string, expected []byte) port.Failure {
	data, failure := d.Read(name)
	if failure != "" {
		return failure
	}
	if data == nil || !bytes.Equal(data, expected) {
		return "state.invalid"
	}
	return d.pin.durableSnapshot(name, expected)
}
