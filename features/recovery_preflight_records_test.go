package features

import (
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
)

// Slice-11's required public registration extends the earlier preflight-only
// allowlist. Every additional path is independently bound to one created worker;
// no arbitrary file, unsafe mode, workload payload or unrelated state is allowed.
func (w *world) localRegistryPaths(tree map[string]runEntry) (map[string]bool, error) {
	allowed := map[string]bool{}
	if w.doctor.engine.offline == nil {
		return allowed, nil
	}
	a := w.doctor.engine.offline
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.created) == 0 {
		return allowed, nil
	}
	root := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions")
	project := filepath.Join(root, w.recoveryIndexKey())
	allowed[root], allowed[project] = true, true
	intents := map[string]bool{}
	for _, r := range a.created {
		token, _ := object(object(r["Config"])["Labels"])["io.itos-container.intent"].(string)
		intents["intents/"+w.approval.project+"/"+token+".json"] = false
	}
	count := 0
	for path, entry := range tree {
		if filepath.Dir(path) != project {
			continue
		}
		id := filepath.Base(path)
		if !offlineHex(id) || !entry.Mode.IsDir() {
			return nil, fmt.Errorf("unexpected public registration path")
		}
		count++
		recordPath := filepath.Join(path, "record.json")
		var r map[string]any
		if json.Unmarshal([]byte(tree[recordPath].Data), &r) != nil || len(r) != 7 || r["schema"] != float64(1) || r["id"] != id || r["project"] != w.approval.project || r["state"] != "stopped" || r["workload_exit"] != float64(0) || r["last_problem"] != nil {
			return nil, fmt.Errorf("public registration lost exact completed facts")
		}
		intent, ok := r["intent"].(string)
		seen, exists := intents[intent]
		if !ok || !exists || seen {
			return nil, fmt.Errorf("public registration did not uniquely bind an owned intent")
		}
		intents[intent] = true
		complete := true
		admission, err := w.admissionPaths(tree, id, intent, &complete)
		if err != nil {
			return nil, err
		}
		maps.Copy(allowed, admission)
		for _, p := range []string{path, recordPath, filepath.Join(path, "controller.lock"), filepath.Join(path, "operation.lock")} {
			allowed[p] = true
		}
	}
	if count != len(intents) {
		return nil, fmt.Errorf("created worker lacked exactly one public registration")
	}
	for path := range allowed {
		entry, ok := tree[path]
		if !ok {
			return nil, fmt.Errorf("missing public registry object")
		}
		directory := path == root || path == project || filepath.Dir(path) == project || path == filepath.Dir(w.admissionPath(""))
		if fmt.Sprint(entry.Owner) != w.approval.uid {
			return nil, fmt.Errorf("public registry owner changed")
		}
		if directory {
			if !entry.Mode.IsDir() || entry.Mode.Perm() != 0700 {
				return nil, fmt.Errorf("unsafe public directory")
			}
		} else {
			if !entry.Mode.IsRegular() || entry.Mode.Perm() != 0600 || fmt.Sprint(entry.Links) != "1" {
				return nil, fmt.Errorf("unsafe public record or lock")
			}
			if strings.HasSuffix(path, ".lock") && entry.Data != "" {
				return nil, fmt.Errorf("lock contains workload data")
			}
		}
	}
	return allowed, nil
}
