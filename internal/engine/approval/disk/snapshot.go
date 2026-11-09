package disk

import (
	"bytes"
	"encoding/hex"
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"io"
	"os"
	"path/filepath"
)

// SnapshotIO supplies file operations at the publication boundary. Nil operations
// select the real filesystem calls; callers cannot bypass record/path validation.
type SnapshotIO struct {
	Create        func(*os.Root, string) (*os.File, error)
	Write         func(*os.File, []byte) (int, error)
	FileSync      func(*os.File) error
	Rename        func(*os.Root, string, string) error
	ParentSync    func(*os.File) error
	DirectorySync func(*os.File) error
}

// Snapshots shares approval's protected state root and canonical record codec.
type Snapshots struct {
	Store   Store
	IO      SnapshotIO
	Records port.FrozenRecords
}

// Publish validates only Frozen, serializes publishers under the project lock and
// returns a handoff only after file and directory durability has been established.
func (s Snapshots) Publish(f port.Frozen) (port.Durable, port.Failure) {
	record, failure := s.Records.FrozenRecord(f)
	if failure != "" {
		return port.Durable{}, "snapshot.invalid"
	}
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
	store.ops = &ops
	path, failure := store.locationAt(record.Candidate, "snapshots")
	reference := filepath.Join(path, f.Generation+".json")
	refused := func(failure port.Failure) (port.Durable, port.Failure) {
		return port.Durable{Reference: reference}, snapshotFailure(failure)
	}
	if failure != "" {
		return refused(failure)
	}
	p, failure := store.pinAt(record.Candidate, true, "snapshots")
	if failure != "" {
		return refused(failure)
	}
	defer p.close()
	name := f.Generation + ".json"
	old, failure := p.readNamed(record.Candidate, s.Records, name)
	if failure != "" {
		return refused(failure)
	}
	if old.Generation != "" && old != record {
		return refused("state.invalid")
	}
	data := s.Records.Encode(record)
	if old.Generation == "" {
		if failure = p.publishSnapshot(name, data); failure != "" {
			return refused(failure)
		}
	}
	if failure = p.durableSnapshot(name, data); failure != "" {
		return refused(failure)
	}
	return port.Durable{Frozen: f, Reference: reference}, ""
}
func snapshotFailure(f port.Failure) port.Failure {
	switch f {
	case "state.invalid":
		return "snapshot.invalid"
	case "state.write", "state.unavailable":
		return "snapshot.write"
	default:
		return "snapshot.unsafe"
	}
}
func (p *pin) publishSnapshot(name string, data []byte) port.Failure {
	var nonce [16]byte
	if _, err := p.ops.nonce(nonce[:]); err != nil {
		return "state.write"
	}
	temporary := ".snapshot-" + hex.EncodeToString(nonce[:]) + ".tmp"
	file, err := p.ops.create(p.root(), temporary)
	if err != nil {
		return "state.write"
	}
	defer func() { _ = file.Close(); _ = p.root().Remove(temporary) }()
	if p.ops.chmod(file, 0600) != nil {
		return "state.write"
	}
	n, err := p.ops.write(file, data)
	if err != nil || n != len(data) {
		return "state.write"
	}
	if p.ops.syncFile(file) != nil {
		return "state.write"
	}
	if p.ops.closeFile(file) != nil {
		return "state.write"
	}
	if !p.unchanged() {
		return "state.path"
	}
	// Cooperative publishers share the pinned directory lock. No existing valid
	// snapshot is replaced: it was compared under this same lock above.
	if p.ops.rename(p.root(), temporary, name) != nil {
		return "state.write"
	}
	return ""
}
func (p *pin) durableSnapshot(name string, expected []byte) port.Failure {
	file, err := p.ops.openRecord(p.root(), name, os.O_RDONLY|noFollow, 0)
	if err != nil {
		return "state.path"
	}
	defer func() { _ = file.Close() }()
	info, err := p.ops.statFile(file)
	if err != nil {
		return "state.write"
	}
	links := p.recordLinks(info)
	if failure := linkedRecordSafety(info, p.uid, links); failure != "" {
		return failure
	}
	data, err := p.ops.readAll(io.LimitReader(file, 1024*1024+1))
	if err != nil {
		return "state.write"
	}
	if !bytes.Equal(data, expected) {
		return "state.invalid"
	}
	if p.ops.syncFile(file) != nil {
		return "state.write"
	}
	if p.ops.syncParent(p.dir) != nil {
		return "state.write"
	}
	// Re-sync the entire ancestor chain on reuse too: a previous attempt may
	// have created an entry whose parent fsync failed before returning a handoff.
	for _, root := range p.roots {
		directory, err := p.ops.openDir(root, ".")
		if err != nil {
			return "state.write"
		}
		err = p.ops.syncDir(directory)
		_ = directory.Close()
		if err != nil {
			return "state.write"
		}
	}
	actual, err := p.ops.lstat(p.root(), name)
	if err != nil || !os.SameFile(info, actual) || linkedRecordSafety(actual, p.uid, links) != "" || !p.unchanged() {
		return "state.path"
	}
	return ""
}
