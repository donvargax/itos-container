// Package porttest supplies stateful, I/O-free session fakes and shared contracts.
package porttest

import (
	"context"
	"strings"
	"sync"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

type WriteEvent struct {
	Operation, Stage, Bytes string
	Failed                  bool
}
type Store struct {
	Codec                        port.Codec
	Records                      map[string][]byte
	Snapshots                    map[string][]byte
	UID, GID, Root               string
	UnsafeRecord, UnsafeSnapshot bool
	Fault                        string
	Events                       []WriteEvent
	Locked                       bool
	// AfterSave changes fixture state only after a successful durable update.
	// Barriers are test controls, not serializable subprocess observations.
	AfterSave func() `json:"-"`
}
type memoryIntent struct {
	store   *Store
	ref     string
	record  port.Record
	binding port.Binding
	bytes   string
	fresh   bool
}

func (s *Store) open(ctx context.Context, ref string, fresh bool) (*memoryIntent, port.Outcome) {
	if ctx.Err() != nil {
		return nil, port.Incomplete
	}
	if s.Locked {
		return nil, port.Incomplete
	}
	project, token, ok := port.ParseReference(ref)
	if !ok {
		return nil, port.Invalid
	}
	if s.UnsafeRecord {
		return nil, port.Unsafe
	}
	if s.Records == nil {
		s.Records = map[string][]byte{}
	}
	data := s.Records[ref]
	if data == nil {
		return nil, port.Refused
	}
	r, failure := s.Codec.Decode(data)
	if failure != "" {
		return nil, failure
	}
	if r.Project != project || r.NameToken() != token {
		return nil, port.Invalid
	}
	if s.UnsafeSnapshot {
		return nil, port.Unsafe
	}
	b, failure := s.Codec.Snapshot(s.Snapshots[r.Snapshot])
	if failure != "" {
		return nil, failure
	}
	expected := b.Intent(token)
	expected.Stage, expected.ContainerID = r.Stage, r.ContainerID
	if expected != r {
		return nil, port.Invalid
	}
	if b.UID != s.UID || b.GID != s.GID || (fresh && b.Checkout.Root != s.Root) || s.UID == "0" {
		return nil, port.Refused
	}
	s.Locked = true
	return &memoryIntent{store: s, ref: ref, record: r, binding: b, bytes: string(data), fresh: fresh}, ""
}
func (s *Store) Open(ctx context.Context, ref string) (port.Intent, port.Outcome) {
	return s.open(ctx, ref, true)
}
func (s *Store) OpenCleanup(ctx context.Context, ref string) (port.Intent, port.Outcome) {
	return s.open(ctx, ref, false)
}
func (s *Store) New(ctx context.Context, d approval.Durable, token string) (port.Intent, port.Outcome) {
	if !port.HexID(token) || !port.HexID(d.Frozen.Project) {
		return nil, port.Invalid
	}
	if ctx.Err() != nil || s.Locked {
		return nil, port.Incomplete
	}
	ref := port.Reference(d.Frozen.Project, token)
	if s.UnsafeRecord {
		return nil, port.Unsafe
	}
	if s.Records == nil {
		s.Records = map[string][]byte{}
	}
	if s.Records[ref] != nil {
		return nil, port.Refused
	}
	b, failure := s.Codec.Frozen(d.Frozen)
	if failure != "" {
		return nil, failure
	}
	r := b.Intent(token)
	if s.UnsafeSnapshot {
		return nil, port.Unsafe
	}
	snapshot, failure := s.Codec.Snapshot(s.Snapshots[r.Snapshot])
	if failure != "" {
		return nil, failure
	}
	if snapshot != b {
		return nil, port.Invalid
	}
	if b.UID != s.UID || b.GID != s.GID || b.Checkout.Root != s.Root || s.UID == "0" {
		return nil, port.Refused
	}
	if !s.publish(ref, r) {
		return nil, port.IntentWrite
	}
	return s.open(ctx, ref, true)
}
func (s *Store) publish(ref string, r port.Record) bool {
	data := s.Codec.Encode(r)
	for _, operation := range []string{"new ancestor fsync", "private file creation", "write", "file fsync", "atomic publication", "parent fsync"} {
		prefix := r.Stage
		if prefix != "intent" {
			prefix += "-record"
		}
		fault := prefix + " " + operation
		failed := s.Fault == fault || (operation == "write" && s.Fault == prefix+" short write") || (r.Stage == "owned" && operation == "file fsync" && strings.HasPrefix(s.Fault, "owned-record file fsync then "))
		s.Events = append(s.Events, WriteEvent{Operation: operation, Stage: r.Stage, Bytes: string(data), Failed: failed})
		if failed {
			return false
		}
		if operation == "atomic publication" {
			s.Records[ref] = append([]byte{}, data...)
		}
	}
	return true
}
func (i *memoryIntent) Record() port.Record   { return i.record }
func (i *memoryIntent) Binding() port.Binding { return i.binding }
func (i *memoryIntent) Close()                { i.store.Locked = false }
func (i *memoryIntent) Validate() port.Outcome {
	if string(i.store.Records[i.ref]) != i.bytes {
		return port.Refused
	}
	if i.store.UnsafeRecord || i.store.UnsafeSnapshot {
		return port.Unsafe
	}
	b, f := i.store.Codec.Snapshot(i.store.Snapshots[i.record.Snapshot])
	if f != "" {
		return f
	}
	if b != i.binding {
		return port.Invalid
	}
	if b.UID != i.store.UID || b.GID != i.store.GID || (i.fresh && b.Checkout.Root != i.store.Root) {
		return port.Refused
	}
	return ""
}
func (i *memoryIntent) Save(next port.Record) port.Outcome {
	if f := i.Validate(); f != "" {
		return f
	}
	expected := i.record
	expected.Stage, expected.ContainerID = next.Stage, next.ContainerID
	if next != expected || !port.HexID(next.ContainerID) || (next.Stage != "owned" && next.Stage != "absent") || (i.record.Stage == "absent" && next != i.record) || (i.record.ContainerID != "" && next.ContainerID != i.record.ContainerID) {
		return port.Invalid
	}
	if next == i.record {
		for _, operation := range []string{"file fsync", "parent fsync", "new ancestor fsync"} {
			failed := i.store.Fault == next.Stage+"-record "+operation
			i.store.Events = append(i.store.Events, WriteEvent{Operation: operation, Stage: next.Stage, Bytes: i.bytes, Failed: failed})
			if failed {
				return port.IntentWrite
			}
		}
		if i.store.AfterSave != nil {
			i.store.AfterSave()
		}
		return ""
	}
	ok := i.store.publish(i.ref, next)
	if string(i.store.Records[i.ref]) == string(i.store.Codec.Encode(next)) {
		i.record = next
		i.bytes = string(i.store.Records[i.ref])
	}
	if !ok {
		return port.IntentWrite
	}
	if i.store.AfterSave != nil {
		i.store.AfterSave()
	}
	return ""
}

type EngineEvent struct {
	Method, Target string
	Record         port.Record
	Observation    port.Observation
	Status         port.Inspection
	Intent         string
	Writes         []WriteEvent
}
type Engine struct {
	// Resources may be inspected while a competing controller signals a worker.
	// Tests may access the exported map only while those operations are quiescent.
	resourcesMu                           sync.Mutex
	Unavailable                           bool
	Endpoint, DaemonID, CreateID          string
	Resources                             map[string]port.Observation
	Events                                []EngineEvent
	Store                                 *Store
	Inspection, Following, Removal, Fault string
	DisagreeName                          bool
	ImageInspection                       string
	Replaced, Removed                     bool
	// Barriers change fixture state at acquisition and observation boundaries.
	AfterOpen, BeforeInspect, AfterInspect func() `json:"-"`
}
type memoryConnection struct {
	engine  *Engine
	binding port.Binding
}

func (e *Engine) compound() bool {
	if e.Unavailable {
		return true
	}
	if e.Fault != "owned-record file fsync then unavailable" {
		return false
	}
	for _, event := range e.Store.Events {
		if event.Stage == "owned" && event.Operation == "file fsync" && event.Failed {
			return true
		}
	}
	return false
}
func (e *Engine) Open(ctx context.Context, b port.Binding) (port.Connection, port.Outcome) {
	c := &memoryConnection{engine: e, binding: b}
	if f := c.Check(ctx); f != "" {
		return nil, f
	}
	if e.AfterOpen != nil {
		e.AfterOpen()
	}
	return c, ""
}
func (c *memoryConnection) Check(ctx context.Context) port.Outcome {
	e := c.engine
	e.Events = append(e.Events, EngineEvent{Method: "INFO"})
	if e.Replaced || e.Endpoint != c.binding.Endpoint || e.DaemonID != c.binding.EngineID {
		return port.Refused
	}
	if ctx.Err() != nil || e.compound() {
		return port.Incomplete
	}
	return ""
}
func (c *memoryConnection) Create(ctx context.Context, r port.Record) (string, port.Outcome) {
	e := c.engine
	if ctx.Err() != nil {
		return "", port.Uncertain
	}
	e.Events = append(e.Events, EngineEvent{Method: "IMAGE", Target: r.ImageID})
	switch e.ImageInspection {
	case "declared volume", "different image config ID":
		return "", port.Refused
	case "missing Config", "unavailable", "malformed response":
		return "", port.Uncertain
	}
	o := port.Observation{ID: e.CreateID, Name: r.Name, ImageID: r.ImageID, Token: r.NameToken(), Project: r.Project, Fingerprint: r.Fingerprint, StatePresent: true, Status: "created"}
	e.resourcesMu.Lock()
	if e.Resources == nil {
		e.Resources = map[string]port.Observation{}
	}
	e.Resources[o.ID] = o
	e.resourcesMu.Unlock()
	ref := port.Reference(r.Project, r.NameToken())
	e.Events = append(e.Events, EngineEvent{Method: "CREATE", Target: r.Name, Record: r, Observation: o, Intent: string(e.Store.Records[ref]), Writes: append([]WriteEvent{}, e.Store.Events...)})
	if e.Fault == "lost create response" || e.Fault == "empty create ID" || e.Fault == "delayed create" {
		return "", port.Uncertain
	}
	return o.ID, ""
}
func (c *memoryConnection) Inspect(ctx context.Context, target string) (port.Observation, port.Inspection) {
	e := c.engine
	if e.BeforeInspect != nil {
		e.BeforeInspect()
	}
	o, status := c.inspect(ctx, target)
	e.Events = append(e.Events, EngineEvent{Method: "INSPECT", Target: target, Observation: o, Status: status})
	if e.Inspection == "replace socket" {
		e.Replaced = true
		e.Inspection = ""
	}
	if e.AfterInspect != nil {
		e.AfterInspect()
	}
	return o, status
}
func (c *memoryConnection) inspect(ctx context.Context, target string) (port.Observation, port.Inspection) {
	e := c.engine
	if e.Replaced {
		return port.Observation{}, port.Changed
	}
	if ctx.Err() != nil || e.compound() {
		return port.Observation{}, port.Unavailable
	}
	condition := e.Inspection
	if e.Removed || e.Fault == "delayed create" {
		condition = e.Following
	}
	switch condition {
	case "unavailable", "malformed response":
		return port.Observation{}, port.Unavailable
	case "explicit not-found":
		return port.Observation{}, port.NotFound
	}
	e.resourcesMu.Lock()
	defer e.resourcesMu.Unlock()
	o, exists := e.Resources[target]
	if !exists {
		for _, candidate := range e.Resources {
			if candidate.Name == target {
				o, exists = candidate, true
				break
			}
		}
	}
	if !exists {
		return port.Observation{}, port.NotFound
	}
	if !e.DisagreeName || target != o.Name {
		o = mutateObservation(o, condition)
	}
	return o, port.Observed
}
func mutateObservation(o port.Observation, condition string) port.Observation {
	switch condition {
	case "mounted volume":
		o.Mounts = true
	case "configured volume":
		o.Volumes = true
	case "different full container ID":
		o.ID = strings.Repeat("e", 64)
	case "different name":
		o.Name = "foreign"
	case "different image config ID":
		o.ImageID = "sha256:" + strings.Repeat("e", 64)
	case "different intent label":
		o.Token = "foreign"
	case "different project label":
		o.Project = "foreign"
	case "different fingerprint label":
		o.Fingerprint = "foreign"
	case "missing State":
		o.StatePresent = false
	case "running state":
		o.Status = "running"
		o.Running = true
	case "exited state after starting":
		o.Status = "exited"
		o.Started = true
	case "created state with nonzero PID":
		o.PID = 1
	case "created state with StartedAt":
		o.Started = true
	}
	return o
}
func (c *memoryConnection) Remove(ctx context.Context, id string) {
	if ctx.Err() != nil || c.engine.Replaced {
		return
	}
	e := c.engine
	e.Removed = true
	e.resourcesMu.Lock()
	defer e.resourcesMu.Unlock()
	resource := e.Resources[id]
	ref := port.Reference(resource.Project, resource.Token)
	e.Events = append(e.Events, EngineEvent{Method: "REMOVE", Target: id, Intent: string(e.Store.Records[ref]), Writes: append([]WriteEvent{}, e.Store.Events...)})
	if e.Following != "still present" {
		delete(e.Resources, id)
	}
}
func (*memoryConnection) Close() {}
