package porttest

import (
	"bytes"
	"context"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

func (s *Registry) Admission(ctx context.Context, _ port.Candidate) (port.AdmissionDirectory, port.Failure) {
	s.mu.Lock()
	if s.admissionGate == nil {
		s.admissionGate = make(chan struct{}, 1)
	}
	gate := s.admissionGate
	s.mu.Unlock()
	select {
	case gate <- struct{}{}:
	case <-ctx.Done():
		return nil, "state.busy"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Failure != "" {
		<-gate
		return nil, s.Failure
	}
	if s.Admissions == nil {
		s.Admissions = map[string][]byte{}
	}
	return &admissionDirectory{registry: s, ctx: ctx}, ""
}

type admissionDirectory struct {
	registry *Registry
	ctx      context.Context
}

func (d *admissionDirectory) Close()          { <-d.registry.admissionGate }
func (d *admissionDirectory) Unchanged() bool { return d.ctx.Err() == nil }
func (d *admissionDirectory) Names() ([]string, port.Failure) {
	d.registry.mu.Lock()
	defer d.registry.mu.Unlock()
	var names []string
	for name := range d.registry.Admissions {
		names = append(names, name)
	}
	return names, ""
}
func (d *admissionDirectory) Read(name string) ([]byte, port.Failure) {
	d.registry.mu.Lock()
	defer d.registry.mu.Unlock()
	return bytes.Clone(d.registry.Admissions[name]), d.registry.AdmissionFailure
}
func (d *admissionDirectory) Publish(name string, previous, next []byte) port.Failure {
	d.registry.mu.Lock()
	defer d.registry.mu.Unlock()
	if f := d.registry.AdmissionFailure; f != "" {
		return f
	}
	if !d.Unchanged() {
		return "state.path"
	}
	if !bytes.Equal(d.registry.Admissions[name], previous) {
		return "state.conflict"
	}
	d.registry.Admissions[name] = bytes.Clone(next)
	return ""
}
func (d *admissionDirectory) Sync(name string, expected []byte) port.Failure {
	data, failure := d.Read(name)
	if failure != "" {
		return failure
	}
	if !bytes.Equal(data, expected) || data == nil {
		return "state.invalid"
	}
	return ""
}
func (d *admissionDirectory) Sessions() ([]port.RegisteredSession, port.Failure) {
	d.registry.mu.Lock()
	defer d.registry.mu.Unlock()
	if d.registry.ReadFailure != "" {
		return nil, d.registry.ReadFailure
	}
	var records []port.RegisteredSession
	for key, files := range d.registry.Sessions {
		if files.Unsafe || files.Changed {
			return nil, "state.path"
		}
		index, id, _ := strings.Cut(key, "/")
		records = append(records, port.RegisteredSession{Index: index, ID: id, Data: bytes.Clone(files.Data)})
	}
	return records, ""
}
