package features

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

func isolationRecord(run *isolationRun, stage string) error {
	actual, err := sessionDecodeRecord(run.record.Bytes)
	if err != nil {
		return err
	}
	expected := approvalClone(run.expected)
	expected["stage"] = stage
	if run.record.Mode != 0600 || run.record.UID != expected["uid"] || !reflect.DeepEqual(actual, expected) {
		return errors.New("protected record lacks the exact original ownership and completion state")
	}
	return nil
}

func (s *isolationWorld) completed(run *isolationRun, status int) error {
	if run.err != nil {
		return run.err
	}
	if run.exit != status || !offlineHex(run.id) {
		return fmt.Errorf("wrapper exit %d differs from required observed workload exit %d, or no native worker ran", run.exit, status)
	}
	waited, removed, confirmed := false, false, false
	creates, attaches, starts := 0, 0, 0
	for _, r := range run.requests {
		if r.Status >= 400 && r.Status != 404 {
			return fmt.Errorf("native recorder request failed or was refused (%d)", r.Status)
		}
		if r.Method == "POST" && r.Path == "/containers/create" {
			creates++
		}
		if r.Method == "POST" && r.Path == "/containers/"+run.id+"/attach" && r.Status == 101 {
			attaches++
		}
		if r.Method == "POST" && r.Path == "/containers/"+run.id+"/start" && r.Status == 204 {
			starts++
		}
		if r.Method == "POST" && r.Path == "/containers/"+run.id+"/wait" && r.Status == 200 && r.Observation["StatusCode"] == float64(status) {
			waited = true
		}
		if r.Method == "DELETE" && r.Path == "/containers/"+run.id && r.Status == 204 {
			if !waited {
				return errors.New("removal preceded known workload exit")
			}
			removed = true
		}
		if removed && r.Method == "GET" && r.Path == "/containers/"+run.id+"/json" && r.Status == 404 {
			confirmed = true
		}
	}
	if creates != 1 || attaches != 1 || starts != 1 || !waited || !removed || !confirmed || !run.absent {
		return errors.New("result lacks actual launch, workload exit, removal or full-ID absence before harness cleanup")
	}
	return isolationRecord(run, "absent")
}

func (s *isolationWorld) completeAll() error {
	if s.controlError != nil {
		return s.controlError
	}
	if len(s.runs) == 0 {
		return errors.New("no native workload ran")
	}
	for _, r := range s.runs {
		if err := s.completed(r, r.config.Status); err != nil {
			return err
		}
	}
	if s.control != nil && (s.mode == "primary" || s.mode == "exit") && s.controlBeatsAfter <= s.controlBeatsBefore {
		return errors.New("unrelated same-image worker lost its live heartbeat")
	}
	return nil
}

func (s *isolationWorld) retained() error {
	if _, err := s.reports("exit", 2); err != nil {
		return err
	}
	if err := s.completed(s.runs[0], s.config.Status); err != nil {
		return fmt.Errorf("fault-free control: %w", err)
	}
	r := s.runs[1]
	if r.absent || object(r.stopped["State"])["Running"] != false || object(r.stopped["State"])["ExitCode"] != float64(s.config.Status) {
		return errors.New("faulted exact worker is not independently observed stopped and present")
	}
	return isolationRecord(r, "owned")
}

func (w *world) isolationResult(status, result int) error {
	s := w.offline.isolation
	if s.mode == "primary" {
		if _, err := s.reports("trailing", 1); err != nil {
			return err
		}
		if s.runs[0].config.Status != status {
			return errors.New("primary exit fixture status differs")
		}
		return s.completed(s.runs[0], status)
	}
	if err := s.retained(); err != nil {
		return err
	}
	r := s.runs[1]
	if r.exit != result || r.config.Status != status {
		return fmt.Errorf("workload %d produced wrapper %d, expected %d", status, r.exit, result)
	}
	waited, failed := false, false
	for _, request := range r.requests {
		if request.Method == "POST" && request.Path == "/containers/"+r.id+"/wait" && request.Status == 200 && request.Observation["StatusCode"] == float64(status) {
			waited = true
		}
		if request.Method == "DELETE" {
			if !waited || request.Status != 503 {
				return errors.New("removal fault did not follow a real native workload exit")
			}
			failed = true
		}
		if request.Status == 403 || request.Status == 502 {
			return errors.New("recorder refusal or unrelated transport failure is not a removal fault")
		}
	}
	if !waited || !failed || !bytes.Contains(r.stderr, []byte("run.cleanup-temporary")) {
		return errors.New("native result lacks real exit and separately classified temporary cleanup failure")
	}
	return nil
}

func (s *isolationWorld) safeDiagnostic() error {
	if len(s.runs) != 2 {
		return errors.New("missing faulted workload")
	}
	r := s.runs[1]
	token := strings.TrimPrefix(fmt.Sprint(r.expected["name"]), "itos-container-")
	reference := "intents/" + s.w.approval.project + "/" + token + ".json"
	if !bytes.Contains(r.stderr, []byte(reference)) {
		return errors.New("cleanup diagnostic omits its original protected intent reference")
	}
	if _, err := s.registryPaths(); err != nil {
		return err
	}
	retries := recoveryRetryPattern.FindAllSubmatch(r.stderr, -1)
	if len(retries) != 1 || string(retries[0][1]) != r.sessionID || !bytes.Contains(r.stderr, []byte("same effective project")) {
		return errors.New("cleanup diagnostic lacks its exact registered same-project stop retry")
	}
	for _, v := range []string{"SLICE_14_SYNTHETIC_REMOVE_BODY", "SLICE_14_SYNTHETIC_ARGV", "SLICE_14_SYNTHETIC_ENV", r.config.Nonce, "stopped successfully", "cleanup complete"} {
		if bytes.Contains(r.stderr, []byte(v)) || strings.Contains(r.record.Bytes, v) || strings.Contains(r.registration.Bytes, v) {
			return errors.New("cleanup diagnostic/ownership leaks payload or claims completed cleanup")
		}
	}
	return nil
}

func (s *isolationWorld) preserved() error {
	if err := s.registryPreserved(); err != nil {
		return err
	}
	before, after := s.resourcesBefore, s.resourcesAfter
	if s.mode == "exit" && len(s.runs) == 2 {
		after = map[string]string{}
		for key, value := range s.resourcesAfter {
			after[key] = value
		}
		var keep []string
		for _, line := range strings.Split(after["/containers/json?all=1"], "\n") {
			var c map[string]any
			_ = json.Unmarshal([]byte(line), &c)
			if c["Id"] != s.runs[1].id {
				keep = append(keep, line)
			}
		}
		after["/containers/json?all=1"] = strings.Join(keep, "\n")
	}
	if before == nil || after == nil || !reflect.DeepEqual(before, after) {
		return errors.New("unrelated resources, networks, persistent volumes or image references changed")
	}
	for _, run := range s.runs {
		input, err := sessionReadRecord(filepath.Join(s.w.doctor.checkout, "slice-14-run-"+run.config.Nonce+".json"))
		if err != nil || input != run.input {
			return errors.New("product changed the separately recorded fixture input")
		}
		token := strings.TrimPrefix(fmt.Sprint(run.expected["name"]), "itos-container-")
		record, err := sessionReadRecord(filepath.Join(s.w.doctor.env["XDG_STATE_HOME"], "itos-container", "intents", s.w.approval.project, token+".json"))
		if err != nil || record != run.record {
			return errors.New("later invocation changed a protected ownership record")
		}
	}
	intentEntries, err := os.ReadDir(filepath.Join(s.w.doctor.env["XDG_STATE_HOME"], "itos-container", "intents", s.w.approval.project))
	if err != nil || len(intentEntries) != len(s.runs) {
		return errors.New("unexpected ownership record inventory")
	}
	if s.mode == "primary" || s.mode == "exit" || s.mode == "filesystem" {
		for _, run := range s.runs {
			b, err := os.ReadFile(filepath.Join(s.w.doctor.checkout, "slice-14-edit-"+run.config.Nonce))
			if err != nil || string(b) != run.config.Nonce {
				return errors.New("checkout edit was lost or changed")
			}
		}
	}
	path := filepath.Join(s.w.doctor.env["XDG_STATE_HOME"], "itos-container", "snapshots", s.w.approval.project, "1.json")
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 || entries[0].Name() != "1.json" {
		return errors.New("unexpected frozen snapshot inventory")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var actual map[string]any
	expected := approvalClone(map[string]any{"schema": 1, "project": s.w.approval.project, "generation": "1", "fingerprint": s.w.approval.fingerprint, "settings": s.w.approval.settings})
	if json.Unmarshal(b, &actual) != nil || !reflect.DeepEqual(actual, expected) {
		return errors.New("original frozen snapshot changed")
	}
	for _, run := range s.runs {
		if run.snapshot != s.runs[0].snapshot {
			return errors.New("second run rewrote the existing frozen snapshot")
		}
	}
	return nil
}
