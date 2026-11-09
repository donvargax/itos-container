package port

import (
	"context"
	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
)

const (
	CheckoutOverlap       Outcome = "checkout-overlap"
	Usage                 Outcome = "usage"
	Environment           Outcome = "environment-unavailable"
	Internal              Outcome = "internal"
	RegistrationInternal  Outcome = "session-registration-internal"
	RegistrationOwnership Outcome = "session-registration-ownership"
	RegistrationTemporary Outcome = "session-registration-temporary"
)

// PublicRecord contains progress only. Its intent supplies the original authority.
type PublicRecord struct {
	MountDetector string   `json:"mount_detector,omitempty"`
	Schema        int      `json:"schema"`
	ID            string   `json:"id"`
	Project       string   `json:"project"`
	Intent        string   `json:"intent"`
	State         string   `json:"state"`
	WorkloadExit  *int     `json:"workload_exit"`
	LastProblem   *Outcome `json:"last_problem"`
}

type PublicCodec interface {
	EncodeAdmission(AdmissionRecord) []byte
	DecodeAdmission([]byte) (AdmissionRecord, Outcome)
	EncodePublic(PublicRecord) []byte
	DecodePublic([]byte) (PublicRecord, Outcome)
}

type Lease interface{ Close() }

// Registry opens only this effective checkout/user's sessions. Open holds the
// session operation lock; individual intent transactions are short and ordered
// after that lock. A controller lease never blocks Open or read-only discovery.
type Registry interface {
	Register(context.Context, approval.Durable, string, string) (Lease, Outcome)
	List(context.Context) ([]string, Outcome)
	Open(context.Context, string, bool) (Session, Outcome)
}
type Session interface {
	Record() PublicRecord
	Intent() Intent
	Controller() (string, Outcome)
	Validate() Outcome
	// ValidateLaunch also compares the current checkout root and Git identity
	// with the approved binding. Call before create/start, never for recovery.
	ValidateLaunch() Outcome
	Save(PublicRecord) Outcome
	Sync() Outcome
	Close()
}

// RunRegistry retains the already selected original binding for foreground
// cleanup. Public lookup and recovery do not open the mutable checkout;
// Session.ValidateLaunch supplies the separate fresh launch check.
type RunRegistry interface {
	ForRun(approval.Durable) Registry
}

type RecoveryEntry struct {
	MountDetector string   `json:"mount_detector,omitempty"`
	Anchor        string   `json:"anchor,omitempty"`
	ID            string   `json:"id"`
	Project       string   `json:"project"`
	Fingerprint   string   `json:"fingerprint"`
	State         string   `json:"state"`
	Resource      string   `json:"resource"`
	Controller    string   `json:"controller"`
	Cleanup       string   `json:"cleanup"`
	WorkloadExit  *int     `json:"workload_exit"`
	StopCandidate bool     `json:"stop_candidate"`
	Retry         []string `json:"retry"`
}
type RecoveryFailure struct {
	Gaps    []RefusalGap
	Session string
	Outcome Outcome
}
type RecoveryResult struct {
	Entries  []RecoveryEntry
	Failures []RecoveryFailure
}
type Recovery interface {
	Status(context.Context) RecoveryResult
	Stop(context.Context, string) RecoveryResult
}

type RecoveryPresentation interface {
	Print(RecoveryResult, bool) int
}

// AdmissionRecord retains the validated root independently of intent publication.
// Complete follows a durable absent or cancelled intent and stopped session.
// The original seven-field public record remains compatible with old sessions.
type AdmissionRecord struct {
	Schema   int    `json:"schema"`
	Root     string `json:"root"`
	ID       string `json:"id"`
	Intent   string `json:"intent"`
	Complete bool   `json:"complete"`
	// Pending is cleared durably before Register returns launch authority. A
	// pending claim without a public record cannot have sent a create request.
	Pending bool `json:"pending,omitempty"`
}
