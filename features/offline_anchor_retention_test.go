package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

func (w *world) anchorRetentionFixture() error {
	if err := w.anchorFixture(); err != nil {
		return err
	}
	w.anchor.retention, w.anchor.control.anchor.retention = true, true
	// Only the already identity-checked container DELETE receives a 503. The
	// recorder still forwards create/start/wait/inspect and any volume request.
	w.offline.api.failRemove = true
	return nil
}

func (w *world) anchorRetentionAction(child int) error {
	w.anchor.child, w.anchor.control.anchor.child = child, child
	w.anchorExecute("checkout root replaced by real directory")
	control := w.anchor.control
	// The first run deliberately retains resources. Include those exact resources
	// in the control's baseline; its cleanup must leave them alone as well.
	control.anchor.err = errors.Join(control.anchor.err, control.offlineCheckpoint())
	control.anchorExecute("checkout root replaced by real directory")
	return nil
}

// The checkpoint is taken while the actual workload is alive, after snapshots
// have been published, and repeated only after the controller process exits.
func (w *world) anchorSelectedRecords() (map[string]string, error) {
	path := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "snapshots", w.approval.project)
	result, err := doctorTree(path)
	if err != nil {
		return nil, err
	}
	record, err := sessionReadRecord(w.approval.record)
	if err != nil {
		return nil, err
	}
	result[w.approval.record] = string(approvalJSON(record))
	return result, nil
}

func (w *world) anchorRetained() error {
	s := w.anchor
	if s.err != nil {
		return fmt.Errorf("native retention observations unavailable: %w", s.err)
	}
	if !s.reached || len(s.volumes) != 1 || len(s.records) == 0 || len(s.retainedRecords) == 0 {
		return errors.New("missing original owned volume and protected recovery evidence after controller exit")
	}
	if err := w.anchorExecutedOriginal(); err != nil {
		return err
	}
	if err := w.anchorOneCheck("source"); err != nil {
		return err
	}
	for _, v := range s.volumes {
		if v.absent || !anchorVolumeMatches(v, v.retained) {
			return errors.New("original labelled volume did not survive controller exit unchanged")
		}
		if len(s.records) != len(s.retainedRecords) {
			return errors.New("retained anchor lost its original protected record paths")
		}
		for path := range s.records {
			r, exists := s.retainedRecords[path]
			var decoded map[string]any
			if !exists || r.Mode != 0600 || r.UID != w.approval.uid || r.Inode == "" ||
				json.Unmarshal([]byte(r.Bytes), &decoded) != nil ||
				!strings.Contains(r.Bytes, v.name) || !strings.Contains(r.Bytes, w.approval.fingerprint) {
				return errors.New("retained ownership lost protection, original volume or frozen approval binding")
			}
		}
	}
	return nil
}

func (w *world) anchorRetainedWorker() error {
	s, a := w.anchor, w.offline.api
	if len(a.created) != 1 || a.absent[a.created[0]] || a.identity(s.workerAfter, a.created[0]) != nil {
		return errors.New("native Docker did not observe the original full worker identity after controller exit")
	}
	state := object(s.workerAfter["State"])
	if state["Running"] != false || state["Status"] != "exited" || state["ExitCode"] != float64(s.child) {
		return errors.New("retained worker does not have the independently observed workload exit")
	}
	faulted := false
	for _, q := range a.requests {
		if q.Method == "DELETE" && strings.HasPrefix(q.Path, "/volumes/") {
			return errors.New("anchor release was attempted while the owned worker remained")
		}
		if q.Method == "DELETE" && q.Path == "/containers/"+a.created[0] {
			if q.Status != 503 {
				return errors.New("worker removal fault did not remain active")
			}
			faulted = true
		}
	}
	if !faulted {
		return errors.New("retention case never exercised the worker removal fault")
	}
	return nil
}

func (w *world) anchorPublicRecord() (map[string]any, error) {
	root := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", w.recoveryIndexKey())
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	if len(entries) != 1 || !entries[0].IsDir() || !offlineHex(entries[0].Name()) || entries[0].Name() == w.session.token {
		return nil, errors.New("anchor run lacks one independent public session ID")
	}
	id := entries[0].Name()
	if _, err = w.offlineRegistryObjects(id); err != nil {
		return nil, err
	}
	record, err := sessionReadRecord(filepath.Join(root, id, "record.json"))
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if json.Unmarshal([]byte(record.Bytes), &value) != nil || value["id"] != id || value["project"] != w.approval.project || value["intent"] != w.session.reference {
		return nil, errors.New("anchor session registration lost its original identity or intent")
	}
	return value, nil
}

func (w *world) anchorRetentionResult(child, code int) error {
	r, err := w.anchorPublicRecord()
	if err != nil {
		return err
	}
	if child != w.anchor.child || w.offline.exit != code || r["workload_exit"] != float64(child) {
		return fmt.Errorf("anchor run lost workload/result distinction: exit %d, recorded workload %v", w.offline.exit, r["workload_exit"])
	}
	id, _ := r["id"].(string)
	if !strings.Contains(string(w.offline.stderr), "run.cleanup-temporary ") ||
		!strings.Contains(string(w.offline.stderr), "itos container stop "+id) {
		return errors.New("anchor cleanup failure lacks separate guidance and registered session ID")
	}
	observed := false
	for _, q := range w.offline.api.requests {
		if q.Method == "POST" && strings.HasSuffix(q.Path, "/wait") && q.Status == 200 && q.Observation["StatusCode"] == float64(child) {
			observed = true
		}
	}
	if !observed {
		return errors.New("controller never received the expected native workload exit")
	}
	return nil
}

func (w *world) anchorRetentionPreserved() error {
	r, err := w.anchorPublicRecord()
	if err != nil {
		return err
	}
	if (r["state"] != "failed" && r["state"] != "stopping") || r["last_problem"] == nil ||
		strings.Contains(string(w.offline.stderr), "cleanup-complete") {
		return errors.New("retained resources were represented as complete")
	}
	if err := w.anchorPreservedFiles(); err != nil {
		return err
	}
	if len(w.offline.api.created) != 1 || len(w.anchor.volumes) != 1 {
		return errors.New("missing concrete identities for unrelated-resource comparison")
	}
	for name := range w.anchor.volumes {
		return anchorUnrelatedResources(w.offline.resourcesBefore, w.offline.resourcesAfter, w.offline.api.created[0], name)
	}
	return errors.New("missing anchor resource")
}

func (w *world) anchorPreservedFiles() error {
	s := w.anchor
	if err := w.anchorExecutedOriginal(); err != nil {
		return err
	}
	after, err := anchorTree(s.original)
	if err != nil {
		return err
	}
	delete(after, filepath.Join("subdir", "slice-17-edit"))
	if s.originalBefore == nil || !reflect.DeepEqual(s.originalBefore, after) {
		return errors.New("original checkout changed beyond the workload edit")
	}
	after, err = anchorTree(s.replacement)
	if err != nil || !reflect.DeepEqual(s.replacementBefore, after) {
		return errors.New("replacement target changed")
	}
	if len(s.protectedBefore) < 2 || !reflect.DeepEqual(s.protectedBefore, s.protectedAfter) {
		return errors.New("selected approval or snapshots changed during execution or cleanup")
	}
	host, err := doctorTree(w.doctor.host)
	if err != nil {
		return err
	}
	for path, value := range w.offline.before {
		if (path == w.doctor.host || strings.HasPrefix(path, w.doctor.host+string(os.PathSeparator))) && host[path] != value {
			return errors.New("pre-existing unrelated host state changed")
		}
	}
	return nil
}

// Exclude only the independently identified retained worker and volume. Unknown
// additions, missing resources and changes to every other resource remain errors.
func anchorUnrelatedResources(before, after map[string]string, id, volume string) error {
	if before == nil || after == nil {
		return errors.New("missing native resource inventory")
	}
	remaining := maps.Clone(after)
	for path, key := range map[string]string{"/containers/json?all=1": "Id", "/volumes": "Name"} {
		want := id
		if key == "Name" {
			want = volume
		}
		found := 0
		var rows []string
		for _, row := range strings.Split(after[path], "\n") {
			if row == "" {
				continue
			}
			var entry map[string]any
			if json.Unmarshal([]byte(row), &entry) != nil {
				return errors.New("malformed native resource inventory")
			}
			if entry[key] == want {
				found++
			} else {
				rows = append(rows, row)
			}
		}
		if want == "" || found != 1 {
			return errors.New("retained resource missing or ambiguous in native inventory")
		}
		remaining[path] = strings.Join(rows, "\n")
	}
	if !reflect.DeepEqual(before, remaining) {
		return errors.New("anchor retention changed unrelated engine resources or image references")
	}
	return nil
}

func (w *world) anchorRetentionControl() error {
	c := w.anchor.control
	if err := c.anchorControlResult(); err != nil {
		return fmt.Errorf("fault-free native control: %w", err)
	}
	return nil
}

func (w *world) anchorControlResult() error {
	if w.anchor.err != nil {
		return w.anchor.err
	}
	if err := w.anchorPreservedFiles(); err != nil {
		return err
	}
	for _, check := range []string{"source", "reads", "mutations", "cleanup"} {
		if err := w.anchorOneCheck(check); err != nil {
			return err
		}
	}
	r, err := w.anchorPublicRecord()
	if err != nil {
		return err
	}
	if w.offline.exit != w.anchor.child || r["state"] != "stopped" || r["last_problem"] != nil || r["workload_exit"] != float64(w.anchor.child) {
		return errors.New("healthy native control lost workload exit or confirmed completion")
	}
	return nil
}
