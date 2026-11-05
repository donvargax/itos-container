package features

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The feature process exchanges data with the test-only cmd assembly; it never
// imports product packages. These wire structs describe observations, not results
// the fake must return. The fake and production controller execute in the child.
type sessionMemoryObservation struct {
	ID, Name, ImageID, Token, Project, Fingerprint string
	StatePresent                                   bool
	Status                                         string
	Running                                        bool
	PID                                            int
	Started                                        bool
}
type sessionMemoryEvent struct {
	Method, Target string
	Record         map[string]any
	Observation    sessionMemoryObservation
	Status         uint8
	Intent         string
	Writes         []sessionEvent
}
type sessionMemoryOutput struct {
	Store struct {
		Records map[string][]byte
		Events  []sessionEvent
	}
	Engine struct {
		Resources map[string]sessionMemoryObservation
		Events    []sessionMemoryEvent
	}
}

func memoryObservation(c map[string]any) sessionMemoryObservation {
	text := func(m map[string]any, k string) string { s, _ := m[k].(string); return s }
	state := object(c["State"])
	labels := object(object(c["Config"])["Labels"])
	running, _ := state["Running"].(bool)
	pid, _ := state["Pid"].(float64)
	started := text(state, "StartedAt")
	return sessionMemoryObservation{ID: text(c, "Id"), Name: strings.TrimPrefix(text(c, "Name"), "/"), ImageID: text(c, "Image"), Token: text(labels, "io.itos-container.intent"), Project: text(labels, "io.itos-container.project"), Fingerprint: text(labels, "io.itos-container.fingerprint"), StatePresent: state != nil, Status: text(state, "Status"), Running: running, PID: int(pid), Started: started != "" && started != "0001-01-01T00:00:00Z"}
}
func memoryContainer(o sessionMemoryObservation) map[string]any {
	c := map[string]any{"Id": o.ID, "Name": "/" + o.Name, "Image": o.ImageID, "Config": map[string]any{"Labels": map[string]any{"io.itos-container.intent": o.Token, "io.itos-container.project": o.Project, "io.itos-container.fingerprint": o.Fingerprint}}}
	if o.StatePresent {
		started := "0001-01-01T00:00:00Z"
		if o.Started {
			started = "2026-01-01T00:00:00Z"
		}
		c["State"] = map[string]any{"Status": o.Status, "Running": o.Running, "Pid": float64(o.PID), "StartedAt": started}
	}
	return c
}
func (w *world) writeMemorySeed(path string) error {
	s := w.session
	records, snapshots := map[string][]byte{}, map[string][]byte{}
	unsafeRecord, unsafeSnapshot := false, false
	for _, target := range []struct {
		path, key string
		files     map[string][]byte
		unsafe    *bool
	}{
		{w.sessionRecordPath(), s.reference, records, &unsafeRecord},
		{w.sessionSnapshotPath(), "snapshots/" + w.approval.project + "/1.json", snapshots, &unsafeSnapshot},
	} {
		info, err := os.Lstat(target.path)
		if err == nil {
			if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
				*target.unsafe = true
			} else {
				record, e := sessionReadRecord(target.path)
				if e != nil {
					return e
				}
				if record.UID != fmt.Sprint(os.Geteuid()) {
					*target.unsafe = true
				}
				target.files[target.key] = []byte(record.Bytes)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	for _, dir := range []string{filepath.Dir(w.sessionRecordPath()), filepath.Dir(filepath.Dir(w.sessionRecordPath()))} {
		info, err := os.Lstat(dir)
		if err == nil && (!info.IsDir() || info.Mode().Perm() != 0700) {
			unsafeRecord = true
		}
	}
	// Hard-link safety is independently read from the fixture metadata.
	if s.condition == "record with another hard link" {
		unsafeRecord = true
	}
	resources := map[string]sessionMemoryObservation{}
	for id, c := range s.api.local {
		resources[id] = memoryObservation(c)
	}
	removed := false
	for _, r := range s.api.requests {
		removed = removed || r.Method == "DELETE"
	}
	unavailable := false
	if s.fault == "owned-record file fsync then unavailable" {
		events, _ := sessionReadEvents(s.journal)
		for _, e := range events {
			if e.Stage == "owned" && e.Operation == "file fsync" && e.Failed {
				unavailable = true
			}
		}
	}
	seed := map[string]any{"Unavailable": unavailable, "Records": records, "Snapshots": snapshots, "Resources": resources, "UnsafeRecord": unsafeRecord, "UnsafeSnapshot": unsafeSnapshot,
		"Endpoint": w.doctor.env["DOCKER_HOST"], "DaemonID": w.doctor.engine.info["ID"], "CreateID": approvalHash(map[string]any{"fixture-created": s.token}),
		"Inspection": s.inspection, "Following": s.following, "Removal": s.removal, "Fault": s.fault, "DisagreeName": s.condition == "name inspection and ID inspection disagree", "Removed": removed}
	return os.WriteFile(path, approvalJSON(seed), 0600)
}
func (w *world) observeMemory(observed *sessionMemoryOutput) error {
	if observed == nil {
		return fmt.Errorf("memory backend produced no independent observations")
	}
	s := w.session
	for ref, data := range observed.Store.Records {
		if ref != s.reference {
			return fmt.Errorf("fake attempted unrelated record mutation")
		}
		path := w.sessionRecordPath()
		info, err := os.Lstat(path)
		if err == nil && !info.Mode().IsRegular() {
			continue
		}
		old, _ := os.ReadFile(path)
		if string(old) == string(data) {
			continue
		}
		if err = writeManifestFixture(path+".observed", data, 0600); err != nil {
			return err
		}
		if err = os.Rename(path+".observed", path); err != nil {
			return err
		}
	}
	for _, e := range observed.Engine.Events {
		r := sessionRequest{Query: ""}
		switch e.Method {
		case "INFO":
			r.Method, r.Path, r.Status = "GET", "/info", 200
		case "CREATE":
			r.Method, r.Path, r.Status = "POST", "/containers/create", 201
			r.Query = "name=" + e.Target
			r.Body = memoryCreateRequest(e.Record)
			r.Record = sessionRecord{Bytes: e.Intent, Mode: 0600, UID: fmt.Sprint(os.Geteuid())}
			r.Events = e.Writes
			s.api.created = append(s.api.created, e.Observation.ID)
			s.api.observed[e.Observation.ID] = memoryContainer(e.Observation)
		case "INSPECT":
			r.Method, r.Path, r.Status = "GET", "/containers/"+e.Target+"/json", 200
			switch e.Status {
			case 1:
				r.Status = 404
				s.api.absent[e.Target] = observed.Engine.Resources[e.Target].ID == ""
			case 2, 3:
				r.Status = 503
			}
		case "REMOVE":
			r.Method, r.Path, r.Status = "DELETE", "/containers/"+e.Target, 204
		default:
			return fmt.Errorf("unexpected memory engine operation %s", e.Method)
		}
		s.api.requests = append(s.api.requests, r)
	}
	s.api.local = map[string]map[string]any{}
	for id, c := range observed.Engine.Resources {
		s.api.local[id] = memoryContainer(c)
	}
	journal, err := os.OpenFile(s.journal, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = journal.Close() }()
	for _, e := range observed.Store.Events {
		if err = json.NewEncoder(journal).Encode(e); err != nil {
			return err
		}
	}
	return nil
}
func memoryCreateRequest(r map[string]any) map[string]any {
	// The fake's Create capability means the same minimal unstarted configuration
	// as the adapter contract. Its recorded record supplies every variable binding.
	return map[string]any{"Image": r["image_reference"], "User": fmt.Sprint(r["uid"]) + ":" + fmt.Sprint(r["gid"]), "Cmd": []any{"/bin/true"}, "Entrypoint": []any{},
		"Labels":     map[string]any{"io.itos-container.intent": strings.TrimPrefix(fmt.Sprint(r["name"]), "itos-container-"), "io.itos-container.project": r["project"], "io.itos-container.fingerprint": r["fingerprint"]},
		"HostConfig": map[string]any{"NetworkMode": "none", "ReadonlyRootfs": true, "PidsLimit": float64(4096), "CapDrop": []any{"ALL"}, "SecurityOpt": []any{"no-new-privileges"}, "RestartPolicy": map[string]any{"Name": "no"}}}
}
