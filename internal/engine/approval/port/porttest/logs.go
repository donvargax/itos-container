package porttest

import (
	"bytes"
	"context"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	budget "github.com/donvargax/itos-container/internal/engine/budget/port"
)

// Stage models a session's staged log in memory. It never waits for the
// session's writer or readers: the staged log holds no operation lock.
func (s *Registry) Stage(ctx context.Context, c port.Candidate, id string) (port.StagedLog, port.Failure) {
	if !registryID(id) {
		return nil, "state.invalid"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return nil, "state.unavailable"
	}
	if s.Failure != "" {
		return nil, s.Failure
	}
	f := s.Sessions[c.Project+"/"+id]
	if f == nil {
		return nil, ""
	}
	if f.Unsafe {
		return nil, "state.unsafe"
	}
	f.Staged++
	return &stagedLog{s: s, files: f, id: id, project: c.Project, ctx: budget.Lifetime(ctx)}, ""
}

// SessionLog returns a session's published log, nil when none was published,
// and the number of its handles staged and not yet released.
func (s *Registry) SessionLog(project, id string) ([]byte, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.Sessions[project+"/"+id]
	if f == nil {
		return nil, 0
	}
	return bytes.Clone(f.Log), f.Staged
}

// SessionReport returns a session's published report, nil when none was
// published, with the log it was committed beside.
func (s *Registry) SessionReport(project, id string) ([]byte, []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.Sessions[project+"/"+id]
	if f == nil {
		return nil, nil
	}
	return bytes.Clone(f.Log), bytes.Clone(f.Report)
}

// Evidence reports a session's published evidence. The memory fake has no
// filesystem, so the path names the session's protected destination by its
// lookup key alone.
func (s *Registry) Evidence(ctx context.Context, c port.Candidate, id string) (port.SessionEvidence, port.Failure) {
	if !registryID(id) {
		return port.SessionEvidence{}, "state.invalid"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return port.SessionEvidence{}, "state.unavailable"
	}
	if s.Failure != "" {
		return port.SessionEvidence{}, s.Failure
	}
	f := s.Sessions[c.Project+"/"+id]
	if f == nil {
		return port.SessionEvidence{}, ""
	}
	if f.Unsafe {
		return port.SessionEvidence{}, "state.unsafe"
	}
	return port.SessionEvidence{Path: "sessions/" + c.Project + "/" + id + "/egress-refusals.jsonl", Log: bytes.Clone(f.Log), Report: bytes.Clone(f.Report)}, ""
}

// stagedLog keeps its bytes with the handle until Publish. Its events are a
// record publication's: write, file fsync, atomic publication, parent fsync.
type stagedLog struct {
	s                  *Registry
	files              *SessionFiles
	id, project        string
	ctx                context.Context
	data, report       []byte
	poisoned, released bool
}

func (l *stagedLog) event(operation string, data []byte) bool {
	failed := l.s.Fault == operation
	l.s.Events = append(l.s.Events, RegistryEvent{ID: l.id, Operation: operation, Data: bytes.Clone(data), Failed: failed})
	return failed
}
func (l *stagedLog) unchanged() bool {
	return l.ctx.Err() == nil && !l.files.Changed && !l.files.Unsafe
}
func (l *stagedLog) Write(data []byte) port.Failure {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	if l.released || l.poisoned {
		return "state.write"
	}
	if l.event("write", data) {
		l.poisoned = true
		return "state.write"
	}
	l.data = append(l.data, data...)
	return ""
}

// Attach stages the report published beside the log, after its writes.
func (l *stagedLog) Attach(data []byte) port.Failure {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	if l.released || l.poisoned {
		return "state.write"
	}
	if l.report != nil {
		return "state.conflict"
	}
	l.report = bytes.Clone(data)
	return ""
}

// Path names the destination the staged log is published to.
func (l *stagedLog) Path() string {
	return "sessions/" + l.project + "/" + l.id + "/egress-refusals.jsonl"
}

func (l *stagedLog) Publish() port.Failure {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	if l.released {
		return "state.conflict"
	}
	l.released = true
	l.files.Staged--
	if l.poisoned || l.event("file fsync", l.data) {
		return "state.write"
	}
	if !l.unchanged() {
		return "state.path"
	}
	if l.event("atomic publication", l.data) {
		return "state.write"
	}
	l.files.Log = append([]byte{}, l.data...)
	if l.event("parent fsync", l.data) {
		return "state.write"
	}
	if l.report != nil {
		l.files.Report = append([]byte{}, l.report...)
	}
	if !l.unchanged() {
		return "state.path"
	}
	return ""
}
func (l *stagedLog) Close() {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	if l.released {
		return
	}
	l.released = true
	l.files.Staged--
}
