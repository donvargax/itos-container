package port

import (
	"context"
	"strconv"
	"strings"
)

const (
	MountWrite       Outcome = "mount-write"
	MountUnavailable Outcome = "mount-unavailable"
	MountRefused     Outcome = "mount-refused"
	MountUncertain   Outcome = "mount-uncertain"
)

// Anchor is protected authority for one descriptor-backed local volume. The
// source is a controller descriptor, never the mutable checkout pathname.
type Anchor struct {
	Session string `json:"session"`
	Name    string `json:"name"`
	Source  string `json:"source"`
	Device  string `json:"device"`
	Inode   string `json:"inode"`
	Created string `json:"created"`
	Stage   string `json:"stage"`
}

type MountSource struct{ Path, Device, Inode string }

// AnchorWorkspace keeps its source open until Close. Mounted observes the
// worker's mounted root using only the PID from a fresh full-ID inspection.
type AnchorWorkspace interface {
	Workspace
	Source() (MountSource, Outcome)
	Mounted(int) (MountSource, Outcome)
}

type AnchorObservation struct {
	Name, Created, Driver, Scope, Source, Type, Options string
	Token, Project, Fingerprint, Session, Device, Inode string
	ExactOptions, ExactLabels                           bool
}

type AnchorConnection interface {
	AllocateAnchor(context.Context, Record) Outcome
	InspectAnchor(context.Context, string) (AnchorObservation, Inspection)
	RemoveAnchor(context.Context, string)
}

func DescriptorPath(path string) bool {
	p := strings.Split(path, "/")
	if len(p) != 5 || p[0] != "" || p[1] != "proc" || p[3] != "fd" {
		return false
	}
	pid, pe := strconv.ParseUint(p[2], 10, 32)
	fd, fe := strconv.ParseUint(p[4], 10, 32)
	return pe == nil && fe == nil && pid > 0 && strconv.FormatUint(pid, 10) == p[2] && strconv.FormatUint(fd, 10) == p[4]
}

// ValidAnchor binds the new resource to the original snapshot and operation.
func (r Record) ValidAnchor(b Binding) bool {
	if r.Schema == 1 {
		return r.Anchor == (Anchor{})
	}
	a := r.Anchor
	return r.Schema == 2 && HexID(a.Session) && a.Name == "itos-container-anchor-"+r.NameToken() &&
		DescriptorPath(a.Source) && a.Device == b.Checkout.Device && a.Inode == b.Checkout.Inode &&
		a.Device != "" && a.Inode != "" && ((a.Stage == "intent" && a.Created == "") ||
		((a.Stage == "owned" || a.Stage == "absent") && a.Created != ""))
}

func (r Record) OwnsAnchor(o AnchorObservation) bool {
	a := r.Anchor
	return o.Name == a.Name && o.Created != "" && (a.Created == "" || o.Created == a.Created) &&
		o.Driver == "local" && o.Scope == "local" && o.Source == a.Source && o.Type == "none" && o.Options == "bind" &&
		o.ExactOptions && o.ExactLabels && o.Token == r.NameToken() && o.Project == r.Project && o.Fingerprint == r.Fingerprint &&
		o.Session == a.Session && o.Device == a.Device && o.Inode == a.Inode
}

func (r Record) anchorTransition(next Record) bool {
	if r.Schema == 1 {
		return next.Schema == 1 && next.Anchor == (Anchor{}) ||
			r.Stage == "reserved" && r.ContainerID == "" && next.Schema == 2 &&
				next.Stage == r.Stage && next.ContainerID == "" && next.Anchor.Stage == "intent" && next.Anchor.Created == "" &&
				HexID(next.Anchor.Session) && next.Anchor.Name == "itos-container-anchor-"+r.NameToken() && DescriptorPath(next.Anchor.Source)
	}
	if next.Schema != 2 {
		return false
	}
	a, n := r.Anchor, next.Anchor
	expected := a
	expected.Stage, expected.Created = n.Stage, n.Created
	if n != expected || (a.Created != "" && a.Created != n.Created) {
		return false
	}
	if a.Stage == "absent" {
		return n == a
	}
	if a.Stage == n.Stage {
		return a.Created == n.Created
	}
	if a.Stage == "intent" {
		return n.Stage == "owned" && n.Created != "" && next.Stage == r.Stage
	}
	return a.Stage == "owned" && n.Stage == "absent" && (next.Stage == "absent" || next.Stage == "reserved" || next.Stage == "cancelled")
}
