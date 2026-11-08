package disk

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"path"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

type Registry struct {
	Store    Store
	Storage  approval.RegistryStorage
	Codec    port.PublicCodec
	Identity approval.ProjectIdentity
	frozen   *approval.Durable
}

func (s Registry) ForRun(frozen approval.Durable) port.Registry { s.frozen = &frozen; return s }

// The public registry is a protected index, not an approval identity. Its key
// must survive changes to writable checkout contents, including the Git inode.
// Each entry still resolves and validates its original intent and snapshot.
func registryKey(root, uid string) string {
	digest := sha256.Sum256([]byte("itos-container/session-index/v1\x00" + uid + "\x00" + root))
	return hex.EncodeToString(digest[:])
}

func (s Registry) lookup() (approval.Candidate, port.Outcome) {
	uid, gid, failure := s.Store.identity()
	if failure != "" {
		return approval.Candidate{}, failure
	}
	// identity() has selected Linux; interpret its paths independently of the
	// host running a portable contract test.
	root := path.Clean(s.Store.Input.Root)
	if s.frozen != nil {
		c, f := s.candidate()
		if f != "" {
			return approval.Candidate{}, f
		}
		root = c.Root
	}
	// Discovery uses only the selected absolute path. Opening the writable
	// checkout would let its worker veto recovery by changing permissions.
	// Open binds the original protected snapshot to this root before cleanup.
	if !path.IsAbs(root) {
		return approval.Candidate{}, port.Refused
	}
	return approval.Candidate{UID: uid, GID: gid, Root: root, Project: registryKey(root, uid)}, ""
}

func (s Registry) candidate() (approval.Candidate, port.Outcome) {
	uid, gid, failure := s.Store.identity()
	if failure != "" {
		return approval.Candidate{}, failure
	}
	if s.frozen != nil {
		b, f := s.Store.Codec.Frozen(s.frozen.Frozen)
		if f != "" {
			return approval.Candidate{}, f
		}
		if b.UID != uid || b.GID != gid {
			return approval.Candidate{}, port.Refused
		}
		return approval.Candidate{UID: uid, GID: gid, Root: b.Checkout.Root, Project: s.frozen.Frozen.Project}, ""
	}
	checkout := s.Store.Checkout
	if checkout.Pin(s.Store.Input.Root) != readiness.None {
		return approval.Candidate{}, port.Refused
	}
	defer checkout.Close()
	identity, f := checkout.Inspect()
	if f != readiness.None || !checkout.Unchanged() {
		return approval.Candidate{}, port.Refused
	}
	return approval.Candidate{UID: uid, GID: gid, Root: identity.Root, Project: s.Identity.Project(identity, uid)}, ""
}
func (s Registry) Register(ctx context.Context, frozen approval.Durable, id, token string) (port.Lease, port.Outcome) {
	c, failure := s.candidate()
	if failure != "" {
		return nil, failure
	}
	if c.Project != frozen.Frozen.Project || !port.HexID(id) || !port.HexID(token) {
		return nil, port.Refused
	}
	b, failure := s.Store.Codec.Frozen(frozen.Frozen)
	if failure != "" {
		return nil, failure
	}
	if b.UID != c.UID || b.GID != c.GID || b.Checkout.Root != c.Root {
		return nil, port.Refused
	}
	r := port.PublicRecord{Schema: 1, ID: id, Project: c.Project, Intent: port.Reference(c.Project, token), State: "preparing"}
	c.Project = registryKey(c.Root, c.UID)
	admission, failure := s.reserve(ctx, c, r)
	if failure != "" {
		return nil, failure
	}
	defer admission.Close()
	lease, f := s.Storage.Register(ctx, c, id, s.Codec.EncodePublic(r))
	if f != "" {
		return nil, storageFailure(f, true)
	}
	pending := port.AdmissionRecord{Schema: 1, Root: c.Root, ID: id, Intent: r.Intent, Pending: true}
	active := pending
	active.Pending = false
	if f = admission.Publish(admissionName(c.Project, id, c.UID), s.Codec.EncodeAdmission(pending), s.Codec.EncodeAdmission(active)); f != "" {
		lease.Close()
		return nil, storageFailure(f, true)
	}
	return lease, ""
}
func (s Registry) List(ctx context.Context) ([]string, port.Outcome) {
	c, failure := s.lookup()
	if failure != "" {
		return nil, failure
	}
	ids, f := s.Storage.List(ctx, c)
	if f != "" {
		return nil, storageFailure(f, false)
	}
	return ids, ""
}
func (s Registry) Open(ctx context.Context, id string, readOnly bool) (port.Session, port.Outcome) {
	c, failure := s.lookup()
	if failure != "" {
		return nil, failure
	}
	d, f := s.Storage.Open(ctx, c, id, readOnly)
	if f != "" {
		return nil, storageFailure(f, false)
	}
	if d == nil {
		return nil, port.Refused
	}
	success := false
	defer func() {
		if !success {
			d.Close()
		}
	}()
	data, f := d.Read("record.json")
	if f != "" {
		return nil, storageFailure(f, false)
	}
	r, failure := s.Codec.DecodePublic(data)
	if failure != "" {
		return nil, failure
	}
	if r.ID != id || (s.frozen != nil && r.Project != s.frozen.Frozen.Project) {
		return nil, port.Invalid
	}
	i, failure := s.Store.transaction(ctx, r.Intent, false)
	if failure != "" {
		return nil, failure
	}
	if i.Binding().Checkout.Root != c.Root {
		return nil, port.Refused
	}
	entry := &publicSession{registry: s, directory: d, record: r, data: data, intent: i, readOnly: readOnly}
	if failure = entry.Validate(); failure != "" {
		return nil, failure
	}
	success = true
	return entry, ""
}

type publicSession struct {
	registry  Registry
	directory approval.SessionDirectory
	record    port.PublicRecord
	data      []byte
	intent    *transactionIntent
	readOnly  bool
}

func (s *publicSession) Record() port.PublicRecord { return clonePublic(s.record) }
func clonePublic(r port.PublicRecord) port.PublicRecord {
	if r.WorkloadExit != nil {
		n := *r.WorkloadExit
		r.WorkloadExit = &n
	}
	if r.LastProblem != nil {
		f := *r.LastProblem
		r.LastProblem = &f
	}
	return r
}
func (s *publicSession) Intent() port.Intent { return s.intent }
func (s *publicSession) Close()              { s.directory.Close() }
func (s *publicSession) Controller() (string, port.Outcome) {
	state, f := s.directory.Controller()
	if f != "" {
		return state, storageFailure(f, false)
	}
	return state, ""
}
func (s *publicSession) Validate() port.Outcome {
	return s.validate(false)
}
func (s *publicSession) ValidateLaunch() port.Outcome {
	return s.validate(true)
}
func (s *publicSession) validate(fresh bool) port.Outcome {
	if !s.directory.Unchanged() {
		return port.Refused
	}
	data, f := s.directory.Read("record.json")
	if f != "" {
		return storageFailure(f, false)
	}
	if !bytes.Equal(data, s.data) {
		return port.Refused
	}
	// Keep this choice local to the short validation transaction. Subsequent
	// recovery reads/writes must not inherit a launch-only checkout dependency.
	i := *s.intent
	i.fresh = fresh
	return i.Validate()
}
func (s *publicSession) Save(next port.PublicRecord) port.Outcome {
	if s.readOnly {
		return port.Refused
	}
	if failure := s.Validate(); failure != "" {
		return failure
	}
	if next.Schema != s.record.Schema || next.ID != s.record.ID || next.Project != s.record.Project || next.Intent != s.record.Intent ||
		(s.record.MountDetector != "" && next.MountDetector != s.record.MountDetector) ||
		(s.record.State == "stopped" && (next.State != "stopped" || (next.LastProblem != nil && *next.LastProblem != port.StreamTimeout))) ||
		(s.record.WorkloadExit != nil && (next.WorkloadExit == nil || *next.WorkloadExit != *s.record.WorkloadExit)) {
		return port.Refused
	}
	data := s.registry.Codec.EncodePublic(next)
	if _, failure := s.registry.Codec.DecodePublic(data); failure != "" {
		return failure
	}
	if bytes.Equal(data, s.registry.Codec.EncodePublic(s.record)) {
		return s.Sync()
	}
	f := s.directory.Publish("record.json", s.data, data)
	if f != "" {
		visible, readFailure := s.directory.Read("record.json")
		if readFailure == "" && bytes.Equal(visible, data) {
			s.record, s.data = clonePublic(next), data
		}
		return storageFailure(f, true)
	}
	s.record, s.data = clonePublic(next), data
	if failure := s.Validate(); failure != "" {
		return failure
	}
	return s.completeAdmission()
}
func (s *publicSession) Sync() port.Outcome {
	if s.readOnly {
		return port.Refused
	}
	if failure := s.Validate(); failure != "" {
		return failure
	}
	if f := s.directory.Sync("record.json", s.data); f != "" {
		return storageFailure(f, true)
	}
	if failure := s.Validate(); failure != "" {
		return failure
	}
	return s.completeAdmission()
}

// A public session holds its operation lock while these short intent
// transactions reload and compare the complete preceding representation.
type transactionIntent struct {
	store     Store
	ctx       context.Context
	reference string
	record    port.Record
	binding   port.Binding
	data      []byte
	fresh     bool
}

func (s Store) transaction(ctx context.Context, ref string, fresh bool) (*transactionIntent, port.Outcome) {
	opened, failure := s.openMissing(ctx, ref, fresh, port.Invalid)
	if failure != "" {
		return nil, failure
	}
	defer opened.Close()
	i := opened.(*intent)
	return &transactionIntent{store: s, ctx: budget.Lifetime(ctx), reference: ref, record: i.record, binding: i.binding, data: bytes.Clone(i.bytes), fresh: fresh}, ""
}
func (t *transactionIntent) Record() port.Record   { return t.record }
func (t *transactionIntent) Binding() port.Binding { return t.binding }
func (*transactionIntent) Close()                  {}
func (t *transactionIntent) open() (*intent, port.Outcome) {
	i, failure := t.store.open(t.ctx, t.reference, t.fresh)
	if failure != "" {
		return nil, failure
	}
	current := i.(*intent)
	if !bytes.Equal(t.data, current.bytes) || current.binding != t.binding {
		i.Close()
		return nil, port.Refused
	}
	return current, ""
}
func (t *transactionIntent) Validate() port.Outcome {
	i, failure := t.open()
	if failure != "" {
		return failure
	}
	i.Close()
	return ""
}
func (t *transactionIntent) Save(next port.Record) port.Outcome {
	i, failure := t.open()
	if failure != "" {
		return failure
	}
	defer i.Close()
	failure = i.Save(next)
	t.record, t.data = i.record, bytes.Clone(i.bytes)
	return failure
}
