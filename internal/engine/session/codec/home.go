package codec

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func (JSON) Scope(b port.Binding) string {
	data, _ := json.Marshal(map[string]any{"schema": 1, "project": b.Frozen.Project, "uid": b.UID, "gid": b.GID,
		"engine": map[string]string{"endpoint": b.Endpoint, "id": b.EngineID}})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func (JSON) EncodeHome(s port.HomeScope) []byte { data, _ := json.Marshal(s); return data }
func (j JSON) DecodeHome(data []byte) (port.HomeScope, port.Outcome) {
	var s port.HomeScope
	if len(data) == 0 || len(data) > 1024*1024 || json.Unmarshal(data, &s) != nil || !bytes.Equal(data, j.EncodeHome(s)) {
		return s, port.Invalid
	}
	u, ue := strconv.ParseUint(s.UID, 10, 32)
	g, ge := strconv.ParseUint(s.GID, 10, 32)
	b := s.Binding()
	b.Frozen.Project = s.Project
	if s.Schema != 1 || !port.HexID(s.Project) || s.Scope != j.Scope(b) || s.Engine.Endpoint == "" || s.Engine.ID == "" ||
		ue != nil || ge != nil || u == 0 || strconv.FormatUint(u, 10) != s.UID || strconv.FormatUint(g, 10) != s.GID || !port.HexID(s.Current) || len(s.Generations) == 0 {
		return s, port.Invalid
	}
	seen := map[string]bool{}
	for _, generation := range s.Generations {
		if !generation.Reference(s.Scope).Valid() || seen[generation.ID] || !validHomeGeneration(generation, s.Project) {
			return s, port.Invalid
		}
		seen[generation.ID] = true
	}
	if s.Generations[len(s.Generations)-1].ID != s.Current {
		return s, port.Invalid
	}
	for i, generation := range s.Generations {
		if i < len(s.Generations)-1 {
			if generation.Stage != "reset" || generation.Successor != s.Generations[i+1].ID {
				return s, port.Invalid
			}
		} else if generation.Stage == "reset" || generation.Successor != "" && seen[generation.Successor] {
			return s, port.Invalid
		}
	}
	return s, ""
}
func validHomeGeneration(g port.HomeGeneration, project string) bool {
	switch g.Stage {
	case "awaiting-allocation", "intent":
		if g.Created != "" || g.Initialized || g.Successor != "" {
			return false
		}
	case "retained":
		if g.Created == "" || g.Successor != "" {
			return false
		}
	case "reset-pending", "reset":
		if g.Created == "" || !port.HexID(g.Successor) || g.Successor == g.ID || len(g.Claims) != 0 {
			return false
		}
	case "delete-pending", "deleted":
		if g.Created == "" || g.Successor != "" || len(g.Claims) != 0 {
			return false
		}
	default:
		return false
	}
	seen := map[string]bool{}
	for _, claim := range g.Claims {
		p, _, ok := port.ParseReference(claim.Intent)
		if !ok || p != project || !port.HexID(claim.Session) || claim.StateRoot == "" || seen[claim.Session] {
			return false
		}
		seen[claim.Session] = true
	}
	return true
}
