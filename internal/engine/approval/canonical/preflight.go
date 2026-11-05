package canonical

import (
	"bytes"
	"encoding/json"
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// LocalCandidate uses the same canonicalizer, with explicitly unobserved engine
// and cached-image identities. Its bytes are never accepted as a run candidate.
func (j JSON) LocalCandidate(r readiness.Result, uid, gid string) port.Candidate {
	r.Engine = &readiness.EngineIdentity{}
	r.Image = &readiness.ImageIdentity{}
	return j.Candidate(r, uid, gid)
}

// LocalMatches compares validated effective request values, not request spelling.
func (JSON) LocalMatches(c port.Candidate, r port.Record) bool {
	var requested, approved settings
	if json.Unmarshal([]byte(c.Canonical), &requested) != nil || json.Unmarshal([]byte(r.Canonical), &approved) != nil {
		return false
	}
	requested.Engine, approved.Engine = engine{}, engine{}
	requested.Image, approved.Image = image{}, image{}
	return bytes.Equal(canonical(requested), canonical(approved))
}

// FrozenRecord consumes only selected values and reuses all envelope validation.
func (j JSON) FrozenRecord(f port.Frozen) (port.Record, port.Failure) {
	if len(f.Settings) > 1024*1024 || !bounded([]byte(f.Settings)) {
		return port.Record{}, "state.invalid"
	}
	var s settings
	if json.Unmarshal([]byte(f.Settings), &s) != nil {
		return port.Record{}, "state.invalid"
	}
	c := candidate(s)
	if c.Canonical != f.Settings || c.Project != f.Project || c.Fingerprint != f.Fingerprint {
		return port.Record{}, "state.invalid"
	}
	r := port.Record{Candidate: c, Generation: f.Generation}
	return j.Decode(c, j.Encode(r))
}
