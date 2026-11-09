package disk

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/donvargax/itos-container/internal/engine/approval/domain"
	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// Control owns the per-account transaction through a validated private lock file.
// The account home is pinned for identity checks, never used as a lock inode.
// Reservations is the session engine's read-only ownership validator.
type Control struct {
	Account      port.Account
	Store        Store
	Reservations port.Reservations
}
type locationIdentity struct {
	Path   string `json:"path"`
	Device string `json:"device"`
	Inode  string `json:"inode"`
}
type locationBinding struct {
	Schema    int                `json:"schema"`
	UID       string             `json:"uid"`
	Path      string             `json:"path"`
	Complete  bool               `json:"complete"`
	Ancestors []locationIdentity `json:"ancestors"`
}
type controlTransaction struct {
	home            *pin
	lock            *os.File
	lockInfo        os.FileInfo
	catalog         *pin
	pins            []*pin
	store           Store
	candidate       port.Candidate
	fixed, selected string
	bindings        map[string]locationBinding
	data            map[string][]byte
}

const controlLockName = "control.lock"

func (t *controlTransaction) Close() {
	for _, p := range t.pins {
		p.close()
	}
	if t.catalog != nil {
		t.catalog.close()
	}
	t.home.close()
	if t.lock != nil {
		_ = t.lock.Close()
	}
}
func (t *controlTransaction) Unchanged() bool {
	return t.pathsUnchanged() && t.lockUnchanged()
}

// A writer may adopt a peer-created lock after unlocked inspection, but every
// directory identity pinned by that inspection must still be unchanged.
func (t *controlTransaction) pathsUnchanged() bool {
	if !t.home.unchanged() || !accountDirectory(t.home) {
		return false
	}
	if t.catalog != nil && !t.catalog.unchanged() {
		return false
	}
	for _, p := range t.pins {
		if !p.unchanged() {
			return false
		}
	}
	return true
}
func accountDirectory(p *pin) bool {
	info, err := p.ops.statFile(p.dir)
	if err != nil {
		return false
	}
	owner, _, ok := ownership(info)
	return ok && owner == p.uid && info.Mode().Perm()&0022 == 0
}
func (s Control) Begin(ctx context.Context, c port.Candidate, write bool) (port.ControlTransaction, port.Failure) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if s.Account == nil || s.Reservations == nil {
		return nil, "state.unavailable"
	}
	home, failure := s.Account.AccountHome()
	if failure != "" || !filepath.IsAbs(home) || filepath.Clean(home) != home || home == "/" {
		return nil, "state.unavailable"
	}
	selected, failure := s.Store.locationAt(c, "approvals")
	if failure != "" {
		return nil, failure
	}
	selected = filepath.Dir(filepath.Dir(selected))
	fixed := filepath.Join(home, ".local", "state", "itos-container")
	if domain.Overlaps(fixed, c.Root) {
		return nil, "state.path"
	}
	// Metadata cannot also contain an ordinary selected state tree. Refuse before
	// bootstrap or claims can invalidate the catalog or its reservation scans.
	if domain.Exposed(selected, filepath.Join(fixed, "coordination")) || inReservationCollection(selected, fixed) {
		return nil, "state.path"
	}
	store := s.Store
	store.Control = nil
	ops := store.operations()
	ops.lock = func(*os.File, bool) error { return nil }
	store.ops = &ops
	h, failure := store.pinPath(c, false, home, 0)
	if failure != "" {
		return nil, failure
	}
	if h == nil {
		return nil, "state.unavailable"
	}
	if !accountDirectory(h) {
		h.close()
		return nil, "state.unsafe"
	}
	t := &controlTransaction{home: h, store: store, candidate: c, fixed: fixed, selected: selected, bindings: map[string]locationBinding{}, data: map[string][]byte{}}
	success := false
	defer func() {
		if !success {
			t.Close()
		}
	}()
	if failure = t.openCatalog(false); failure != "" {
		return nil, failure
	}
	if failure = t.acquire(ctx, false, write); failure != "" {
		return nil, failure
	}
	if failure = t.inspect(ctx, s.Reservations); failure != "" {
		return nil, failure
	}
	if write {
		if t.lock == nil {
			// No publication/registration can precede the private lock. Check
			// legacy evidence before mkdir, then re-read after racing creators
			// converge on the same validated inode.
			if !t.pathsUnchanged() {
				return nil, "state.path"
			}
			if failure = t.openCatalog(true); failure != "" {
				return nil, failure
			}
			if failure = t.acquire(ctx, true, true); failure != "" {
				return nil, failure
			}
			for _, p := range t.pins {
				p.close()
			}
			t.pins = nil
			t.bindings, t.data = map[string]locationBinding{}, map[string][]byte{}
			if failure = t.inspect(ctx, s.Reservations); failure != "" {
				return nil, failure
			}
		}
		if failure = t.enroll(); failure != "" {
			return nil, failure
		}
	}
	if !t.Unchanged() {
		return nil, "state.path"
	}
	success = true
	return t, ""
}

func inReservationCollection(selected, root string) bool {
	return domain.Exposed(selected, filepath.Join(root, "sessions")) ||
		domain.Exposed(selected, filepath.Join(root, "admission"))
}

// prospective retains every existing ancestor even when the managed tree is
// absent. The same pinned identities must survive pending-claim completion.
func (s Store) prospective(c port.Candidate, path string, depth int) (*pin, port.Failure) {
	for {
		p, f := s.pinPath(c, false, path, depth)
		if f != "" || p != nil {
			return p, f
		}
		parent := filepath.Dir(path)
		if parent == path || parent == "/" {
			return nil, "state.unavailable"
		}
		path, depth = parent, depth-1
	}
}
func (t *controlTransaction) inspect(ctx context.Context, reservations port.Reservations) port.Failure {
	if t.catalog != nil {
		if _, err := t.catalog.dir.Seek(0, 0); err != nil {
			return "state.unavailable"
		}
		names, f := pinnedNames(t.catalog)
		if f != "" {
			return f
		}
		for _, name := range names {
			if name == controlLockName {
				continue
			}
			if name != "self.json" && !recordName(name) && !locationTemp(name) {
				return "state.invalid"
			}
			data, f := t.readBinding(name)
			if f != "" {
				return f
			}
			if locationTemp(name) {
				continue
			}
			var binding locationBinding
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&binding) != nil || decoder.Decode(new(any)) != io.EOF || !bindingShape(data) {
				return "state.invalid"
			}
			if binding.Schema != 1 || binding.UID != t.candidate.UID || !filepath.IsAbs(binding.Path) || filepath.Clean(binding.Path) != binding.Path || binding.Path == "/" || len(binding.Ancestors) == 0 {
				return "state.invalid"
			}
			expected := locationName(binding.Path)
			if binding.Path == t.fixed {
				expected = "self.json"
				if !binding.Complete {
					return "state.invalid"
				}
			}
			if name != expected {
				return "state.invalid"
			}
			if _, exists := t.bindings[binding.Path]; exists {
				return "state.invalid"
			}
			t.bindings[binding.Path], t.data[binding.Path] = binding, data
		}
		if len(t.bindings) > 0 && t.lock == nil {
			return "state.unavailable"
		}
		if len(t.bindings) > 0 && t.data[t.fixed] == nil {
			return "state.invalid"
		}
	}
	paths := []string{t.fixed, t.selected}
	for path := range t.bindings {
		if path != t.fixed && path != t.selected {
			paths = append(paths, path)
		}
	}
	seen := map[string]bool{}
	for _, path := range paths {
		if seen[path] {
			continue
		}
		seen[path] = true
		if domain.Overlaps(path, t.candidate.Root) || inReservationCollection(t.selected, path) {
			return "state.path"
		}
		end, depth := path, 1
		binding, enrolled := t.bindings[path]
		if enrolled && path == t.fixed {
			end, depth = filepath.Join(path, "coordination"), 2
		}
		p, f := t.store.prospective(t.candidate, end, depth)
		if f != "" {
			return f
		}
		t.pins = append(t.pins, p)
		if enrolled {
			if f = validateLocation(binding, p, end); f != "" {
				return f
			}
		}
		if t.data[t.fixed] == nil {
			for _, collection := range []string{"admission", "sessions", "intents"} {
				evidence, f := t.store.pinPath(t.candidate, false, filepath.Join(path, collection), 2)
				if f != "" {
					return f
				}
				if evidence != nil {
					names, f := pinnedNames(evidence)
					evidence.close()
					if f != "" {
						return f
					}
					if len(names) != 0 {
						return "state.unavailable"
					}
				}
			}
		}
		roots, f := reservations.Roots(ctx, t.candidate, path)
		if f != "" {
			return f
		}
		if t.data[t.fixed] == nil && roots != nil {
			return "state.unavailable"
		}
		// A worker checkout inside a protected tree mounts that tree's records
		// as surely as one above it, so the comparison runs both ways.
		for _, root := range roots {
			if domain.Overlaps(t.fixed, root) || domain.Overlaps(t.selected, root) {
				return "state.exposed"
			}
		}
	}
	return ""
}
func locationTemp(name string) bool {
	return strings.HasPrefix(name, ".location-") && strings.HasSuffix(name, ".tmp") && len(name) == 46 && strings.Trim(name[10:42], "0123456789abcdef") == ""
}
func locationName(path string) string {
	data, _ := json.Marshal(path)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]) + ".json"
}
func pinIdentities(p *pin) []locationIdentity {
	identities := make([]locationIdentity, 0, len(p.infos))
	path := "/"
	parts := strings.Split(strings.TrimPrefix(p.path, "/"), "/")
	for i, info := range p.infos {
		path = filepath.Join(path, parts[i])
		stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
		identities = append(identities, locationIdentity{Path: path, Device: fmt.Sprint(stat.FieldByName("Dev")), Inode: fmt.Sprint(stat.FieldByName("Ino"))})
	}
	return identities
}
func validateLocation(b locationBinding, p *pin, end string) port.Failure {
	identities := pinIdentities(p)
	if len(b.Ancestors) > len(identities) {
		return "state.unavailable"
	}
	for i, bound := range b.Ancestors {
		if bound.Path != identities[i].Path || bound.Device == "" || bound.Inode == "" {
			return "state.invalid"
		}
		if _, err := strconv.ParseUint(bound.Device, 10, 64); err != nil {
			return "state.invalid"
		}
		if _, err := strconv.ParseUint(bound.Inode, 10, 64); err != nil {
			return "state.invalid"
		}
		if bound != identities[i] {
			return "state.path"
		}
	}
	if b.Complete {
		if p.path != end {
			return "state.unavailable"
		}
		if len(b.Ancestors) != len(identities) {
			return "state.invalid"
		}
	}
	return ""
}
func (t *controlTransaction) readBinding(name string) ([]byte, port.Failure) {
	p := t.catalog
	info, err := p.ops.lstat(p.root(), name)
	if err != nil {
		return nil, "state.unavailable"
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, "state.path"
	}
	if f := recordSafety(info, p.uid); f != "" {
		return nil, f
	}
	file, err := p.ops.openRecord(p.root(), name, os.O_RDONLY|noFollow, 0)
	if err != nil {
		return nil, "state.unavailable"
	}
	defer func() { _ = file.Close() }()
	actual, err := p.ops.statFile(file)
	if err != nil || !os.SameFile(info, actual) || recordSafety(actual, p.uid) != "" {
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
	if err != nil || !os.SameFile(info, latest) || recordSafety(latest, p.uid) != "" || !t.Unchanged() {
		return nil, "state.path"
	}
	return data, ""
}
func (t *controlTransaction) enroll() port.Failure {
	if !t.Unchanged() {
		return "state.path"
	}
	if t.catalog.ops.syncFile(t.lock) != nil {
		return "state.write"
	}
	if f := syncDirectories(t.catalog); f != "" {
		return f
	}
	if t.data[t.fixed] == nil {
		b := locationBinding{Schema: 1, UID: t.candidate.UID, Path: t.fixed, Complete: true, Ancestors: pinIdentities(t.catalog)}
		if f := t.publishBinding("self.json", b); f != "" {
			return f
		}
	} else if f := t.syncBinding("self.json", t.data[t.fixed]); f != "" {
		return f
	}
	if t.selected == t.fixed {
		return ""
	}
	b, exists := t.bindings[t.selected]
	if !exists {
		p, f := t.store.prospective(t.candidate, t.selected, 1)
		if f != "" {
			return f
		}
		t.pins = append(t.pins, p)
		b = locationBinding{Schema: 1, UID: t.candidate.UID, Path: t.selected, Complete: p.path == t.selected, Ancestors: pinIdentities(p)}
		if f := t.publishBinding(locationName(t.selected), b); f != "" {
			return f
		}
	} else if f := t.syncBinding(locationName(t.selected), t.data[t.selected]); f != "" {
		return f
	}
	if !b.Complete {
		if !t.Unchanged() {
			return "state.path"
		}
		p, f := t.store.pinPath(t.candidate, true, t.selected, 1)
		if f != "" {
			return f
		}
		t.pins = append(t.pins, p)
		if f = validateLocation(b, p, t.selected); f != "" {
			return f
		}
		if f = syncDirectories(p); f != "" {
			return f
		}
		b.Complete, b.Ancestors = true, pinIdentities(p)
		if f = t.publishBinding(locationName(t.selected), b); f != "" {
			return f
		}
	}
	return ""
}
func syncDirectories(p *pin) port.Failure {
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
	return ""
}
func (t *controlTransaction) syncBinding(name string, expected []byte) port.Failure {
	if !t.Unchanged() {
		return "state.path"
	}
	data, f := t.readBinding(name)
	if f != "" {
		return f
	}
	if !bytes.Equal(data, expected) {
		return "state.invalid"
	}
	return t.catalog.durableSnapshot(name, expected)
}
func (t *controlTransaction) publishBinding(name string, b locationBinding) port.Failure {
	p := t.catalog
	if !t.Unchanged() {
		return "state.path"
	}
	// The binding contains only JSON primitives; encoding cannot fail.
	data, _ := json.Marshal(b)
	var nonce [16]byte
	if _, err := p.ops.nonce(nonce[:]); err != nil {
		return "state.write"
	}
	temporary := ".location-" + hex.EncodeToString(nonce[:]) + ".tmp"
	file, err := p.ops.create(p.root(), temporary)
	if err != nil {
		return "state.write"
	}
	// An interrupted private file is retained as non-authoritative evidence.
	defer func() { _ = file.Close() }()
	if p.ops.chmod(file, 0600) != nil {
		return "state.write"
	}
	n, err := p.ops.write(file, data)
	if err != nil || n != len(data) {
		return "state.write"
	}
	if p.ops.syncFile(file) != nil || p.ops.closeFile(file) != nil {
		return "state.write"
	}
	if !t.Unchanged() {
		return "state.path"
	}
	if p.ops.rename(p.root(), temporary, name) != nil || p.ops.syncParent(p.dir) != nil {
		return "state.write"
	}
	if !t.Unchanged() {
		return "state.path"
	}
	t.bindings[b.Path], t.data[b.Path] = b, data
	return ""
}

// Locations supplies the session engine with protected, unlocked discovery. Its
// caller already holds account exclusion, and never waits for an operation lock.
type Locations struct{ Store Store }

func (s Locations) At(path string) port.Storage {
	store := s.Store
	store.Control = nil
	store.StateHome = filepath.Dir(path)
	return Storage{Store: store}
}
func (s Locations) Inspect(ctx context.Context, c port.Candidate, path string) (port.AdmissionDirectory, port.Failure) {
	store := s.Store
	store.Control = nil
	store.StateHome = filepath.Dir(path)
	ops := store.operations()
	ops.lock = func(*os.File, bool) error { return nil }
	store.ops = &ops
	// The selected root was pinned by Control, but its admission directory may
	// legitimately be absent (including legacy public-only state).
	p, f := store.pinPath(c, false, filepath.Join(path, "admission"), 2)
	if f != "" {
		return nil, f
	}
	return &admissionDirectory{directoryStore: directoryStore{pin: p, ctx: ctx}, store: store, candidate: c, sessions: filepath.Join(path, "sessions")}, ""
}
