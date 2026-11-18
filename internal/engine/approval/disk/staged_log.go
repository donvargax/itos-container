package disk

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
)

// stagedLogName is the one file a session directory stages. No caller chooses
// it, so none can aim a staged write at a record, a lock or a path.
const stagedLogName = "egress-refusals.jsonl"

// stagedReportName is the committed copy's own report, published beside the log
// only after the log's own rename and parent sync.
const stagedReportName = "egress-refusals.json"

// evidenceLimit bounds one evidence read. The refusal cap's full log is about
// 40 MiB; anything past this bound is refused, never silently accepted.
const evidenceLimit = 64 << 20

// Evidence reads a session's published log and report through the same pinned
// directory Stage publishes into. It takes no lock and creates no directory; an
// absent file is nil with no failure.
func (s Registry) Evidence(ctx context.Context, c port.Candidate, id string) (port.SessionEvidence, port.Failure) {
	if !publicID(id) {
		return port.SessionEvidence{}, "state.invalid"
	}
	if ctx.Err() != nil {
		return port.SessionEvidence{}, "state.unavailable"
	}
	store := s.store(ctx)
	path, failure := store.locationAt(c, "sessions")
	if failure != "" {
		return port.SessionEvidence{}, failure
	}
	p, failure := store.pinPath(c, false, filepath.Join(path, id), 4)
	if failure != "" || p == nil {
		return port.SessionEvidence{}, failure
	}
	defer p.close()
	evidence := port.SessionEvidence{Path: filepath.Join(path, id, stagedLogName)}
	for name, target := range map[string]*[]byte{stagedLogName: &evidence.Log, stagedReportName: &evidence.Report} {
		data, f := p.readEvidence(name, evidenceLimit)
		if f != "" {
			return port.SessionEvidence{}, f
		}
		*target = data
	}
	if !p.unchanged() {
		return port.SessionEvidence{}, "state.path"
	}
	return evidence, ""
}

// readEvidence reads one private regular file of the pinned directory, or nil
// when it is absent. A second link or a foreign file is unsafe, never evidence.
func (p *pin) readEvidence(name string, limit int64) ([]byte, port.Failure) {
	info, err := p.ops.lstat(p.root(), name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ""
	}
	if err != nil {
		return nil, "state.unavailable"
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, "state.path"
	}
	uid, links, ok := ownership(info)
	if !ok || !info.Mode().IsRegular() || uid != p.uid || links != 1 || info.Mode().Perm() != 0600 || info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		return nil, "state.unsafe"
	}
	if info.Size() > limit {
		return nil, "state.unsafe"
	}
	file, err := p.ops.openRecord(p.root(), name, os.O_RDONLY|noFollow, 0)
	if err != nil {
		return nil, "state.path"
	}
	defer func() { _ = file.Close() }()
	actual, err := p.ops.statFile(file)
	if err != nil || !os.SameFile(actual, info) || recordSafety(actual, p.uid) != "" {
		return nil, "state.path"
	}
	data, err := p.ops.readAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, "state.unavailable"
	}
	if int64(len(data)) > limit || !p.unchanged() {
		return nil, "state.path"
	}
	return data, ""
}

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
	l := &stagedLog{pin: p, ctx: budget.Lifetime(ctx), temporary: ".refusals-" + hex.EncodeToString(nonce[:]) + ".tmp", published: filepath.Join(path, id, stagedLogName)}
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
	pin                 *pin
	ctx                 context.Context
	file                *os.File
	report              *os.File
	temporary, reportID string
	published           string
	poisoned, released  bool
}

func (l *stagedLog) unchanged() bool { return l.ctx.Err() == nil && l.pin.unchanged() }

// release closes the file, removes the temporary's name when a rename has not
// already taken it, and gives up the pin.
func (l *stagedLog) release() {
	l.released = true
	_ = l.file.Close()
	_ = l.pin.root().Remove(l.temporary)
	if l.report != nil {
		_ = l.report.Close()
		_ = l.pin.root().Remove(l.reportID)
	}
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

// Attach stages the report published beside the log. The log is renamed and its
// parent synced before the report becomes visible, so a copy interrupted at the
// last durable step exposes its log without claiming a committed report.
func (l *stagedLog) Attach(data []byte) port.Failure {
	if l.released || l.poisoned {
		return "state.write"
	}
	if l.report != nil {
		return "state.conflict"
	}
	var nonce [16]byte
	if _, err := l.pin.ops.nonce(nonce[:]); err != nil {
		return "state.write"
	}
	l.reportID = ".refusals-" + hex.EncodeToString(nonce[:]) + ".report.tmp"
	file, err := l.pin.ops.create(l.pin.root(), l.reportID)
	if err != nil {
		return "state.write"
	}
	if l.pin.ops.chmod(file, 0600) != nil {
		_ = file.Close()
		_ = l.pin.root().Remove(l.reportID)
		l.reportID = ""
		return "state.write"
	}
	n, err := l.pin.ops.write(file, data)
	if err != nil || n != len(data) || l.pin.ops.syncFile(file) != nil {
		_ = file.Close()
		_ = l.pin.root().Remove(l.reportID)
		l.reportID = ""
		return "state.write"
	}
	l.report = file
	return ""
}

// Path names the destination Publish renames the staged log to.
func (l *stagedLog) Path() string { return l.published }

// Publish runs once, in the order a record's publication keeps after its write.
// A failure before the rename leaves the earlier file as it was; one after it
// leaves the new bytes visible without a durability claim. Whatever it returns,
// the temporary's name is gone and the pin is released. A staged report is
// renamed only after the log's own rename and parent sync: a new log never
// stands beside an old committed report.
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
	if l.report != nil {
		if p.ops.closeFile(l.report) != nil || p.ops.rename(p.root(), l.reportID, stagedReportName) != nil || p.ops.syncParent(p.dir) != nil {
			return "state.write"
		}
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
