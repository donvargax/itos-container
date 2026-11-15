package domain

import (
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Preflight composes local refusal, fresh inspection and current approval without
// granting consent. Freeze still atomically checks the inspected generation.
type Preflight struct {
	Service
	Local     port.StagedReadiness
	Canonical port.PreflightCodec
}

func (s Preflight) Inspect() port.View {
	v := port.View{Status: "missing"}
	uid, gid := s.Host.Identity()
	if v.Failure = IdentityFailure(uid, gid); v.Failure != "" {
		return v
	}
	localMatched := false
	v.Inspection = s.Local.InspectBeforeEngine(func(r readiness.Result) bool {
		v.Field = Mismatch(r, uid)
		if v.Field != "" {
			v.Failure = "identity.mismatch"
			return false
		}
		c := s.Canonical.LocalCandidate(r, uid, gid)
		v.Record, v.Failure = s.Store.Read(c)
		if v.Failure != "" {
			v.Status = "invalid"
			return false
		}
		if v.Record.Generation == "" {
			return false
		}
		v.Status = "changed"
		if !s.Canonical.LocalMatches(c, v.Record) {
			v.ChangedPaths = s.Canonical.ChangedPaths(c, v.Record, true)
			return false
		}
		localMatched = true
		return true
	})
	if !localMatched || !v.Inspection.Ready {
		return v
	}
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
	v.Status = "changed"
	if v.Record.Canonical == v.Candidate.Canonical {
		v.Status = "matching"
	} else {
		v.ChangedPaths = s.Canonical.ChangedPaths(v.Candidate, v.Record, false)
	}
	return v
}
