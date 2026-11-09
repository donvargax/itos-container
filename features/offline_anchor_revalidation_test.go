package features

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// Docker local volumes have no container-style ID. Their original recorded
// name, creation time, driver, scope, labels and options are the identity tuple;
// a matching name alone never authorizes a mutation.
func (a *offlineAPI) anchorMutationRecord(e offlineRequest, v *anchorVolume, stage, worker string) error {
	w := a.w
	want := approvalClone(w.sessionExpectedRecord(stage, worker))
	want["schema"] = float64(2)
	want["name"] = "itos-container-" + fmt.Sprint(v.labels["io.itos-container.intent"])
	anchorStage, created := "owned", v.observed["CreatedAt"]
	if stage == "reserved" {
		anchorStage, created = "intent", ""
	}
	want["anchor"] = map[string]any{
		"name": v.name, "session": v.labels["io.itos-container.session"], "source": v.options["device"],
		"device": v.labels["io.itos-container.device"], "inode": v.labels["io.itos-container.inode"], "created": created, "stage": anchorStage,
	}
	actual, err := sessionDecodeRecord(e.Record.Bytes)
	if err != nil || e.Record.Mode != 0600 || e.Record.UID != w.approval.uid || e.Record.Inode == "" || !reflect.DeepEqual(actual, want) {
		return errors.New("anchor mutation lacks its exact protected original ownership record")
	}
	return nil
}

// This checks the product's journal separately from the recorder's independent
// safety observations. The latter may refuse an unsafe request but must never
// supply missing evidence that the product revalidated before a mutation.
func (w *world) anchorRevalidated(entries []offlineRequest) error {
	a := w.offline.api
	checked := false
	absent := map[string]bool{}
	inspected := map[string]map[string]any{}
	for _, e := range entries {
		if e.Method == "GET" && e.Path == "/info" {
			checked = e.Status == 200 && e.Observation["ID"] == w.doctor.engineID
		}
		if e.Method == "GET" && strings.HasPrefix(e.Path, "/containers/") && strings.HasSuffix(e.Path, "/json") {
			id := strings.TrimSuffix(strings.TrimPrefix(e.Path, "/containers/"), "/json")
			absent[id] = e.Status == 404 && offlineHex(id) && containsSessionID(a.created, id)
		}
		if e.Method == "GET" && strings.HasPrefix(e.Path, "/volumes/") {
			name := strings.TrimPrefix(e.Path, "/volumes/")
			delete(inspected, name)
			if e.Status == 200 {
				inspected[name] = e.Observation
			}
		}
		create := e.Method == "POST" && e.Path == "/volumes/create"
		release := e.Method == "DELETE" && strings.HasPrefix(e.Path, "/volumes/")
		if create || release {
			name := strings.TrimPrefix(e.Path, "/volumes/")
			if create {
				name, _ = e.Body["Name"].(string)
			}
			v := a.anchorVolumes()[name]
			if !checked || v == nil || !anchorVolumeMatches(v, v.observed) {
				return errors.New("anchor mutation lacks fresh original daemon and independently owned volume identity")
			}
			stage, worker := "reserved", ""
			if create {
				if e.Query != "" || e.Body["Driver"] != "local" || !reflect.DeepEqual(e.Body["Labels"], v.labels) || !reflect.DeepEqual(e.Body["DriverOpts"], v.options) {
					return errors.New("anchor allocation differs from the original intent")
				}
			} else {
				r, _ := sessionDecodeRecord(e.Record.Bytes)
				worker, _ = r["container_id"].(string)
				stage = "absent"
				if !absent[worker] || !anchorVolumeMatches(v, inspected[name]) || !anchorVolumeMatches(v, e.Observation) || (e.Query != "" && e.Query != "force=false") {
					return errors.New("anchor release lacks fresh full-ID worker absence and original volume tuple or requests force")
				}
			}
			if err := a.anchorMutationRecord(e, v, stage, worker); err != nil {
				return err
			}
		}
		mutation := e.Method == "DELETE" || (e.Method == "POST" && (strings.HasSuffix(e.Path, "/create") || strings.HasSuffix(e.Path, "/start") || strings.HasSuffix(e.Path, "/kill") || strings.HasSuffix(e.Path, "/stop")))
		if mutation {
			checked = false
			clear(inspected)
		}
	}
	return nil
}

func (w *world) anchorReleased(entries []offlineRequest) error {
	if err := w.anchorRevalidated(entries); err != nil {
		return err
	}
	volumes := w.offline.api.anchorVolumes()
	if len(volumes) == 0 {
		return errors.New("completed run has no independently owned anchor")
	}
	for name := range volumes {
		creates, deletes, confirmed := 0, 0, false
		for _, e := range entries {
			if e.Method == "POST" && e.Path == "/volumes/create" && e.Body["Name"] == name {
				creates++
			}
			if e.Method == "DELETE" && e.Path == "/volumes/"+name && e.Status == 204 {
				deletes++
				confirmed = false
			}
			if deletes > 0 && e.Method == "GET" && e.Path == "/volumes/"+name {
				confirmed = e.Status == 404
			}
		}
		if creates != 1 || deletes != 1 || !confirmed {
			return errors.New("completion lacks one original anchor allocation, release and freshly confirmed absence")
		}
	}
	return nil
}

// Remove only a retained anchor whose full tuple is still independently
// present. A stopped session's deleted anchor requires no inventory exclusion.
func (w *world) recoveryWithoutRetainedAnchor(resources map[string]string) (map[string]string, error) {
	result := resources
	for name, v := range w.offline.api.anchorVolumes() {
		if w.doctor.real.read("/volumes/"+name, nil, 404) == nil {
			continue
		}
		if _, err := w.offline.api.anchorInspect(v); err != nil {
			return nil, err
		}
		retainedWorker := false
		for _, id := range w.offline.api.created {
			worker, err := w.offline.api.inspect(id)
			labels := object(object(worker["Config"])["Labels"])
			if err == nil && w.offline.api.identity(worker, id) == nil && labels["io.itos-container.intent"] == v.labels["io.itos-container.intent"] {
				retainedWorker = true
			}
		}
		if !retainedWorker {
			return nil, errors.New("anchor remains without its independently observed owned worker")
		}
		result = anchorsWithoutVolume(result, name)
	}
	return result, nil
}
