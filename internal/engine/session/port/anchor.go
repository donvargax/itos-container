package port

import "context"

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
