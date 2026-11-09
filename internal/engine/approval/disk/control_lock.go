package disk

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

func (t *controlTransaction) openCatalog(create bool) port.Failure {
	if t.catalog != nil {
		t.catalog.close()
		t.catalog = nil
	}
	p, failure := t.store.pinPath(t.candidate, create, filepath.Join(t.fixed, "coordination"), 2)
	if failure != "" || p == nil {
		return failure
	}
	t.catalog = p
	// Open the named directory itself with O_DIRECTORY|O_NOFOLLOW, then
	// compare its descriptor to the protected chain; never trust just lstat.
	parent, err := p.ops.openDir(p.roots[len(p.roots)-2], ".")
	if err != nil {
		return "state.unavailable"
	}
	defer func() { _ = parent.Close() }()
	fd, err := p.ops.openPrivate(parent.Fd(), "coordination")
	if err != nil {
		return "state.path"
	}
	file := os.NewFile(uintptr(fd), "coordination")
	defer func() { _ = file.Close() }()
	info, err := p.ops.statFile(file)
	if err != nil || !os.SameFile(info, p.infos[len(p.infos)-1]) {
		return "state.path"
	}
	return directory(info, p.uid, true)
}

func (t *controlTransaction) lockUnchanged() bool {
	if t.lock == nil {
		_, err := t.home.ops.lstat(t.home.root(), ".local/state/itos-container/coordination/"+controlLockName)
		return errors.Is(err, os.ErrNotExist)
	}
	p := t.catalog
	info, err := p.ops.statFile(t.lock)
	if err != nil || recordSafety(info, p.uid) != "" || !os.SameFile(info, t.lockInfo) {
		return false
	}
	latest, err := p.ops.lstat(p.root(), controlLockName)
	return err == nil && recordSafety(latest, p.uid) == "" && os.SameFile(info, latest)
}

func (t *controlTransaction) acquire(ctx context.Context, create, write bool) port.Failure {
	p := t.catalog
	if p == nil {
		return ""
	}
	info, err := p.ops.lstat(p.root(), controlLockName)
	if errors.Is(err, os.ErrNotExist) && create {
		file, e := p.ops.create(p.root(), controlLockName)
		if e == nil {
			// Only the exclusively created inode may be initialized. Existing
			// unsafe files are refused, never chmodded or replaced on retry.
			e = p.ops.chmod(file, 0600)
			_ = file.Close()
		}
		if e != nil && !errors.Is(e, os.ErrExist) {
			return "state.write"
		}
		info, err = p.ops.lstat(p.root(), controlLockName)
	}
	if errors.Is(err, os.ErrNotExist) && !create {
		return ""
	}
	if err != nil {
		return "state.unavailable"
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "state.path"
	}
	if failure := recordSafety(info, p.uid); failure != "" {
		return failure
	}
	// flock(2) on NFS places an exclusive lock only through a descriptor open for
	// writing, and a shared one through a readable descriptor. The 0600 file was
	// just validated as ours alone, so only a writer opens it for writing.
	access := os.O_RDONLY
	if write {
		access = os.O_RDWR
	}
	file, err := p.ops.openRecord(p.root(), controlLockName, access|noFollow, 0)
	if err != nil {
		return "state.unavailable"
	}
	t.lock, t.lockInfo = file, info
	if !t.Unchanged() {
		return "state.path"
	}
	if err = sessionLock(ctx, file, !write); err != nil {
		return lockFailure(err)
	}
	if !t.Unchanged() {
		return "state.path"
	}
	return ""
}

func lockFailure(err error) port.Failure {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "state.busy"
	}
	return "state.unavailable"
}
