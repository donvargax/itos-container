package features

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
)

func (w *world) admissionPath(id string) string {
	key := sha256.Sum256([]byte("itos-container/session-index/v1\x00" + w.approval.uid + "\x00" + w.recoveryIndexKey() + "/" + id))
	return filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "admission", hex.EncodeToString(key[:])+".json")
}

// Admission is specified owned state, not a wildcard exemption from preservation.
// Only the exact certificate bound to this public session may be added or changed.
func (w *world) admissionPaths(tree map[string]runEntry, id, intent string, complete *bool) (map[string]bool, error) {
	path := w.admissionPath(id)
	root := filepath.Dir(path)
	for _, p := range []string{root, path} {
		e, ok := tree[p]
		if !ok || fmt.Sprint(e.Owner) != w.approval.uid {
			return nil, fmt.Errorf("admission lacks its exact owned object")
		}
		if p == root {
			if !e.Mode.IsDir() || e.Mode.Perm() != 0700 {
				return nil, fmt.Errorf("unsafe admission directory")
			}
		} else if !e.Mode.IsRegular() || e.Mode.Perm() != 0600 || fmt.Sprint(e.Links) != "1" {
			return nil, fmt.Errorf("unsafe admission record")
		}
	}
	var claim map[string]any
	if json.Unmarshal([]byte(tree[path].Data), &claim) != nil || len(claim) != 5 || claim["schema"] != float64(1) || claim["root"] != w.doctor.checkout || claim["id"] != id || claim["intent"] != intent {
		return nil, fmt.Errorf("admission lost its exact checkout/session/intent binding")
	}
	done, ok := claim["complete"].(bool)
	if !ok || (complete != nil && done != *complete) {
		return nil, fmt.Errorf("admission lost its cleanup result")
	}
	return map[string]bool{root: true, path: true}, nil
}

// Recovery may encounter legacy registrations without admission metadata. Any
// certificate that is present must still bind to one of this checkout's sessions.
func (w *world) currentAdmissionPaths(tree map[string]runEntry) (map[string]bool, error) {
	allowed := map[string]bool{}
	project := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", w.recoveryIndexKey())
	for path, entry := range tree {
		if filepath.Dir(path) != project || !entry.Mode.IsDir() {
			continue
		}
		id := filepath.Base(path)
		if _, exists := tree[w.admissionPath(id)]; !exists {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(tree[filepath.Join(path, "record.json")].Data), &record); err != nil {
			return nil, err
		}
		intent, ok := record["intent"].(string)
		if !ok || !publicRecordShape(record) || record["schema"] != float64(1) || record["id"] != id || record["project"] != w.approval.project {
			return nil, fmt.Errorf("admission has no original public authority")
		}
		var completion *bool
		// A stopped session may retain the independently reported output
		// timeout after resource cleanup and its certificate are complete.
		if record["state"] != "stopped" || (record["last_problem"] != nil && record["last_problem"] != "stream-timeout") {
			incomplete := false
			completion = &incomplete
		}
		admission, err := w.admissionPaths(tree, id, intent, completion)
		if err != nil {
			return nil, err
		}
		maps.Copy(allowed, admission)
	}
	return allowed, nil
}
