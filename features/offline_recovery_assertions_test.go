package features

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func (w *world) offlineRegisteredRetry() error {
	s, r := w.session, w.session.create
	if !offlineHex(r.SessionID) || r.SessionID == s.token {
		return fmt.Errorf("cleanup did not retain an independent registered session ID")
	}
	retries := regexp.MustCompile(`itos container stop (\S+)([^\r\n]*)`).FindAllStringSubmatch(r.Diagnostic, -1)
	if len(retries) == 0 || !strings.Contains(r.Diagnostic, "same effective project") {
		return fmt.Errorf("cleanup diagnostic lacks an executable same-project retry")
	}
	for _, retry := range retries {
		if retry[1] != r.SessionID || strings.TrimSpace(retry[2]) != "" {
			return fmt.Errorf("cleanup retry does not target the registered session")
		}
	}
	_, err := w.offlinePublicRecord()
	return err
}

func (w *world) offlinePublicRecord() ([]byte, error) {
	s, r := w.session, w.session.create
	if !offlineHex(r.SessionID) || r.SessionID == s.token {
		return nil, fmt.Errorf("cleanup did not retain an independent registered session ID")
	}
	data := r.PublicRecords[r.SessionID]
	if s.backend == "disk" {
		path := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", w.recoveryIndexKey(), r.SessionID, "record.json")
		var err error
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("retry has no protected registration: %w", err)
		}
	} else if len(r.PublicRecords) != 1 {
		return nil, fmt.Errorf("retry must bind exactly one public registration")
	}
	var record map[string]any
	if json.Unmarshal(data, &record) != nil || !publicRecordShape(record) || record["schema"] != float64(1) || record["id"] != r.SessionID || record["project"] != w.approval.project || record["intent"] != s.reference {
		return nil, fmt.Errorf("retry registration lost the original project or intent")
	}
	if record["workload_exit"] != nil && record["workload_exit"] != float64(s.launch.child) {
		return nil, fmt.Errorf("registration changed the workload exit")
	}
	if r.Cleanup == "cleanup-complete" {
		var expectedExit any = float64(s.launch.child)
		if s.anchorFixture && r.Workload == nil {
			expectedExit = nil
		}
		if record["state"] != "stopped" || record["workload_exit"] != expectedExit || record["last_problem"] != nil {
			return nil, fmt.Errorf("completed registration lost its workload and cleanup facts")
		}
	} else if record["state"] != "running" && record["state"] != "stopping" && record["state"] != "failed" {
		return nil, fmt.Errorf("incomplete cleanup fabricated a stopped registration")
	}
	return data, nil
}

// Accept only the exact new protected registration and empty lock objects. Every
// pre-existing path and every other new path remains in the preservation check.
func (w *world) offlineRegistryPaths() (map[string]bool, error) {
	if _, err := w.offlinePublicRecord(); err != nil {
		return nil, err
	}
	if w.session.backend != "disk" {
		return map[string]bool{}, nil
	}
	return w.offlineRegistryObjects(w.session.create.SessionID)
}

func (w *world) offlineCompletedRegistryPaths() (map[string]bool, error) {
	root := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", w.recoveryIndexKey())
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	if len(entries) != 1 || !entries[0].IsDir() || !offlineHex(entries[0].Name()) || entries[0].Name() == w.session.token {
		return nil, fmt.Errorf("native run did not retain exactly one independent public registration")
	}
	id := entries[0].Name()
	allowed, err := w.offlineRegistryObjects(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(root, id, "record.json"))
	if err != nil {
		return nil, err
	}
	want := map[string]any{"schema": 1, "id": id, "project": w.approval.project, "intent": w.session.reference,
		"state": "stopped", "workload_exit": 0, "last_problem": nil}
	if !completedDetector(string(data), want) {
		return nil, fmt.Errorf("native run lost its exact completed registration or original binding")
	}
	return allowed, nil
}

func (w *world) offlineRegistryObjects(id string) (map[string]bool, error) {
	allowed := map[string]bool{}
	root := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions")
	project := filepath.Join(root, w.recoveryIndexKey())
	dir := filepath.Join(project, id)
	tree, err := runTree(filepath.Dir(root))
	if err != nil {
		return nil, err
	}
	for _, path := range []string{root, project, dir, filepath.Join(dir, "record.json"), filepath.Join(dir, "controller.lock"), filepath.Join(dir, "operation.lock")} {
		e, ok := tree[path]
		if !ok || fmt.Sprint(e.Owner) != w.approval.uid {
			return nil, fmt.Errorf("registration lacks its owned protected object")
		}
		if path == root || path == project || path == dir {
			if !e.Mode.IsDir() || e.Mode.Perm() != 0700 {
				return nil, fmt.Errorf("unsafe registration directory")
			}
		} else if !e.Mode.IsRegular() || e.Mode.Perm() != 0600 || fmt.Sprint(e.Links) != "1" {
			return nil, fmt.Errorf("unsafe registration record or lock")
		}
		if strings.HasSuffix(path, ".lock") && e.Data != "" {
			return nil, fmt.Errorf("registration lock contains workload data")
		}
		allowed[path] = true
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(tree[filepath.Join(dir, "record.json")].Data), &record); err != nil {
		return nil, err
	}
	intent, ok := record["intent"].(string)
	if !ok {
		return nil, fmt.Errorf("registration has no original intent")
	}
	complete := record["state"] == "stopped" && record["last_problem"] == nil
	admission, err := w.admissionPaths(tree, id, intent, &complete)
	if err != nil {
		return nil, err
	}
	maps.Copy(allowed, admission)
	return allowed, nil
}
