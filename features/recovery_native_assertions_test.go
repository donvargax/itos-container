package features

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type recoveryReport struct {
	Schema   int               `json:"schema"`
	OK       bool              `json:"ok"`
	Problems []recoveryProblem `json:"problems"`
	Sessions []recoveryEntry   `json:"sessions"`
}
type recoveryProblem struct {
	Rule, Message, Fix, Session string
	Exit                        int
}
type recoveryEntry struct {
	ID, Project, Fingerprint, State, Resource, Controller, Cleanup string
	WorkloadExit                                                   *int     `json:"workload_exit"`
	StopCandidate                                                  bool     `json:"stop_candidate"`
	Retry                                                          []string `json:"retry"`
}

func recoveryReadReport(p *recoveryProcess) (recoveryReport, error) {
	var report recoveryReport
	if p == nil {
		return report, errors.New("recovery command did not run")
	}
	p.wait()
	if p.err != nil {
		return report, p.err
	}
	if p.exit != 0 {
		return report, fmt.Errorf("recovery command exit = %d, expected zero", p.exit)
	}
	decoder := json.NewDecoder(bytes.NewReader(p.out.bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return report, fmt.Errorf("recovery command did not emit its schema-1 result: %w", err)
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || report.Schema != 1 || !report.OK || report.Problems == nil || len(report.Problems) != 0 || report.Sessions == nil {
		return report, errors.New("recovery command did not emit exactly one complete successful schema-1 object")
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(p.out.bytes(), &raw) != nil || !recoveryExactFields(raw, "schema", "ok", "problems", "sessions") {
		return report, errors.New("recovery result has missing or incorrectly named fields")
	}
	var entries []map[string]json.RawMessage
	if json.Unmarshal(raw["sessions"], &entries) != nil {
		return report, errors.New("recovery sessions are not objects")
	}
	for _, entry := range entries {
		if !recoveryExactFields(entry, "id", "project", "fingerprint", "state", "resource", "controller", "cleanup", "workload_exit", "stop_candidate", "retry") {
			return report, errors.New("recovery session has missing or incorrectly named fields")
		}
	}
	return report, nil
}

func recoveryExactFields(object map[string]json.RawMessage, fields ...string) bool {
	if len(object) != len(fields) {
		return false
	}
	for _, field := range fields {
		if _, ok := object[field]; !ok {
			return false
		}
	}
	return true
}

func (w *world) recoveryEntry(p *recoveryProcess, state, resource, cleanup string) (recoveryEntry, error) {
	report, err := recoveryReadReport(p)
	if err != nil {
		return recoveryEntry{}, err
	}
	r := w.recovery
	if len(report.Sessions) != 1 {
		return recoveryEntry{}, fmt.Errorf("project recovery listed %d sessions, expected exactly its own session", len(report.Sessions))
	}
	s := report.Sessions[0]
	if !offlineHex(r.id) || s.ID != r.id || s.Project != w.approval.project || s.Fingerprint != w.approval.fingerprint ||
		s.State != state || s.Resource != resource || s.Cleanup != cleanup {
		return s, errors.New("recovery did not report the registered session with its original binding and observed state")
	}
	return s, nil
}

func (w *world) recoveryRetrySucceeded() error {
	r := w.recovery
	if r.err != nil {
		return r.err
	}
	if r.run == nil {
		return errors.New("foreground recovery workload did not run")
	}
	r.run.wait()
	if r.run.err != nil {
		return r.run.err
	}
	if r.run.exit != 7 || !r.retained || !strings.Contains(string(r.run.errout.bytes()), "run.cleanup-temporary") ||
		!strings.Contains(string(r.run.out.bytes()), "slice-11 deliberate workload marker\n") ||
		!reflect.DeepEqual(r.retry, []string{"stop", r.id}) || !offlineHex(r.id) {
		return errors.New("run did not preserve exit 7 with its registered session ID and executable stop retry after failed cleanup")
	}
	if len(r.stops) != 1 || r.stops[0].exit != 0 || r.stops[0].err != nil {
		return errors.New("the exact printed stop argv did not complete successfully")
	}
	a := w.offline.api
	a.mu.Lock()
	waited, failedRemoval := false, false
	for _, request := range a.requests {
		if request.Method == "POST" && request.Path == "/containers/"+r.worker+"/wait" && request.Status == 200 && request.Observation["StatusCode"] == float64(7) {
			waited = true
		}
		if waited && request.Method == "DELETE" && request.Path == "/containers/"+r.worker && request.Status == 503 {
			failedRemoval = true
		}
	}
	a.mu.Unlock()
	if !failedRemoval {
		return errors.New("transient removal failure did not follow the native observed workload exit 7")
	}
	return w.recoveryCompleted()
}

func (w *world) recoveryLostController() error {
	r := w.recovery
	if r.err != nil {
		return r.err
	}
	s, err := w.recoveryEntry(r.status, "running", "running", "pending")
	if err != nil {
		return err
	}
	if s.Controller != "absent" || !s.StopCandidate || s.WorkloadExit != nil ||
		!reflect.DeepEqual(s.Retry, []string{"itos", "container", "stop", r.id}) {
		return errors.New("status did not identify controller loss with an unknown workload exit and exact session retry")
	}
	if r.run.exit != 128+9 {
		return fmt.Errorf("the loss barrier did not kill only the recorded controller (exit %d)", r.run.exit)
	}
	return nil
}

func (w *world) recoveryHeartbeatAdvanced() error {
	r := w.recovery
	if r.heartbeat <= 0 || r.laterHeartbeat <= r.heartbeat {
		return errors.New("independent native observation found no new heartbeat after the controller loss or other project's stop")
	}
	return nil
}

func (w *world) recoveryConcurrentStops() error {
	r := w.recovery
	if r.err != nil {
		return r.err
	}
	if !offlineHex(r.id) || len(r.stops) != 2 {
		return errors.New("two independent stop processes could not target a registered session")
	}
	for _, stop := range r.stops {
		s, err := w.recoveryEntry(stop, "stopped", "absent", "complete")
		if err != nil {
			return err
		}
		if s.Retry != nil || s.StopCandidate {
			return errors.New("completed stop still advertises pending cleanup")
		}
	}
	if !r.stops[0].finished.After(r.stops[1].started) {
		return errors.New("fixture failed to overlap its two independent stop invocations")
	}
	if err := w.recoveryCompleted(); err != nil {
		return err
	}
	if !r.second.recoveryNativePresent(true) {
		return errors.New("stopping project A changed project B's running worker")
	}
	return r.second.recoveryHeartbeatAdvanced()
}

func (w *world) recoveryCompleted() error {
	r := w.recovery
	if !offlineHex(r.worker) || w.doctor.real.read("/containers/"+r.worker+"/json", nil, 404) != nil {
		return errors.New("independent observer did not confirm the original full worker ID absent before harness cleanup")
	}
	s, err := w.recoveryEntry(r.after, "stopped", "absent", "complete")
	if err != nil {
		return err
	}
	if s.Retry != nil || s.StopCandidate || (r.mode == "retry" && (s.WorkloadExit == nil || *s.WorkloadExit != 7)) ||
		(r.mode == "loss" && s.WorkloadExit != nil && *s.WorkloadExit == 0) {
		return errors.New("cleanup success replaced workload knowledge or retained a pending retry")
	}
	if r.mode != "retry" {
		if len(r.stops) == 0 {
			return errors.New("no product stop completed")
		}
		for _, stop := range r.stops {
			if stop.err != nil || stop.exit != 0 {
				return errors.New("product stop did not return zero for confirmed cleanup")
			}
		}
	}
	a := w.offline.api
	a.mu.Lock()
	removed, confirmed := false, false
	for _, request := range a.requests {
		if request.Method == "DELETE" && request.Path == "/containers/"+r.worker && request.Status == 204 {
			removed = true
		}
		if removed && request.Method == "GET" && request.Path == "/containers/"+r.worker+"/json" && request.Status == 404 {
			confirmed = true
		}
	}
	a.mu.Unlock()
	if !removed || !confirmed {
		return errors.New("product did not remove its full ID and freshly confirm absence")
	}
	state := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")
	intent := w.sessionExpectedRecord("absent", r.worker)
	if err := recoveryDurableFile(filepath.Join(state, filepath.FromSlash(w.session.reference)), intent, r.stops); err != nil {
		return err
	}
	var exit any
	if s.WorkloadExit != nil {
		exit = *s.WorkloadExit
	}
	record := map[string]any{"schema": 1, "id": r.id, "project": w.approval.project, "intent": w.session.reference,
		"state": "stopped", "workload_exit": exit, "last_problem": nil}
	return recoveryDurableFile(filepath.Join(state, "sessions", w.recoveryIndexKey(), r.id, "record.json"), record, r.stops)
}

func recoveryDurableFile(path string, want map[string]any, processes []*recoveryProcess) error {
	final, err := sessionReadRecord(path)
	if err != nil {
		return err
	}
	if final.Mode != 0600 || !recoveryJSONEqual(final.Bytes, want) {
		return errors.New("cleanup did not retain its exact protected completion record")
	}
	for _, process := range processes {
		events, err := recoveryReadSyncs(process.journal)
		if err != nil {
			return err
		}
		fileSynced := false
		for _, event := range events {
			if !event.Directory && event.Inode == final.Inode && recoveryJSONEqual(event.Data, want) {
				fileSynced = true
			}
			if fileSynced && event.Directory && event.Path == filepath.Dir(path) && event.Files[filepath.Base(path)] == final.Inode {
				return nil
			}
		}
	}
	return errors.New("no successful file fsync followed by publication-directory fsync proved the visible completion inode durable")
}

func (w *world) recoveryPreserved() error {
	r := w.recovery
	current, err := w.recoveryProtected()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(r.before, current) {
		return errors.New("recovery changed unrelated checkout edits, approval or host state")
	}
	edit, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-11-worker-edit"))
	if err != nil || string(edit) != "slice-11 retained edit" {
		return errors.New("recovery lost the worker's checkout edit")
	}
	snapshotPath := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "snapshots", w.approval.project, "1.json")
	snapshot, err := sessionReadRecord(snapshotPath)
	if err != nil {
		return err
	}
	want := map[string]any{"schema": 1, "project": w.approval.project, "generation": "1", "fingerprint": w.approval.fingerprint, "settings": w.approval.settings}
	if snapshot != r.snapshot || !recoveryJSONEqual(snapshot.Bytes, want) {
		return errors.New("recovery changed the original protected snapshot")
	}
	resources, err := w.doctor.real.snapshotResources()
	if err != nil {
		return err
	}
	owned := []string{r.worker}
	if r.second != nil {
		owned = append(owned, r.second.recovery.worker)
	}
	if !reflect.DeepEqual(recoveryWithoutWorkers(resources, owned), recoveryWithoutWorkers(w.offline.resourcesBefore, owned)) {
		return errors.New("recovery changed unrelated native containers, images, networks or volumes")
	}
	return nil
}

func recoveryWithoutWorkers(resources map[string]string, ids []string) map[string]string {
	result := make(map[string]string, len(resources))
	for path, value := range resources {
		if path != "/containers/json?all=1" {
			result[path] = value
			continue
		}
		var retained []string
		for _, line := range strings.Split(value, "\n") {
			var entry map[string]any
			_ = json.Unmarshal([]byte(line), &entry)
			owned := false
			for _, id := range ids {
				owned = owned || (id != "" && entry["Id"] == id)
			}
			if !owned {
				retained = append(retained, line)
			}
		}
		result[path] = strings.Join(retained, "\n")
	}
	return result
}

func (w *world) recoveryOneLaunch() error {
	a, r := w.offline.api, w.recovery
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.problem != nil {
		return a.problem
	}
	creates, starts, removes := 0, 0, 0
	for _, request := range a.requests {
		transient := r.mode == "retry" && request.Method == "DELETE" && request.Status == 503
		cancelled := request.Cancelled && request.Method == "POST" && (strings.HasSuffix(request.Path, "/wait") || strings.HasSuffix(request.Path, "/attach"))
		if request.Status >= 400 && request.Status != 404 && !transient && !cancelled {
			return errors.New("recorder refused or failed a product request outside the configured recovery fault")
		}
		if request.Method == "POST" && request.Path == "/containers/create" {
			creates++
		}
		if request.Method == "POST" && strings.HasSuffix(request.Path, "/start") {
			starts++
		}
		if request.Method == "DELETE" {
			if request.Path != "/containers/"+r.worker {
				return errors.New("recovery attempted to remove a different native resource")
			}
			if request.Status == 204 {
				removes++
			}
		}
	}
	if creates != 1 || starts != 1 || removes != 1 || len(a.created) != 1 {
		return fmt.Errorf("product create/start/successful-remove totals = %d/%d/%d, expected 1/1/1", creates, starts, removes)
	}
	return nil
}
