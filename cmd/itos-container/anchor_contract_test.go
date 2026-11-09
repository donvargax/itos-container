package main

import (
	"context"
	"strings"

	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

// The red checkpoint delegates where a capability exists and otherwise exposes
// its absence. No decorator manufactures a successful product lifecycle.
type anchorWorkspace struct {
	sp.Workspace
	observer *offlineObserver
}

func (w *anchorWorkspace) HoldProcess(pid int) (sp.Process, sp.Outcome) {
	if actual, ok := w.Workspace.(sp.ProcessWorkspace); ok {
		return actual.HoldProcess(pid)
	}
	return nil, sp.MountRefused
}

func (w *anchorWorkspace) Source() (sp.MountSource, sp.Outcome) {
	s, f := sp.MountSource{}, sp.MountUnavailable
	if actual, ok := w.Workspace.(sp.AnchorWorkspace); ok {
		s, f = actual.Source()
	}
	switch w.observer.request.AnchorFault {
	case "anchoring facility unavailable":
		s, f = sp.MountSource{}, sp.MountUnavailable
	case "opened checkout descriptor has a different checkout inode":
		s.Inode += "1"
	}
	w.observer.event(offlineEvent{Operation: "source", Source: s, Failure: string(f)})
	return s, f
}

func (w *anchorWorkspace) Mounted(pid int) (sp.MountSource, sp.Outcome) {
	s, f := sp.MountSource{}, sp.MountRefused
	if actual, ok := w.Workspace.(sp.AnchorWorkspace); ok {
		s, f = actual.Mounted(pid)
	}
	switch w.observer.request.AnchorFault {
	case "mounted workspace device differs":
		s.Device += "1"
	case "mounted workspace inode differs":
		s.Inode += "1"
	case "mounted workspace stat unavailable":
		s, f = sp.MountSource{}, sp.MountRefused
	}
	w.observer.event(offlineEvent{Operation: "mounted stat", Source: s, Exit: pid, Failure: string(f)})
	return s, f
}

func (w *anchorWorkspace) Close() {
	w.Workspace.Close()
	w.observer.event(offlineEvent{Operation: "descriptor close"})
}

func (o *offlineObserver) anchorPublication(r sp.Record) {
	if o.request.AnchorFixture != "true" {
		return
	}
	fault := ""
	prefix := "anchor " + r.Anchor.Stage + " "
	if r.Anchor.Stage == "owned" {
		prefix = "anchor ownership "
	}
	if r.Anchor.Stage == "absent" {
		prefix = "anchor completion "
	}
	if strings.HasPrefix(o.request.AnchorFault, prefix) {
		operation := strings.TrimSuffix(strings.TrimPrefix(o.request.AnchorFault, prefix), " fails")
		if operation == "ancestor fsync" {
			operation = "new ancestor fsync"
		}
		stage := r.Stage
		if stage != "intent" {
			stage += "-record"
		}
		fault = stage + " " + operation
	}
	o.ops.fault = fault
	if r.Stage == "reserved" {
		o.ops.fault = strings.Replace(fault, "reserved-record ", "reserved ", 1)
	}
	if o.memory != nil {
		o.memory.Store.Fault = fault
	}
}

func (c offlineConnection) AllocateAnchor(ctx context.Context, r sp.Record) sp.Outcome {
	f := sp.MountUnavailable
	if actual, ok := c.RunConnection.(sp.AnchorConnection); ok {
		f = actual.AllocateAnchor(ctx, r)
	}
	if c.o.request.AnchorFault == "allocation response lost and reconciliation unavailable" {
		f = sp.MountUncertain
	}
	c.o.event(offlineEvent{Operation: "anchor allocate", Failure: string(f)})
	return f
}

func (c offlineConnection) InspectAnchor(ctx context.Context, name string) (sp.AnchorObservation, sp.Inspection) {
	o, status := sp.AnchorObservation{}, sp.Unavailable
	if actual, ok := c.RunConnection.(sp.AnchorConnection); ok {
		o, status = actual.InspectAnchor(ctx, name)
	}
	c.o.anchorInspections++
	switch c.o.request.AnchorFault {
	case "allocation response lost and reconciliation unavailable":
		o, status = sp.AnchorObservation{}, sp.Unavailable
	case "protected source replaced after ownership publication":
		if c.o.record.Anchor.Stage == "owned" {
			o.Token = "foreign"
		}
	case "anchor identity is now foreign":
		if c.o.removed {
			o.Token = "foreign"
		}
	}
	c.o.event(offlineEvent{Operation: "anchor inspect", ID: name, Anchor: o, Status: status})
	return o, status
}

func (c offlineConnection) RemoveAnchor(ctx context.Context, name string) {
	if c.o.request.AnchorFault != "owned anchor release fails" {
		if actual, ok := c.RunConnection.(sp.AnchorConnection); ok {
			actual.RemoveAnchor(ctx, name)
		}
	}
	c.o.event(offlineEvent{Operation: "anchor remove", ID: name})
}

func (c offlineConnection) Signal(ctx context.Context, id, signal string) sp.Outcome {
	f := c.RunConnection.Signal(ctx, id, signal)
	c.o.event(offlineEvent{Operation: signal, ID: id, Failure: string(f)})
	return f
}
