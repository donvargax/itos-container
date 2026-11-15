package disk

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
)

// stagedLogName is the one file a session directory stages. No caller chooses
// it, so none can aim a staged write at a record, a lock or a path.
const stagedLogName = "egress-refusals.jsonl"

// Stage pins the session directory as Open does and creates a private temporary
// in it. It creates no directory and neither takes nor checks the operation
// lock, which its caller holds: a second Open of a held session would wait out
// its budget. The temporary's prefix is not Publish's, so recordLinks never
// takes it for a record's twin.
func (s Registry) Stage(ctx context.Context, c port.Candidate, id string) (port.StagedLog, port.Failure) {
	if !publicID(id) {
		return nil, "state.invalid"
	}
	if ctx.Err() != nil {
		return nil, "state.unavailable"
	}
	store := s.store(ctx)
	path, failure := store.locationAt(c, "sessions")
	if failure != "" {
		return nil, failure
	}
	ops := *store.ops
	ops.lock = func(*os.File, bool) error { return nil }
	store.ops = &ops
	p, failure := store.pinPath(c, false, filepath.Join(path, id), 4)
	if failure != "" || p == nil {
		return nil, failure
	}
	// Re-establish directory durability after any earlier uncertain mkdir/fsync.
	for _, root := range p.roots {
		directory, err := p.ops.openDir(root, ".")
		if err == nil {
			err = p.ops.syncDir(directory)
			_ = directory.Close()
		}
		if err != nil {
			p.close()
			return nil, "state.write"
		}
	}
	var nonce [16]byte
	if _, err := p.ops.nonce(nonce[:]); err != nil {
		p.close()
		return nil, "state.write"
	}
	l := &stagedLog{pin: p, ctx: budget.Lifetime(ctx), temporary: ".refusals-" + hex.EncodeToString(nonce[:]) + ".tmp"}
	file, err := p.ops.create(p.root(), l.temporary)
	if err != nil {
		p.close()
		return nil, "state.write"
	}
	l.file = file
	if p.ops.chmod(file, 0600) != nil {
		l.release()
		return nil, "state.write"
	}
	return l, ""
}

// stagedLog serves one caller at a time; the session adapter serializes its
// calls. Its integrity rests on its own random name and one rename, not a lock.
type stagedLog struct {
	pin                *pin
	ctx                context.Context
	file               *os.File
	temporary          string
	poisoned, released bool
}

func (l *stagedLog) unchanged() bool { return l.ctx.Err() == nil && l.pin.unchanged() }

// release closes the file, removes the temporary's name when a rename has not
// already taken it, and gives up the pin.
func (l *stagedLog) release() {
	l.released = true
	_ = l.file.Close()
	_ = l.pin.root().Remove(l.temporary)
	l.pin.close()
}

// Write hands its bytes to the file before it returns. An error or a short
// write poisons the handle, which then refuses every later Write unwritten.
func (l *stagedLog) Write(data []byte) port.Failure {
	if l.released || l.poisoned {
		return "state.write"
	}
	n, err := l.pin.ops.write(l.file, data)
	if err != nil || n != len(data) {
		l.poisoned = true
		return "state.write"
	}
	return ""
}

// Publish runs once, in the order a record's publication keeps after its write.
// A failure before the rename leaves the earlier file as it was; one after it
// leaves the new bytes visible without a durability claim. Whatever it returns,
// the temporary's name is gone and the pin is released.
func (l *stagedLog) Publish() port.Failure {
	if l.released {
		return "state.conflict"
	}
	defer l.release()
	p := l.pin
	if l.poisoned || p.ops.syncFile(l.file) != nil || p.ops.closeFile(l.file) != nil {
		return "state.write"
	}
	if !l.unchanged() {
		return "state.path"
	}
	if p.ops.rename(p.root(), l.temporary, stagedLogName) != nil || p.ops.syncParent(p.dir) != nil {
		return "state.write"
	}
	if !l.unchanged() {
		return "state.path"
	}
	return ""
}

// Close without Publish removes the temporary; after Publish it changes nothing.
func (l *stagedLog) Close() {
	if !l.released {
		l.release()
	}
}
