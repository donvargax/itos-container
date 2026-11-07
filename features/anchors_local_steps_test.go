package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// These records are independently encoded fixture data, not product run output.
// The descriptor belongs to no fixture process; recovery must only compare it.
type anchorRecoveryLocal struct {
	ready, release                          chan struct{}
	held                                    bool
	priorDelay                              bool
	control                                 *world
	savedWorker                             map[string]any
	firstVolume                             string
	reached                                 bool
	mode, fault                             string
	phase, worker, volume, completion, exit string
	name                                    string
	intent                                  map[string]any
	legacyID                                string
	legacy                                  map[string]string
	volumes                                 map[string]map[string]any
	memory                                  doctorResult
	problem                                 error
	firstRequests                           int
	firstRecords                            map[string]string
	original                                string
}

func (w *world) anchorRecoveryLocalSteps(sc *godog.ScenarioContext) {
	w.anchorRecoveryStopSteps(sc)
	w.anchorRecoveryFaultSteps(sc)
	w.anchorRecoveryDefectSteps(sc)
	w.anchorRecoveryRaceSteps(sc)
	sc.Step(`^an anchor recovery fixture with phase "([^"]*)" worker "([^"]*)" volume "([^"]*)" completion "([^"]*)" and observed exit "([^"]*)"$`, w.anchorRecoverySeed)
	sc.Step(`^fresh anchor status processes inspect each local backend in plain and JSON modes$`, func() error {
		w.anchorLocalRecovery.problem = errors.Join(w.recoveryInvocation("status", "--json"), w.recoveryInvocation("status", "--no-json"))
		return w.anchorRecoveryMemory()
	})
	sc.Step(`^each status reports state "([^"]*)" worker "([^"]*)" anchor "([^"]*)" cleanup "([^"]*)" and observed exit "([^"]*)" for the original session$`, w.anchorRecoveryStatus)
	sc.Step(`^each status retains the original fingerprint and the specified controller candidate and exact retry facts$`, w.anchorRecoveryStatusIdentity)
	sc.Step(`^anchor status leaves records locks resources and outside sentinels unchanged without opening checkout or descriptor paths$`, func() error {
		return errors.Join(w.recoveryUnchanged(), w.anchorRecoveryNoReads(), w.anchorRecoveryVolumesUnchanged())
	})
	sc.Step(`^the legacy session keeps its original result without adopting or inspecting the unrelated volume$`, w.anchorRecoveryLegacy)
}

func (w *world) anchorRecoverySeed(phase, worker, volume, completion, exit string) error {
	if err := w.recoverySeed(phase, worker, exit); err != nil {
		return err
	}
	return w.anchorRecoveryAdd(phase, worker, volume, completion, exit)
}
func (w *world) anchorRecoveryAdd(phase, worker, volume, completion, exit string) error {
	a := &anchorRecoveryLocal{phase: phase, worker: worker, volume: volume, completion: completion, exit: exit, name: "itos-container-anchor-" + w.session.token, legacyID: strings.Repeat("c", 64)}
	w.anchorLocalRecovery = a
	r := w.sessionExpectedRecord("owned", w.session.seedID)
	if completion == "durable" {
		r["stage"] = "absent"
	}
	var frozen struct {
		Settings struct {
			Checkout struct{ Device, Inode string }
		}
	}
	data, err := os.ReadFile(w.sessionSnapshotPath())
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, &frozen); err != nil {
		return err
	}
	stage := "owned"
	if completion == "durable" {
		stage = "absent"
	}
	r["schema"] = 2
	r["anchor"] = map[string]any{"session": w.recoveryLocal.id, "name": a.name, "source": "/proc/2147483647/fd/17", "device": frozen.Settings.Checkout.Device, "inode": frozen.Settings.Checkout.Inode, "created": "2026-11-07T17:00:00Z", "stage": stage}
	a.intent = r
	if err = os.WriteFile(w.sessionRecordPath(), approvalJSON(r), 0600); err != nil {
		return err
	}
	anchor := object(r["anchor"])
	labels := map[string]any{"io.itos-container.kind": "checkout-anchor", "io.itos-container.intent": w.session.token, "io.itos-container.project": w.approval.project, "io.itos-container.fingerprint": w.approval.fingerprint, "io.itos-container.session": w.recoveryLocal.id, "io.itos-container.device": anchor["device"], "io.itos-container.inode": anchor["inode"]}
	v := map[string]any{"Name": a.name, "Driver": "local", "Scope": "local", "CreatedAt": anchor["created"], "Labels": labels, "Options": map[string]any{"type": "none", "o": "bind", "device": anchor["source"]}}
	w.session.api.volumes = map[string]map[string]any{"unrelated-anchor": approvalClone(v)}
	w.session.api.volumes["unrelated-anchor"]["Name"] = "unrelated-anchor"
	if volume == "present" {
		w.session.api.volumes[a.name] = v
	}
	if c := w.session.api.local[w.session.seedID]; c != nil {
		c["Mounts"] = []any{map[string]any{"Type": "volume", "Name": a.name, "Driver": "local", "Destination": "/workspace", "RW": true}}
		if c["HostConfig"] == nil {
			c["HostConfig"] = map[string]any{}
		}
		object(c["HostConfig"])["Mounts"] = []any{map[string]any{"Type": "volume", "Source": a.name, "Target": "/workspace", "VolumeOptions": map[string]any{"NoCopy": true}}}
	}
	if err = w.anchorRecoverySeedLegacy(); err != nil {
		return err
	}
	a.volumes = map[string]map[string]any{}
	for name, v := range w.session.api.volumes {
		a.volumes[name] = approvalClone(v)
	}
	if err = w.recoveryCapture(); err != nil {
		return err
	}
	w.recoveryLocal.watch, err = w.sessionWatchInputs()
	if err == nil {
		w.doctor.cleanup = append(w.doctor.cleanup, func() { _ = w.recoveryLocal.watch.file.Close() })
	}
	return err
}

func (w *world) anchorRecoverySeedLegacy() error {
	a := w.anchorLocalRecovery
	token := strings.Repeat("c", 63) + "1"
	r := w.sessionExpectedRecord("absent", strings.Repeat("c", 63)+"2")
	r["name"] = "itos-container-" + token
	intent := filepath.Join(filepath.Dir(w.sessionRecordPath()), token+".json")
	if err := os.WriteFile(intent, approvalJSON(r), 0600); err != nil {
		return err
	}
	dir := filepath.Join(filepath.Dir(filepath.Dir(w.recoveryRecordPath())), a.legacyID)
	if err := os.Mkdir(dir, 0700); err != nil {
		return err
	}
	p := map[string]any{"schema": 1, "id": a.legacyID, "project": w.approval.project, "intent": "intents/" + w.approval.project + "/" + token + ".json", "state": "stopped", "workload_exit": 7, "last_problem": nil}
	if err := os.WriteFile(filepath.Join(dir, "record.json"), approvalJSON(p), 0600); err != nil {
		return err
	}
	for _, lock := range []string{"operation.lock", "controller.lock"} {
		if err := os.WriteFile(filepath.Join(dir, lock), nil, 0600); err != nil {
			return err
		}
	}
	var err error
	a.legacy, err = doctorTree(dir)
	return err
}

func (w *world) anchorRecoveryEntry() (map[string]any, error) {
	if w.anchorLocalRecovery.problem != nil {
		return nil, w.anchorLocalRecovery.problem
	}
	if len(w.recoveryLocal.results) != 2 {
		return nil, fmt.Errorf("missing status processes")
	}
	r := w.recoveryLocal.results[0]
	var report struct {
		Schema   int
		OK       bool
		Sessions []map[string]any
	}
	if r.exit != 0 || json.Unmarshal([]byte(r.stdout), &report) != nil || report.Schema != 1 || !report.OK {
		return nil, fmt.Errorf("anchor status did not succeed: %+v", r)
	}
	for _, e := range report.Sessions {
		if e["id"] == w.recoveryLocal.id {
			return e, nil
		}
	}
	return nil, fmt.Errorf("status omitted anchor-bearing session")
}

func (w *world) anchorRecoveryStatus(phase, worker, volume, cleanup, exit string) error {
	e, err := w.anchorRecoveryEntry()
	if err == nil {
		for key, want := range map[string]string{"state": phase, "resource": worker, "anchor": volume, "cleanup": cleanup} {
			if e[key] != want {
				err = errors.Join(err, fmt.Errorf("anchor status %s = %v, want %s", key, e[key], want))
			}
		}
		if exit == "unknown" && e["workload_exit"] != nil {
			err = errors.Join(err, fmt.Errorf("invented workload exit"))
		}
		if exit != "unknown" {
			n, _ := strconv.Atoi(exit)
			if e["workload_exit"] != float64(n) {
				err = errors.Join(err, fmt.Errorf("lost workload exit"))
			}
		}
		plain := w.recoveryLocal.results[1]
		if plain.exit != 0 || !strings.Contains(plain.stdout, "anchor="+volume) || !strings.Contains(plain.stdout, "resource="+worker) || !strings.Contains(plain.stdout, "cleanup="+cleanup) {
			err = errors.Join(err, fmt.Errorf("plain status omits separate resource facts: %+v", plain))
		}
	}
	return errors.Join(err, w.anchorRecoveryMemoryResult())
}

func (w *world) anchorRecoveryStatusIdentity() error {
	e, err := w.anchorRecoveryEntry()
	if err != nil {
		return err
	}
	complete := w.anchorLocalRecovery.completion == "durable"
	if e["project"] != w.approval.project || e["fingerprint"] != w.approval.fingerprint || e["controller"] != "absent" || e["stop_candidate"] != !complete {
		return fmt.Errorf("anchor identity/controller facts differ: %v", e)
	}
	if complete {
		if e["retry"] != nil {
			return fmt.Errorf("completed anchor retains retry")
		}
	} else if !reflect.DeepEqual(e["retry"], []any{"itos", "container", "stop", w.recoveryLocal.id}) {
		return fmt.Errorf("anchor retry differs: %v", e["retry"])
	}
	return nil
}

func (w *world) anchorRecoveryNoReads() error {
	if err := w.recoveryFinishWatch(); err != nil {
		return err
	}
	if len(w.recoveryLocal.reads) != 0 {
		return fmt.Errorf("anchor recovery opened mutable inputs: %v", w.recoveryLocal.reads)
	}
	for _, r := range w.recoveryLocal.results {
		if strings.Contains(r.stdout+r.stderr, "/proc/") {
			return fmt.Errorf("descriptor source leaked in output")
		}
	}
	return nil
}
func (w *world) anchorRecoveryVolumesUnchanged() error {
	if !reflect.DeepEqual(w.anchorLocalRecovery.volumes, w.session.api.volumes) {
		return fmt.Errorf("status changed volumes")
	}
	return nil
}
func (w *world) anchorRecoveryLegacy() error {
	a := w.anchorLocalRecovery
	dir := filepath.Join(filepath.Dir(filepath.Dir(w.recoveryRecordPath())), a.legacyID)
	after, err := doctorTree(dir)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(a.legacy, after) {
		return fmt.Errorf("legacy state changed")
	}
	var report struct{ Sessions []map[string]any }
	if json.Unmarshal([]byte(w.recoveryLocal.results[0].stdout), &report) != nil {
		return fmt.Errorf("invalid status")
	}
	found := false
	for _, e := range report.Sessions {
		if e["id"] == a.legacyID {
			found = true
			if _, ok := e["anchor"]; ok {
				return fmt.Errorf("legacy result invented anchor")
			}
			if e["cleanup"] != "complete" || e["resource"] != "absent" {
				return fmt.Errorf("legacy result changed")
			}
		}
	}
	if !found {
		return fmt.Errorf("legacy session missing")
	}
	for _, q := range w.sessionRequests() {
		if strings.Contains(q.Path, "unrelated-anchor") {
			return fmt.Errorf("inspected unrelated volume")
		}
	}
	return nil
}
