package disk

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// Publish writes .intent-<nonce>.tmp beside a record, then links or renames it
// onto the record and removes the name on return. A publisher killed in between
// leaves the temporary alone, or as a second hard link of its record, and nothing
// else ever removes it. The holder of the control lock sweeps what is provably
// such a leftover. The sweep is a private step of Begin: it never fails, never
// waits, and spends the call budget Begin already has, so a transaction's result
// is what it would be without it.

// sweepKind names the three directory kinds Publish writes into, each with the
// records its temporaries can be a second link of.
type sweepKind int

const (
	sweepAdmission sweepKind = iota // L/admission
	sweepIntents                    // L/intents/<project>
	sweepSession                    // L/sessions/<index>/<session>
)

func (k sweepKind) record(name string) bool {
	switch k {
	case sweepAdmission:
		return recordName(name) && len(name) == 69
	case sweepIntents:
		return (recordName(name) && len(name) == 69) || port.ExecutionName(name)
	default:
		return name == "record.json"
	}
}

// sweepTemporaries visits every managed root this transaction inspected: the
// fixed tree, the selected tree and each enrolled location.
func (t *controlTransaction) sweepTemporaries(ctx context.Context) {
	roots := []string{t.fixed}
	if t.selected != t.fixed {
		roots = append(roots, t.selected)
	}
	var others []string
	for path := range t.bindings {
		if path != t.fixed && path != t.selected {
			others = append(others, path)
		}
	}
	slices.Sort(others)
	for _, root := range append(roots, others...) {
		if ctx.Err() != nil {
			return
		}
		t.sweepRoot(ctx, root)
	}
}

// sweepPin pins one directory by the rules every reader uses, without the pin's
// waiting lock. An absent directory, or one that will not pin, is skipped.
func (t *controlTransaction) sweepPin(path string, depth int) *pin {
	p, failure := t.store.pinPath(t.candidate, false, path, depth)
	if failure != "" || p == nil {
		return nil
	}
	return p
}

// sweepChildren lists the entries of a pinned collection that are 64 lowercase hex.
func (t *controlTransaction) sweepChildren(ctx context.Context, path string, depth int) []string {
	if ctx.Err() != nil {
		return nil
	}
	p := t.sweepPin(path, depth)
	if p == nil {
		return nil
	}
	defer p.close()
	names, ok := p.sweepNames(ctx)
	if !ok {
		return nil
	}
	var children []string
	for _, name := range names {
		if publicID(name) {
			children = append(children, name)
		}
	}
	slices.Sort(children)
	return children
}

func (t *controlTransaction) sweepRoot(ctx context.Context, root string) {
	t.sweepDirectory(ctx, filepath.Join(root, "admission"), 2, sweepAdmission)
	for _, project := range t.sweepChildren(ctx, filepath.Join(root, "intents"), 2) {
		t.sweepDirectory(ctx, filepath.Join(root, "intents", project), 3, sweepIntents)
	}
	for _, index := range t.sweepChildren(ctx, filepath.Join(root, "sessions"), 2) {
		for _, id := range t.sweepChildren(ctx, filepath.Join(root, "sessions", index), 3) {
			t.sweepDirectory(ctx, filepath.Join(root, "sessions", index, id), 4, sweepSession)
		}
	}
}

func (t *controlTransaction) sweepDirectory(ctx context.Context, path string, depth int, kind sweepKind) {
	if ctx.Err() != nil {
		return
	}
	p := t.sweepPin(path, depth)
	if p == nil {
		return
	}
	defer p.close()
	names, ok := p.sweepNames(ctx)
	if !ok || !slices.ContainsFunc(names, intentTemporary) || ctx.Err() != nil {
		return
	}
	lock, ok := p.sweepExclusion(kind)
	if !ok {
		return
	}
	defer lock.close()
	slices.Sort(names)
	for _, name := range names {
		if ctx.Err() != nil {
			return
		}
		if intentTemporary(name) {
			t.sweepOne(ctx, p, kind, names, name, lock)
		}
	}
}

// sweepNames reads a directory in bounded batches so a large collection cannot
// keep traversal going after the call budget ends. The removal rechecks the
// pin and transaction before trusting any name returned here.
func (p *pin) sweepNames(ctx context.Context) ([]string, bool) {
	var names []string
	for ctx.Err() == nil {
		batch, err := p.dir.Readdirnames(128)
		names = append(names, batch...)
		if err != nil {
			return names, errors.Is(err, io.EOF) && ctx.Err() == nil
		}
	}
	return nil, false
}

const sweepOperationLock = "operation.lock"

type sweepLock struct {
	file      *os.File
	operation bool
}

func (l sweepLock) close() {
	_ = unlock(l.file)
	if l.operation {
		_ = l.file.Close()
	}
}

func (l sweepLock) unchanged(p *pin) bool {
	if !l.operation {
		// The enclosing pin verifies the locked directory itself.
		return true
	}
	info, err := p.ops.statFile(l.file)
	if err != nil || recordSafety(info, p.uid) != "" || info.Size() != 0 {
		return false
	}
	latest, err := p.ops.lstat(p.root(), sweepOperationLock)
	return err == nil && os.SameFile(info, latest) && recordSafety(latest, p.uid) == "" && latest.Size() == 0
}

// sweepExclusion never waits: a stop may hold operation.lock while it waits for
// the control lock, and a run holds its intent directory for its transaction.
func (p *pin) sweepExclusion(kind sweepKind) (sweepLock, bool) {
	if kind != sweepSession {
		return sweepLock{file: p.dir}, tryLock(p.dir) == nil
	}
	// Skip unsafe entries before opening them. openLock still opens with
	// O_NONBLOCK on Unix and checks the inode in case the entry was replaced.
	info, err := p.ops.lstat(p.root(), sweepOperationLock)
	if err != nil || recordSafety(info, p.uid) != "" || info.Size() != 0 {
		return sweepLock{}, false
	}
	file, failure := p.openLock(sweepOperationLock, false)
	if failure != "" {
		return sweepLock{}, false
	}
	if tryLock(file) != nil {
		_ = file.Close()
		return sweepLock{}, false
	}
	return sweepLock{file: file, operation: true}, true
}

// sweepable is the file Publish leaves: regular, ours, exactly 0600, no setuid,
// setgid or sticky bit, and the given number of links.
func sweepable(info os.FileInfo, uid, links uint64) bool {
	return linkedRecordSafety(info, uid, links) == "" && info.Mode()&os.ModeSticky == 0
}

// sweepOne removes one temporary when every piece of durable evidence holds. Its
// name is exactly the one Publish makes (the caller checked), and its lstat
// through the pinned root, never followed, shows a lone file, or one with exactly
// two links whose twin is a record of this directory. Only its own name goes: the
// twin keeps its bytes, inode and mtime, and has one link.
func (t *controlTransaction) sweepOne(ctx context.Context, p *pin, kind sweepKind, names []string, name string, lock sweepLock) {
	info, err := p.ops.lstat(p.root(), name)
	if err != nil {
		return
	}
	_, links, ok := ownership(info)
	if !ok {
		return
	}
	twinName, twin := "", os.FileInfo(nil)
	switch links {
	case 1:
	case 2:
		if twinName, twin = p.sweepTwin(ctx, kind, names, info); twin == nil {
			return
		}
	default:
		return
	}
	if !sweepable(info, p.uid, links) {
		return
	}
	// Just before removal, the file and its twin are still the same inodes, the
	// pin is unchanged and so is the transaction.
	latest, err := p.ops.lstat(p.root(), name)
	if err != nil || !os.SameFile(info, latest) || !sweepable(latest, p.uid, links) {
		return
	}
	if twin != nil {
		again, err := p.ops.lstat(p.root(), twinName)
		if err != nil || !os.SameFile(twin, again) || !sweepable(again, p.uid, 2) {
			return
		}
	}
	if ctx.Err() != nil || !p.unchanged() || !t.Unchanged() || !lock.unchanged(p) || ctx.Err() != nil {
		return
	}
	_ = p.root().Remove(name)
}

// sweepTwin finds the record in this directory that is the same inode as the
// temporary, which is itself a file Publish could have made.
func (p *pin) sweepTwin(ctx context.Context, kind sweepKind, names []string, info os.FileInfo) (string, os.FileInfo) {
	if !sweepable(info, p.uid, 2) {
		return "", nil
	}
	for _, name := range names {
		if ctx.Err() != nil {
			return "", nil
		}
		if !kind.record(name) {
			continue
		}
		twin, err := p.ops.lstat(p.root(), name)
		if err == nil && os.SameFile(info, twin) && sweepable(twin, p.uid, 2) {
			return name, twin
		}
	}
	return "", nil
}
