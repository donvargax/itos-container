package features

import (
	"encoding/json"
	"fmt"
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
		path := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", w.approval.project, r.SessionID, "record.json")
		var err error
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("retry has no protected registration: %w", err)
		}
	} else if len(r.PublicRecords) != 1 {
		return nil, fmt.Errorf("retry must bind exactly one public registration")
	}
	var record map[string]any
	if json.Unmarshal(data, &record) != nil || len(record) != 7 || record["schema"] != float64(1) || record["id"] != r.SessionID || record["project"] != w.approval.project || record["intent"] != s.reference {
		return nil, fmt.Errorf("retry registration lost the original project or intent")
	}
	if record["workload_exit"] != nil && record["workload_exit"] != float64(s.launch.child) {
		return nil, fmt.Errorf("registration changed the workload exit")
	}
	if r.Cleanup == "cleanup-complete" {
		if record["state"] != "stopped" || record["workload_exit"] != float64(s.launch.child) || record["last_problem"] != nil {
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
	allowed := map[string]bool{}
	if w.session.backend != "disk" {
		return allowed, nil
	}
	root := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions")
	project := filepath.Join(root, w.approval.project)
	dir := filepath.Join(project, w.session.create.SessionID)
	tree, err := runTree(root)
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
	return allowed, nil
}
