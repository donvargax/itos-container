package features

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

func (w *world) sessionLabels() map[string]any {
	return map[string]any{"io.itos-container.intent": w.session.token, "io.itos-container.project": w.approval.project, "io.itos-container.fingerprint": w.approval.fingerprint}
}

func (w *world) sessionRequestProblem(body map[string]any) error {
	if body == nil || body["Image"] != w.doctor.image || body["User"] != w.approval.uid+":"+w.approval.gid || !reflect.DeepEqual(body["Labels"], w.sessionLabels()) {
		return fmt.Errorf("create request lacks exact frozen identity")
	}
	if !reflect.DeepEqual(body["Cmd"], []any{"/bin/true"}) || !reflect.DeepEqual(body["Entrypoint"], []any{}) {
		return fmt.Errorf("create request lacks inert command and explicitly empty entrypoint")
	}
	host := object(body["HostConfig"])
	if host["NetworkMode"] != "none" || host["ReadonlyRootfs"] != true || host["PidsLimit"] != float64(4096) || !reflect.DeepEqual(host["CapDrop"], []any{"ALL"}) || !reflect.DeepEqual(host["SecurityOpt"], []any{"no-new-privileges"}) || object(host["RestartPolicy"])["Name"] != "no" {
		return fmt.Errorf("create request lacks minimal hardening")
	}
	for _, key := range []string{"AutoRemove", "Privileged", "PublishAllPorts"} {
		if host[key] != nil && host[key] != false {
			return fmt.Errorf("create request enables %s", key)
		}
	}
	for _, key := range []string{"PidMode", "IpcMode", "UTSMode", "UsernsMode", "CgroupnsMode", "Runtime", "VolumeDriver"} {
		if host[key] != nil && host[key] != "" {
			return fmt.Errorf("create request enables %s", key)
		}
	}
	for _, fields := range []struct {
		object map[string]any
		keys   []string
	}{
		{body, []string{"Volumes", "ExposedPorts", "Env"}},
		{host, []string{"Mounts", "Binds", "Tmpfs", "VolumesFrom", "PortBindings", "CapAdd", "Devices", "DeviceRequests", "ExtraHosts", "Links", "Dns", "DnsSearch", "DnsOptions", "GroupAdd"}},
	} {
		for _, key := range fields.keys {
			value := fields.object[key]
			empty := value == nil
			switch collection := value.(type) {
			case []any:
				empty = len(collection) == 0
			case map[string]any:
				empty = len(collection) == 0
			}
			if !empty {
				return fmt.Errorf("create request supplies %s", key)
			}
		}
	}
	return nil
}

func (w *world) sessionIdentityProblem(container map[string]any, id string) error {
	decoded, err := hex.DecodeString(id)
	if err != nil || len(decoded) != 32 || strings.ToLower(id) != id || container["Id"] != id || container["Name"] != "/itos-container-"+w.session.token || container["Image"] != w.doctor.imageID || !reflect.DeepEqual(object(container["Config"])["Labels"], w.sessionLabels()) {
		return fmt.Errorf("independent container identity differs from frozen intent")
	}
	state := object(container["State"])
	if state == nil || state["Status"] != "created" || state["Running"] != false || state["Pid"] != float64(0) || (state["StartedAt"] != "0001-01-01T00:00:00Z" && state["StartedAt"] != "") {
		return fmt.Errorf("independent container observation does not prove never-started state")
	}
	return nil
}

func (w *world) sessionObservedCreate() error {
	s := w.session
	if s.fixtureError != nil {
		return s.fixtureError
	}
	if s.create.PreflightFailure != "" {
		return fmt.Errorf("session prerequisite failed before the tested boundary: %s", s.create.PreflightFailure)
	}
	if s.api.problem != nil {
		return s.api.problem
	}
	count := 0
	for _, r := range s.api.requests {
		if r.Method == "POST" && r.Path == "/containers/create" {
			count++
		}
	}
	if count != 1 || len(s.api.created) != 1 {
		return fmt.Errorf("expected exactly one observed create, got %d requests and %d created IDs (outcome %s)", count, len(s.api.created), s.create.Outcome)
	}
	id := s.api.created[0]
	return w.sessionIdentityProblem(s.api.observed[id], id)
}

func (w *world) sessionOwned() error {
	if err := w.sessionObservedCreate(); err != nil {
		return err
	}
	s := w.session
	id := s.api.created[0]
	if s.create.Outcome != "owned-unstarted" || !s.create.Handoff || s.create.ID != id || s.create.Reference != s.reference || s.create.Workload != nil {
		return fmt.Errorf("creation did not return the exact owned-unstarted handoff")
	}
	if err := w.sessionExactRecord(s.owned, "owned", id); err != nil {
		return err
	}
	return w.sessionDurability(s.create.Events, "owned", id)
}

func (w *world) sessionExactRecord(observed sessionRecord, stage, id string) error {
	record, err := sessionDecodeRecord(observed.Bytes)
	if err != nil || !reflect.DeepEqual(record, w.sessionExpectedRecord(stage, id)) {
		return fmt.Errorf("record differs from independent %s bindings", stage)
	}
	if observed.Mode != 0600 || observed.UID != w.approval.uid {
		return fmt.Errorf("intent record lacks private effective-user ownership")
	}
	for _, path := range []string{filepath.Dir(w.sessionRecordPath()), filepath.Dir(filepath.Dir(w.sessionRecordPath()))} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
			return fmt.Errorf("intent directory lacks private permissions")
		}
	}
	return nil
}

func (w *world) sessionDurability(events []sessionEvent, stage, id string) error {
	positions := map[string]int{}
	for i, event := range events {
		if event.Stage != stage || event.Failed {
			continue
		}
		record, err := sessionDecodeRecord(event.Bytes)
		if err != nil || !reflect.DeepEqual(record, w.sessionExpectedRecord(stage, id)) {
			return fmt.Errorf("%s durability event contains different bindings", stage)
		}
		positions[event.Operation] = i + 1
	}
	for _, required := range []string{"write", "file fsync", "atomic publication", "parent fsync"} {
		if positions[required] == 0 {
			return fmt.Errorf("%s record lacks successful %s", stage, required)
		}
	}
	if positions["write"] >= positions["file fsync"] || positions["file fsync"] >= positions["atomic publication"] || positions["atomic publication"] >= positions["parent fsync"] {
		return fmt.Errorf("%s publication did not follow write/fsync/publish/parent-fsync order", stage)
	}
	return nil
}

func (w *world) sessionIntentBeforeCreate() error {
	if err := w.sessionObservedCreate(); err != nil {
		return err
	}
	for _, request := range w.session.api.requests {
		if request.Method != "POST" || request.Path != "/containers/create" {
			continue
		}
		if err := w.sessionRequestProblem(request.Body); err != nil {
			return err
		}
		if err := w.sessionExactRecord(request.Record, "intent", ""); err != nil {
			return err
		}
		if err := w.sessionDurability(request.Events, "intent", ""); err != nil {
			return err
		}
		for _, event := range request.Events {
			if event.Operation == "new ancestor fsync" && !event.Failed {
				return nil
			}
		}
		return fmt.Errorf("create preceded new ancestor durability")
	}
	return fmt.Errorf("no create checkpoint")
}

func (w *world) sessionNeverStarted() error {
	if err := w.sessionObservedCreate(); err != nil {
		return err
	}
	id := w.session.api.created[0]
	inspected := false
	for _, r := range w.session.api.requests {
		if r.Method == "GET" && r.Path == "/containers/"+id+"/json" && r.Status == 200 {
			inspected = true
		}
		if r.Method == "DELETE" {
			if !inspected || r.Path != "/containers/"+id || r.Status != 204 {
				return fmt.Errorf("removal preceded full-ID inspection or targeted another resource")
			}
			return w.sessionIdentityProblem(w.session.api.observed[id], id)
		}
	}
	return fmt.Errorf("no product removal observed")
}

func (w *world) sessionAbsent() error {
	if err := w.sessionNeverStarted(); err != nil {
		return err
	}
	s := w.session
	id := s.api.created[0]
	deleted := false
	for _, r := range s.api.requests {
		if r.Method == "DELETE" && r.Path == "/containers/"+id && r.Status == 204 {
			deleted = true
		}
		if deleted && r.Method == "GET" && r.Path == "/containers/"+id+"/json" && r.Status == 404 && s.api.absent[id] {
			return nil
		}
	}
	return fmt.Errorf("no independent concrete-ID absence after product removal")
}

func (w *world) sessionDurableAbsent() error {
	if err := w.sessionAbsent(); err != nil {
		return err
	}
	s := w.session
	if s.cleanup.Outcome != "cleanup-complete" || s.cleanup.Handoff || s.cleanup.Workload != nil || s.create.Workload != nil {
		return fmt.Errorf("cleanup lacks completion or invented a worker result")
	}
	id := s.api.created[0]
	if err := w.sessionExactRecord(s.final, "absent", id); err != nil {
		return err
	}
	return w.sessionDurability(s.cleanup.Events, "absent", id)
}

func (w *world) sessionRecoveryInspection() error {
	if err := w.sessionIntentBeforeCreate(); err != nil {
		return err
	}
	s := w.session
	if s.create.Reference != s.reference {
		return fmt.Errorf("failed creation lost the surviving intent reference")
	}
	if err := w.sessionExactRecord(s.owned, "intent", ""); err != nil {
		return err
	}
	id := s.api.created[0]
	seenDaemon, seenName, seenID := false, false, false
	for _, r := range s.api.requests[s.cleanupStart:] {
		if r.Method == "GET" && r.Path == "/info" && r.Status == 200 {
			seenDaemon = true
		}
		if r.Method == "GET" && r.Path == "/containers/itos-container-"+s.token+"/json" && r.Status == 200 {
			seenName = true
		}
		if seenName && r.Method == "GET" && r.Path == "/containers/"+id+"/json" && r.Status == 200 {
			seenID = true
		}
		if r.Method == "DELETE" {
			if !seenDaemon || !seenName || !seenID || r.Path != "/containers/"+id {
				return fmt.Errorf("fresh cleanup did not revalidate daemon, exact name and full ID")
			}
			return nil
		}
	}
	return fmt.Errorf("fresh cleanup never removed the reconciled resource")
}
