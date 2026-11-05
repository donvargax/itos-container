// Package domain owns approval resolution and selection decisions.
package domain

import (
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"strconv"
	"strings"
)

// Resolve binds a validated inspection to effective IDs, overriding image users.
func Resolve(r readiness.Result, uid, gid string, codec port.Codec) (port.Candidate, port.Failure) {
	if failure := IdentityFailure(uid, gid); failure != "" {
		return port.Candidate{}, failure
	}
	if !r.Ready {
		return port.Candidate{}, "prerequisite"
	}
	if Mismatch(r, uid) != "" {
		return port.Candidate{}, "identity.mismatch"
	}
	return codec.Candidate(r, uid, gid), ""
}

// IdentityFailure validates effective host IDs without engine or filesystem access.
func IdentityFailure(uid, gid string) port.Failure {
	if !identity(uid) || !identity(gid) {
		return "identity.unavailable"
	}
	if uid == "0" {
		return "identity.root"
	}
	return ""
}
func identity(id string) bool {
	n, err := strconv.ParseUint(id, 10, 32)
	return err == nil && strconv.FormatUint(n, 10) == id
}

// Mismatch identifies a requested user that differs from the effective UID.
func Mismatch(r readiness.Result, uid string) string {
	if r.Manifest != nil && r.Manifest.Effective != nil {
		m := r.Manifest.Effective
		if m.ContainerUser != nil && *m.ContainerUser != uid {
			return "containerUser"
		}
		if m.RemoteUser != nil && *m.RemoteUser != uid {
			return "remoteUser"
		}
	}
	return ""
}

// Publish preserves the reviewed generation during consented publication.
func Publish(store port.Store, c port.Candidate, generation string) (port.Record, port.Failure) {
	return store.Commit(c, generation)
}

// Select returns only the exact current approval as independent frozen input.
func Select(store port.Store, c port.Candidate, generation string) (port.Frozen, port.Failure) {
	frozen, failure := store.Select(c, generation)
	if failure == "state.invalid" {
		failure = "invalid"
	}
	return frozen, failure
}

// Service coordinates validated input and host-owned state; it never prompts.
type Service struct {
	Readiness port.Readiness
	Host      port.Host
	Codec     port.Codec
	Store     port.Store
}

// Inspect is read-only, including when host state is absent.
func (s Service) Inspect() port.View {
	v := port.View{Inspection: s.Readiness.Inspect(), Status: "missing"}
	if !v.Inspection.Ready {
		return v
	}
	uid, gid := s.Host.Identity()
	v.Candidate, v.Failure = Resolve(v.Inspection, uid, gid, s.Codec)
	if v.Failure != "" {
		v.Field = Mismatch(v.Inspection, uid)
		return v
	}
	v.Socket = s.Host.Socket(v.Inspection.Engine.Endpoint)
	if v.Socket == "" {
		v.Failure = "identity.unavailable"
		return v
	}
	v.Record, v.Failure = s.Store.Read(v.Candidate)
	if v.Failure != "" {
		v.Status = "invalid"
		return v
	}
	if v.Record.Generation != "" {
		v.Status = "changed"
		if v.Record.Canonical == v.Candidate.Canonical {
			v.Status = "matching"
		}
	}
	return v
}

// Approve re-inspects after consent, then compares and publishes under the store
// lock. No lock or writable state exists while a person is reading the review.
func (s Service) Approve(review port.View) port.View {
	fresh := s.Inspect()
	if fresh.Candidate.Canonical != review.Candidate.Canonical || fresh.Socket != review.Socket || !fresh.Inspection.Ready {
		review.Failure = "approve.changed"
		return review
	}
	if fresh.Failure != "" {
		return fresh
	}
	record, failure := Publish(s.Store, review.Candidate, review.Record.Generation)
	if failure != "" {
		review.Failure = failure
		if strings.HasPrefix(string(failure), "state.") {
			review.Status = "invalid"
		}
		return review
	}
	fresh.Record = record
	fresh.Status = "matching"
	return fresh
}

// ValidFingerprint refuses noncanonical spelling before consent is considered.
func ValidFingerprint(value string) bool {
	digest, ok := strings.CutPrefix(value, "sha256:")
	return ok && len(digest) == 64 && strings.Trim(digest, "0123456789abcdef") == ""
}

// Freeze is the run-facing selection of the inspected active generation.
func (s Service) Freeze(view port.View) (port.Frozen, port.Failure) {
	return Select(s.Store, view.Candidate, view.Record.Generation)
}
