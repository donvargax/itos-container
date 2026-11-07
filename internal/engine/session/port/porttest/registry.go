package porttest

import (
	"context"
	"strings"
	"time"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

type Registry struct {
	Store                                   *Store
	Admissions                              *Admissions
	Project                                 string
	Records                                 map[string]port.PublicRecord
	Leases                                  map[string]bool
	Locks                                   map[string]bool
	Readers                                 map[string]int
	Failure, SaveFailure, ControllerFailure port.Outcome
	Writes, Syncs                           int
	AfterRegister                           func()
	AfterClose                              func(string)
	AfterOpen                               func(string)
	BeforeSave                              func(port.PublicRecord)
	WaitForLock                             bool
	LockReleased                            <-chan struct{}
}

type boundRegistry struct {
	*Registry
	frozen approval.Durable
}

func (s *Registry) ForRun(d approval.Durable) port.Registry { return boundRegistry{s, d} }
func (s boundRegistry) Open(ctx context.Context, id string, readOnly bool) (port.Session, port.Outcome) {
	if s.frozen.Frozen.Project != s.Project {
		return nil, port.Refused
	}
	return s.Registry.Open(ctx, id, readOnly)
}

func copyPublic(r port.PublicRecord) port.PublicRecord {
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
func samePublic(a, b port.PublicRecord) bool {
	if (a.WorkloadExit == nil) != (b.WorkloadExit == nil) || (a.LastProblem == nil) != (b.LastProblem == nil) {
		return false
	}
	if a.WorkloadExit != nil && *a.WorkloadExit != *b.WorkloadExit {
		return false
	}
	if a.LastProblem != nil && *a.LastProblem != *b.LastProblem {
		return false
	}
	a.WorkloadExit, a.LastProblem, b.WorkloadExit, b.LastProblem = nil, nil, nil, nil
	return a == b
}
func (s *Registry) Register(ctx context.Context, d approval.Durable, id, token string) (port.Lease, port.Outcome) {
	if s.Failure != "" {
		return nil, s.Failure
	}
	if ctx.Err() != nil {
		return nil, port.Incomplete
	}
	if !port.HexID(id) || !port.HexID(token) || d.Frozen.Project != s.Project {
		return nil, port.Refused
	}
	if s.Records == nil {
		s.Records = map[string]port.PublicRecord{}
	}
	if s.Leases == nil {
		s.Leases = map[string]bool{}
	}
	if _, exists := s.Records[id]; exists {
		return nil, port.Refused
	}
	if s.SaveFailure != "" {
		return nil, s.SaveFailure
	}
	if s.Admissions != nil {
		if failure := s.Admissions.reserve(s.Store.Root, id, port.Reference(s.Project, token)); failure != "" {
			return nil, failure
		}
	}
	s.Records[id] = port.PublicRecord{Schema: 1, ID: id, Project: s.Project, Intent: port.Reference(s.Project, token), State: "preparing"}
	s.Leases[id] = true
	s.Writes++
	if s.AfterRegister != nil {
		s.AfterRegister()
	}
	return &sessionLease{s: s, id: id}, ""
}

type sessionLease struct {
	s  *Registry
	id string
}

func (l *sessionLease) Close() { l.s.Leases[l.id] = false }
func (s *Registry) List(ctx context.Context) ([]string, port.Outcome) {
	if s.Failure != "" {
		return nil, s.Failure
	}
	if ctx.Err() != nil {
		return nil, port.Incomplete
	}
	var ids []string
	for id, r := range s.Records {
		if r.Project == s.Project {
			ids = append(ids, id)
		}
	}
	return ids, ""
}
func (s *Registry) Open(ctx context.Context, id string, readOnly bool) (port.Session, port.Outcome) {
	if s.Failure != "" {
		return nil, s.Failure
	}
	if ctx.Err() != nil {
		return nil, port.Incomplete
	}
	r, ok := s.Records[id]
	if !ok || r.Project != s.Project {
		return nil, port.Refused
	}
	if r.ID != id {
		return nil, port.Invalid
	}
	if s.Locks[id] || (!readOnly && s.Readers[id] > 0) {
		if s.WaitForLock {
			select {
			case <-ctx.Done():
			case <-s.LockReleased:
				return s.Open(ctx, id, readOnly)
			}
		}
		return nil, port.Incomplete
	}
	i, f := s.Store.open(ctx, r.Intent, false)
	if f != "" {
		if f == port.Refused && s.Store.Records[r.Intent] == nil {
			f = port.Invalid
		}
		return nil, f
	}
	i.Close()
	if s.Locks == nil {
		s.Locks = map[string]bool{}
	}
	if readOnly {
		if s.Readers == nil {
			s.Readers = map[string]int{}
		}
		s.Readers[id]++
	} else {
		s.Locks[id] = true
	}
	entry := &memorySession{registry: s, record: copyPublic(r), intent: i, readOnly: readOnly}
	if s.AfterOpen != nil {
		s.AfterOpen(id)
	}
	return entry, ""
}

type memorySession struct {
	registry *Registry
	record   port.PublicRecord
	intent   *memoryIntent
	readOnly bool
}

func (s *memorySession) Record() port.PublicRecord { return copyPublic(s.record) }
func (s *memorySession) Intent() port.Intent       { return s.intent }
func (s *memorySession) Controller() (string, port.Outcome) {
	if s.registry.ControllerFailure != "" {
		return "unknown", s.registry.ControllerFailure
	}
	if s.registry.Leases[s.record.ID] {
		return "present", ""
	}
	return "absent", ""
}
func (s *memorySession) Validate() port.Outcome {
	if !samePublic(s.record, s.registry.Records[s.record.ID]) {
		return port.Refused
	}
	return s.intent.Validate()
}
func (s *memorySession) ValidateLaunch() port.Outcome {
	if f := s.Validate(); f != "" {
		return f
	}
	i := *s.intent
	i.fresh = true
	return i.Validate()
}
func (s *memorySession) Save(r port.PublicRecord) port.Outcome {
	if s.registry.BeforeSave != nil {
		s.registry.BeforeSave(copyPublic(r))
	}
	if s.readOnly {
		return port.Refused
	}
	if f := s.Validate(); f != "" {
		return f
	}
	if s.registry.SaveFailure != "" {
		return s.registry.SaveFailure
	}
	if r.ID != s.record.ID || r.Schema != s.record.Schema || r.Project != s.record.Project || r.Intent != s.record.Intent ||
		(s.record.State == "stopped" && (r.State != "stopped" || (r.LastProblem != nil && *r.LastProblem != port.StreamTimeout))) || (s.record.WorkloadExit != nil && (r.WorkloadExit == nil || *r.WorkloadExit != *s.record.WorkloadExit)) {
		return port.Refused
	}
	if samePublic(s.record, r) {
		return s.Sync()
	}
	s.record = copyPublic(r)
	s.registry.Records[r.ID] = copyPublic(r)
	s.registry.Writes++
	s.completeAdmission()
	return ""
}
func (s *memorySession) Sync() port.Outcome {
	if s.readOnly {
		return port.Refused
	}
	if f := s.Validate(); f != "" {
		return f
	}
	if s.registry.SaveFailure != "" {
		return s.registry.SaveFailure
	}
	s.registry.Syncs++
	s.completeAdmission()
	return ""
}
func (s *memorySession) Close() {
	if s.readOnly {
		s.registry.Readers[s.record.ID]--
	} else {
		s.registry.Locks[s.record.ID] = false
	}
	if s.registry.AfterClose != nil {
		s.registry.AfterClose(s.record.ID)
	}
}

func RegistryContract(t Reporter, open func() (port.Registry, port.Store, approval.Durable)) {
	t.Helper()
	registry, store, d := open()
	ctx := context.Background()
	id, token := strings.Repeat("a", 64), strings.Repeat("1", 64)
	if ids, f := registry.List(ctx); f != "" || len(ids) != 0 {
		t.Errorf("empty registry %v %s", ids, f)
	}
	lease, f := registry.Register(ctx, d, id, token)
	if f != "" {
		t.Errorf("register %s", f)
		return
	}
	defer lease.Close()
	if _, f = registry.Register(ctx, d, id, strings.Repeat("2", 64)); f != port.Refused {
		t.Errorf("occupied public identity replaced: %s", f)
	}
	if _, f = registry.Open(ctx, id, true); f != port.Invalid {
		t.Errorf("missing intent was adopted: %s", f)
	}
	i, f := store.New(ctx, d, token)
	if f != "" {
		t.Errorf("intent %s", f)
		return
	}
	i.Close()
	s, f := registry.Open(ctx, id, false)
	if f != "" {
		t.Errorf("open %s", f)
		return
	}
	if state, f := s.Controller(); state != "present" || f != "" {
		t.Errorf("controller %s %s", state, f)
	}
	r := s.Record()
	n := 7
	r.WorkloadExit = &n
	r.State = "failed"
	if f = s.Save(r); f != "" {
		t.Errorf("progress %s", f)
	}
	n = 1
	if result := s.Record(); result.WorkloadExit == nil || *result.WorkloadExit != 7 {
		t.Errorf("caller mutated stored exit")
	}
	s.Close()
	lease.Close()
	s, f = registry.Open(ctx, id, true)
	if f != "" {
		t.Errorf("read-only reconstruction %s", f)
		return
	}
	defer s.Close()
	if state, f := s.Controller(); state != "absent" || f != "" {
		t.Errorf("released controller %s %s", state, f)
	}
	if f = s.Save(s.Record()); f != port.Refused {
		t.Errorf("read-only session wrote state: %s", f)
	}
	if f = s.Sync(); f != port.Refused {
		t.Errorf("status repaired durability: %s", f)
	}
}

// RegistryLaunchContract separates fresh launch identity from protected cleanup
// authority on both a held session and a freshly opened recovery handle.
func RegistryLaunchContract(t Reporter, open func() (port.Registry, port.Store, approval.Durable, func())) {
	t.Helper()
	registry, store, frozen, replaceCheckout := open()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	id, token := strings.Repeat("a", 64), strings.Repeat("1", 64)
	lease, f := registry.Register(ctx, frozen, id, token)
	if f != "" {
		t.Errorf("register: %s", f)
		return
	}
	defer lease.Close()
	i, f := store.New(ctx, frozen, token)
	if f != "" {
		t.Errorf("intent: %s", f)
		return
	}
	i.Close()
	s, f := registry.Open(ctx, id, false)
	if f != "" {
		t.Errorf("open: %s", f)
		return
	}
	if f := s.ValidateLaunch(); f != "" {
		t.Errorf("approved launch refused: %s", f)
	}
	replaceCheckout()
	for attempt := range 2 {
		if attempt != 0 {
			s, f = registry.Open(ctx, id, true)
			if f != "" {
				t.Errorf("checkout replacement vetoed recovery: %s", f)
				return
			}
		}
		before := s.Record()
		if f := s.ValidateLaunch(); f != port.Refused {
			t.Errorf("substituted checkout authorized launch: %s", f)
		}
		if f := s.Validate(); f != "" || s.Record() != before {
			t.Errorf("launch refusal changed protected recovery authority: %s", f)
		}
		// Validation cannot retain a project-wide lock across engine calls.
		i, f := store.OpenCleanup(ctx, before.Intent)
		if f != "" {
			t.Errorf("launch validation retained intent authority: %s", f)
		} else {
			i.Close()
		}
		s.Close()
	}
}

func (s *memorySession) completeAdmission() {
	if s.registry.Admissions != nil && s.record.State == "stopped" && s.intent.Record().Completed() {
		s.registry.Admissions.complete(s.registry.Store.Root, s.record.ID)
	}
}
