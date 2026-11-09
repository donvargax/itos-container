// Package disk stores approval using pinned directories and atomic publication.
package disk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
)

// Store receives path selection and canonical encoding from cmd assembly.
// It never consults a checkout for an approval record.
type Store struct {
	StateHome, Home string
	Records         port.Records
	Control         port.Control
	ops             *operations
	ctx             context.Context
}
type operations struct {
	// Keep filesystem failures injectable while retaining real pinned handles.
	openRoot      func(string) (*os.Root, error)
	lstat         func(*os.Root, string) (os.FileInfo, error)
	mkdir         func(*os.Root, string, os.FileMode) error
	openDir       func(*os.Root, string) (*os.File, error)
	openChild     func(*os.Root, string) (*os.Root, error)
	statRoot      func(*os.Root, string) (os.FileInfo, error)
	syncDir       func(*os.File) error
	lock          func(*os.File, bool) error
	openPrivate   func(uintptr, string) (int, error)
	chmodPrivate  func(uintptr) error
	openRecord    func(*os.Root, string, int, os.FileMode) (*os.File, error)
	statFile      func(*os.File) (os.FileInfo, error)
	readAll       func(io.Reader) ([]byte, error)
	nonce         func([]byte) (int, error)
	chmod         func(*os.File, os.FileMode) error
	closeFile     func(*os.File) error
	create        func(*os.Root, string) (*os.File, error)
	write         func(*os.File, []byte) (int, error)
	syncFile      func(*os.File) error
	rename        func(*os.Root, string, string) error
	syncParent    func(*os.File) error
	beforePublish func()
}

func (s Store) operations() operations {
	ops := operations{
		openRoot: os.OpenRoot, lstat: (*os.Root).Lstat, mkdir: (*os.Root).Mkdir,
		openDir: (*os.Root).Open, openChild: (*os.Root).OpenRoot, statRoot: (*os.Root).Stat,
		syncDir: (*os.File).Sync, lock: func(f *os.File, exclusive bool) error {
			ctx := s.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			return sessionLock(ctx, f, !exclusive)
		}, openPrivate: openPrivate, chmodPrivate: chmodPrivate,
		openRecord: (*os.Root).OpenFile, statFile: (*os.File).Stat, readAll: io.ReadAll,
		nonce: rand.Read, chmod: (*os.File).Chmod, closeFile: (*os.File).Close,
		create: func(r *os.Root, n string) (*os.File, error) {
			return r.OpenFile(n, os.O_WRONLY|os.O_CREATE|os.O_EXCL|noFollow, 0600)
		}, write: (*os.File).Write, syncFile: (*os.File).Sync, rename: (*os.Root).Rename, syncParent: (*os.File).Sync}
	if s.ops != nil {
		return *s.ops
	}
	return ops
}

type pin struct {
	ops     operations
	roots   []*os.Root
	infos   []os.FileInfo
	path    string
	uid     uint64
	managed int
	dir     *os.File
}

func (p *pin) close() {
	if p.dir != nil {
		_ = unlock(p.dir)
		_ = p.dir.Close()
	}
	for i := len(p.roots) - 1; i >= 0; i-- {
		_ = p.roots[i].Close()
	}
}
func (p *pin) root() *os.Root { return p.roots[len(p.roots)-1] }

func (s Store) locationAt(c port.Candidate, collection string) (string, port.Failure) {
	state := s.StateHome
	if state == "" {
		if !filepath.IsAbs(s.Home) {
			return "", "state.location"
		}
		state = filepath.Join(s.Home, ".local", "state")
	}
	if !filepath.IsAbs(state) {
		return "", "state.location"
	}
	managed := filepath.Join(state, "itos-container")
	if within(managed, c.Root) || within(c.Root, managed) {
		return "", "state.path"
	}
	if len(c.Project) != 64 || strings.Trim(c.Project, "0123456789abcdef") != "" {
		return "", "state.invalid"
	}
	return filepath.Join(managed, collection, c.Project), ""
}

// within reports whether path is root or lies below it, by whole components.
func within(path, root string) bool {
	separator := string(filepath.Separator)
	return path == root || strings.HasPrefix(path, strings.TrimRight(root, separator)+separator)
}

func (s Store) pin(c port.Candidate, create bool) (*pin, port.Failure) {
	return s.pinAt(c, create, "approvals")
}
func (s Store) pinAt(c port.Candidate, create bool, collection string) (*pin, port.Failure) {
	path, failure := s.locationAt(c, collection)
	if failure != "" {
		return nil, failure
	}
	return s.pinPath(c, create, path, 3)
}

func (s Store) pinPath(c port.Candidate, create bool, path string, managedDepth int) (*pin, port.Failure) {
	uid, err := strconv.ParseUint(c.UID, 10, 32)
	if err != nil {
		return nil, "state.unsafe"
	}
	ops := s.operations()
	p := &pin{path: path, uid: uid, ops: ops}
	root, err := ops.openRoot(string(filepath.Separator))
	if err != nil {
		return nil, "state.unavailable"
	}
	p.roots = append(p.roots, root)
	parts := strings.Split(strings.TrimPrefix(path, string(filepath.Separator)), string(filepath.Separator))
	p.managed = len(parts) - managedDepth
	for index, name := range parts {
		current := p.root()
		info, err := ops.lstat(current, name)
		created := false
		if errors.Is(err, os.ErrNotExist) && create {
			err = ops.mkdir(current, name, 0700)
			created = err == nil
			if err != nil && !errors.Is(err, os.ErrExist) {
				p.close()
				return nil, "state.write"
			}
			parent, e := ops.openDir(current, ".")
			if e != nil {
				p.close()
				return nil, "state.write"
			}
			e = ops.syncDir(parent)
			_ = parent.Close()
			if e != nil {
				p.close()
				return nil, "state.write"
			}
			info, err = ops.lstat(current, name)
			if errors.Is(err, os.ErrNotExist) {
				// Made here, or by a peer, and removed again before this lookup. A
				// creating pin never reports an absent directory: its callers
				// dereference what it returns when no failure comes with it.
				p.close()
				return nil, "state.path"
			}
		}
		if errors.Is(err, os.ErrNotExist) {
			p.close()
			return nil, ""
		}
		if err != nil {
			p.close()
			return nil, "state.unavailable"
		}
		if created {
			var failure port.Failure
			info, failure = ops.privateDirectory(current, name, info)
			if failure != "" {
				p.close()
				return nil, failure
			}
		}
		if !created {
			if failure := directory(info, uid, index >= p.managed); failure != "" {
				p.close()
				return nil, failure
			}
		}
		next, err := ops.openChild(current, name)
		if err != nil {
			p.close()
			return nil, "state.path"
		}
		actual, err := ops.statRoot(next, ".")
		if err != nil || !os.SameFile(info, actual) {
			_ = next.Close()
			p.close()
			return nil, "state.path"
		}
		if created {
			directoryFile, e := ops.openDir(next, ".")
			if e != nil {
				_ = next.Close()
				p.close()
				return nil, "state.write"
			}
			e = ops.syncDir(directoryFile)
			_ = directoryFile.Close()
			if e != nil {
				_ = next.Close()
				p.close()
				return nil, "state.write"
			}
			actual, err = ops.statRoot(next, ".")
			if err != nil {
				_ = next.Close()
				p.close()
				return nil, "state.write"
			}
		}
		p.infos = append(p.infos, actual)
		p.roots = append(p.roots, next)
	}
	p.dir, err = ops.openDir(p.root(), ".")
	if err != nil {
		p.close()
		return nil, "state.unavailable"
	}
	if err = ops.lock(p.dir, create); err != nil {
		p.close()
		return nil, lockFailure(err)
	}
	if !p.unchanged() {
		p.close()
		return nil, "state.path"
	}
	return p, ""
}
func directory(info os.FileInfo, uid uint64, managed bool) port.Failure {
	if info.Mode()&os.ModeSymlink != 0 {
		return "state.path"
	}
	if !info.IsDir() {
		return "state.unsafe"
	}
	owner, _, ok := ownership(info)
	if !ok {
		return "state.unsafe"
	}
	if managed {
		if owner != uid || info.Mode().Perm() != 0700 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
			return "state.unsafe"
		}
		return ""
	}
	if owner != 0 && owner != uid {
		return "state.unsafe"
	}
	if info.Mode().Perm()&0022 != 0 && (owner != 0 || info.Mode()&os.ModeSticky == 0) {
		return "state.unsafe"
	}
	return ""
}
func ownership(info os.FileInfo) (uint64, uint64, bool) {
	stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
	if !stat.IsValid() || stat.Kind() != reflect.Struct {
		return 0, 0, false
	}
	uid, nlink := stat.FieldByName("Uid"), stat.FieldByName("Nlink")
	if !uid.IsValid() || !nlink.IsValid() {
		return 0, 0, false
	}
	return uid.Uint(), nlink.Uint(), true
}
func (p *pin) unchanged() bool {
	root, err := p.ops.openRoot(string(filepath.Separator))
	if err != nil {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(p.path, string(filepath.Separator)), string(filepath.Separator))
	for index, name := range parts {
		info, err := p.ops.lstat(root, name)
		if err != nil || directory(info, p.uid, index >= p.managed) != "" || !os.SameFile(info, p.infos[index]) {
			_ = root.Close()
			return false
		}
		next, err := p.ops.openChild(root, name)
		_ = root.Close()
		if err != nil {
			return false
		}
		actual, err := p.ops.statRoot(next, ".")
		if err != nil || !os.SameFile(info, actual) {
			_ = next.Close()
			return false
		}
		root = next
	}
	_ = root.Close()
	return true
}
func (p *pin) read(c port.Candidate, records port.Records) (port.Record, port.Failure) {
	return p.readNamed(c, records, "current.json")
}
func (p *pin) readNamed(c port.Candidate, records port.Records, name string) (port.Record, port.Failure) {
	info, err := p.ops.lstat(p.root(), name)
	if errors.Is(err, os.ErrNotExist) {
		return port.Record{}, ""
	}
	if err != nil {
		return port.Record{}, "state.unavailable"
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return port.Record{}, "state.path"
	}
	uid, links, ok := ownership(info)
	if !ok || !info.Mode().IsRegular() || uid != p.uid || links != 1 || info.Mode().Perm() != 0600 || info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		return port.Record{}, "state.unsafe"
	}
	file, err := p.ops.openRecord(p.root(), name, os.O_RDONLY|noFollow, 0)
	if err != nil {
		return port.Record{}, "state.path"
	}
	defer func() { _ = file.Close() }()
	actual, err := p.ops.statFile(file)
	if err != nil || !os.SameFile(actual, info) || recordSafety(actual, p.uid) != "" {
		return port.Record{}, "state.path"
	}
	data, err := p.ops.readAll(io.LimitReader(file, 1024*1024+1))
	if err != nil {
		return port.Record{}, "state.unavailable"
	}
	if !p.unchanged() {
		return port.Record{}, "state.path"
	}
	actual, err = p.ops.statFile(file)
	if err != nil {
		return port.Record{}, "state.unavailable"
	}
	if failure := recordSafety(actual, p.uid); failure != "" {
		return port.Record{}, failure
	}
	return records.Decode(c, data)
}

// Read never creates a directory, lock or record. Existing private coordination
// and selected-directory locks serialize readers with publication.
func (s Store) Read(c port.Candidate) (port.Record, port.Failure) {
	ctx, cancel := context.WithTimeout(context.Background(), budget.CallDuration)
	defer cancel()
	s.ctx = ctx
	transaction, failure := s.control(ctx, c, false)
	if failure != "" {
		return port.Record{}, failure
	}
	defer transaction.Close()
	p, failure := s.pin(c, false)
	if failure != "" || p == nil {
		return port.Record{}, failure
	}
	defer p.close()
	record, failure := p.read(c, s.Records)
	if failure == "" && !transaction.Unchanged() {
		return port.Record{}, "state.path"
	}
	return record, failure
}

// Commit compares generation and publishes only a complete durable record.
func (s Store) Commit(c port.Candidate, generation string) (port.Record, port.Failure) {
	ctx, cancel := context.WithTimeout(context.Background(), budget.CallDuration)
	defer cancel()
	s.ctx = ctx
	transaction, failure := s.control(ctx, c, true)
	if failure != "" {
		return port.Record{}, failure
	}
	defer transaction.Close()
	p, failure := s.pin(c, true)
	if failure != "" {
		return port.Record{}, failure
	}
	defer p.close()
	old, failure := p.read(c, s.Records)
	if failure != "" {
		return port.Record{}, failure
	}
	if old.Generation != generation {
		return port.Record{}, "approve.conflict"
	}
	if generation != "" && old.Canonical == c.Canonical {
		if !transaction.Unchanged() {
			return port.Record{}, "state.path"
		}
		return old, ""
	}
	n := uint64(0)
	if generation != "" {
		var err error
		n, err = strconv.ParseUint(generation, 10, 64)
		if err != nil || n == ^uint64(0) {
			return port.Record{}, "state.invalid"
		}
	}
	record := port.Record{Candidate: c, Generation: strconv.FormatUint(n+1, 10)}
	data := s.Records.Encode(record)
	if len(data) == 0 || len(data) > 1024*1024 {
		return port.Record{}, "state.invalid"
	}
	ops := s.operations()
	if ops.beforePublish != nil {
		ops.beforePublish()
	}
	if !p.unchanged() || !transaction.Unchanged() {
		return port.Record{}, "state.path"
	}
	var nonce [16]byte
	if _, err := ops.nonce(nonce[:]); err != nil {
		return port.Record{}, "state.write"
	}
	name := ".approval-" + hex.EncodeToString(nonce[:]) + ".tmp"
	file, err := ops.create(p.root(), name)
	if err != nil {
		return port.Record{}, "state.write"
	}
	defer func() { _ = file.Close(); _ = p.root().Remove(name) }()
	if ops.chmod(file, 0600) != nil {
		return port.Record{}, "state.write"
	}
	written, err := ops.write(file, data)
	if err != nil || written != len(data) {
		return port.Record{}, "state.write"
	}
	if ops.syncFile(file) != nil {
		return port.Record{}, "state.write"
	}
	if ops.closeFile(file) != nil {
		return port.Record{}, "state.write"
	}
	if !p.unchanged() || !transaction.Unchanged() {
		return port.Record{}, "state.path"
	}
	if ops.rename(p.root(), name, "current.json") != nil {
		return port.Record{}, "state.write"
	}
	if ops.syncParent(p.dir) != nil {
		return port.Record{}, "state.write"
	}
	if !p.unchanged() || !transaction.Unchanged() {
		return port.Record{}, "state.path"
	}
	return record, ""
}

// Select atomically verifies the active generation, exact bytes and fingerprint.
// Its returned string owns the bytes; it never consults checkout request files.
func (s Store) Select(c port.Candidate, generation string) (port.Frozen, port.Failure) {
	ctx, cancel := context.WithTimeout(context.Background(), budget.CallDuration)
	defer cancel()
	s.ctx = ctx
	// Establish existing approval before any enrollment, then recheck it while
	// holding common exclusion in the required coordination-before-selected lock order.
	if _, failure := s.selectRecord(c, generation); failure != "" {
		return port.Frozen{}, failure
	}
	transaction, failure := s.control(ctx, c, true)
	if failure != "" {
		return port.Frozen{}, failure
	}
	defer transaction.Close()
	frozen, failure := s.selectRecord(c, generation)
	if failure == "" && !transaction.Unchanged() {
		return port.Frozen{}, "state.path"
	}
	return frozen, failure
}
func (s Store) selectRecord(c port.Candidate, generation string) (port.Frozen, port.Failure) {
	p, failure := s.pin(c, false)
	if failure != "" {
		return port.Frozen{}, failure
	}
	if p == nil {
		return port.Frozen{}, "missing"
	}
	defer p.close()
	record, failure := p.read(c, s.Records)
	if failure != "" {
		return port.Frozen{}, failure
	}
	if record.Generation == "" {
		return port.Frozen{}, "missing"
	}
	if record.Generation != generation {
		return port.Frozen{}, "superseded"
	}
	if record.Canonical != c.Canonical || record.Fingerprint != c.Fingerprint {
		return port.Frozen{}, "changed"
	}
	return port.Frozen{Project: record.Project, Generation: record.Generation, Fingerprint: record.Fingerprint, Settings: record.Canonical}, ""
}

func recordSafety(info os.FileInfo, expected uint64) port.Failure {
	return linkedRecordSafety(info, expected, 1)
}

// recordLinks is the number of links a record in this directory may have. Publish
// links a new record to its final name and then removes the temporary, so a
// publisher killed between the two leaves the record with a second link: the
// temporary it named .intent-<nonce>.tmp, beside the record in this pinned
// private directory. The record's bytes were synced before the link, and no name
// outside the directory reaches the inode, so such a record is read as it would
// be after the removal that did not run. Names has always skipped the temporary.
// Any other second link, and any further one, leaves the record unsafe.
func (p *pin) recordLinks(info os.FileInfo) uint64 {
	if _, links, ok := ownership(info); !ok || links != 2 {
		return 1
	}
	dir, err := p.ops.openDir(p.root(), ".")
	if err != nil {
		return 1
	}
	// Entries read before an error still name only files that are checked below.
	entries, _ := dir.ReadDir(-1)
	_ = dir.Close()
	for _, entry := range entries {
		if !intentTemporary(entry.Name()) {
			continue
		}
		twin, err := p.ops.lstat(p.root(), entry.Name())
		if err == nil && os.SameFile(info, twin) {
			return 2
		}
	}
	return 1
}

// intentTemporary reports a name that Publish makes: ".intent-", a 128-bit hex
// nonce and ".tmp".
func intentTemporary(name string) bool {
	return len(name) == 44 && strings.HasPrefix(name, ".intent-") && strings.HasSuffix(name, ".tmp") && strings.Trim(name[8:40], "0123456789abcdef") == ""
}

// linkedRecordSafety is recordSafety for a record that is expected to have the
// given number of links.
func linkedRecordSafety(info os.FileInfo, expected, links uint64) port.Failure {
	uid, actual, ok := ownership(info)
	if !ok || !info.Mode().IsRegular() || uid != expected || actual != links || info.Mode().Perm() != 0600 || info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		return "state.unsafe"
	}
	return ""
}

// privateDirectory uses an O_PATH handle to initialize a new directory even if
// umask removed its search permission. The procfs link targets that exact inode;
// neither the parent pathname nor a replacement symlink can redirect the change.
func (ops operations) privateDirectory(parent *os.Root, name string, expected os.FileInfo) (os.FileInfo, port.Failure) {
	if !expected.IsDir() || expected.Mode()&os.ModeSymlink != 0 {
		return nil, "state.path"
	}
	directory, err := ops.openDir(parent, ".")
	if err != nil {
		return nil, "state.write"
	}
	defer func() { _ = directory.Close() }()
	fd, err := ops.openPrivate(directory.Fd(), name)
	if err != nil {
		return nil, "state.path"
	}
	file := os.NewFile(uintptr(fd), name)
	defer func() { _ = file.Close() }()
	actual, err := ops.statFile(file)
	if err != nil || !os.SameFile(expected, actual) {
		return nil, "state.path"
	}
	if ops.chmodPrivate(file.Fd()) != nil {
		return nil, "state.write"
	}
	latest, err := ops.lstat(parent, name)
	if err != nil || !os.SameFile(expected, latest) {
		return nil, "state.path"
	}
	return latest, ""
}

// Uncoordinated stores are primitives used beneath the common transaction and
// by isolated storage contracts. Production assembly always supplies Control.
type localTransaction struct{}

func (localTransaction) Close()          {}
func (localTransaction) Unchanged() bool { return true }
func (s Store) control(ctx context.Context, c port.Candidate, write bool) (port.ControlTransaction, port.Failure) {
	if s.Control == nil {
		return localTransaction{}, ""
	}
	return s.Control.Begin(ctx, c, write)
}
