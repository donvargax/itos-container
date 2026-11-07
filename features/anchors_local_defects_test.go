package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

func (w *world) anchorRecoveryDefectSteps(sc *godog.ScenarioContext) {
	sc.Step(`^an anchor recovery fixture with defect "([^"]*)" a safe independent entry and a fault-free anchor control$`, w.anchorRecoveryDefectSeed)
	sc.Step(`^fresh status and stop inspect the defective session and the anchor control runs through both local backends$`, func() error {
		if err := w.anchorRecoveryBeginWatch(); err != nil {
			return err
		}
		a := w.anchorLocalRecovery
		a.problem = errors.Join(w.recoveryInvocation("status", "--json"), w.recoveryTracedInvocation("stop", "--json", w.recoveryLocal.id), a.control.recoveryTracedInvocation("stop", "--json", a.control.recoveryLocal.id))
		return w.anchorRecoveryMemory()
	})
	sc.Step(`^the control releases its own anchor and the defective session reports "([^"]*)" with exit (\d+) without anchor mutation or adoption$`, func(rule string, code int) error {
		var err error
		for _, r := range w.recoveryLocal.results {
			err = errors.Join(err, recoveryHasProblem(r, rule, code))
		}
		return errors.Join(err, w.anchorRecoveryControlResult(), w.recoveryNoMutation(), w.anchorRecoveryMemoryResult())
	})
	sc.Step(`^status retains the safe entry with anchor absent while the defective entry cannot claim completed cleanup$`, w.anchorRecoverySafeEntry)
	sc.Step(`^protected evidence foreign volumes workers and outside targets remain unchanged without repair or descriptor access$`, func() error {
		return errors.Join(w.anchorRecoveryNoReads(), w.recoveryUnchanged(), w.anchorRecoveryVolumesUnchanged())
	})
}

func (w *world) anchorRecoveryDefectSeed(defect string) error {
	worker, phase, completion := "absent", "failed", "pending"
	if defect == "worker mount names a different anchor" {
		worker = "created"
	}
	if defect == "recorded volume name now holds a foreign volume" {
		phase, completion = "stopped", "durable"
	}
	if defect == "anchor belongs to a different host user" {
		if err := w.recoveryOwnershipDefect("mutually consistent other UID"); err != nil {
			return err
		}
		delete(w.session.api.local, w.session.seedID)
		if err := w.anchorRecoveryAdd(phase, worker, "present", completion, "unknown"); err != nil {
			return err
		}
	} else if err := w.anchorRecoverySeed(phase, worker, "present", completion, "unknown"); err != nil {
		return err
	}
	a := w.anchorLocalRecovery
	a.mode, a.fault = "defect", defect
	if defect == "anchor belongs to a different host user" {
		old := filepath.Dir(filepath.Dir(w.recoveryRecordPath()))
		w.approval.uid = strconv.Itoa(os.Geteuid())
		next := filepath.Dir(filepath.Dir(w.recoveryRecordPath()))
		if old != next {
			if err := os.MkdirAll(next, 0700); err != nil {
				return err
			}
			entries, err := os.ReadDir(old)
			if err != nil {
				return err
			}
			for _, entry := range entries {
				if err := os.Rename(filepath.Join(old, entry.Name()), filepath.Join(next, entry.Name())); err != nil {
					return err
				}
			}
			if err := os.Remove(old); err != nil {
				return err
			}
		}
	}
	if err := w.anchorRecoveryControlSeed(); err != nil {
		return err
	}
	v := w.session.api.volumes[a.name]
	labels, options := object(v["Labels"]), object(v["Options"])
	path := w.sessionRecordPath()
	data := approvalJSON(a.intent)
	switch defect {
	case "selected daemon differs":
		w.doctor.engine.info["ID"] = "another-daemon"
	case "anchor belongs to a different host user":
	case "anchor belongs to a different project":
		labels["io.itos-container.project"] = strings.Repeat("e", 64)
	case "anchor session label differs":
		labels["io.itos-container.session"] = strings.Repeat("e", 64)
	case "anchor operation label differs":
		labels["io.itos-container.intent"] = strings.Repeat("e", 64)
	case "anchor fingerprint label differs":
		labels["io.itos-container.fingerprint"] = "sha256:" + strings.Repeat("e", 64)
	case "anchor driver differs":
		v["Driver"] = "foreign"
	case "anchor bind type differs":
		options["type"] = "tmpfs"
	case "anchor bind options differ":
		options["o"] = "rbind"
	case "anchor device names a reused foreign descriptor":
		options["device"] = fmt.Sprintf("/proc/%d/fd/0", os.Getpid())
	case "worker mount names a different anchor":
		w.session.api.local[w.session.seedID]["Mounts"] = []any{map[string]any{"Type": "volume", "Name": "foreign-anchor", "Destination": "/workspace", "RW": true}}
	case "recorded volume name now holds a foreign volume":
		v["CreatedAt"] = "2026-11-07T18:00:00Z"
	case "required protected anchor record missing":
		delete(a.intent, "anchor")
		data = approvalJSON(a.intent)
	case "unsupported mount-bearing record version":
		a.intent["schema"] = 3
		data = approvalJSON(a.intent)
	case "duplicate anchor ownership field":
		data = []byte(strings.Replace(string(data), `"anchor":{`, `"anchor":{"session":"duplicate",`, 1))
	case "anchor record symlink":
		if err := os.Rename(path, path+".outside"); err != nil {
			return err
		}
		if err := os.Symlink(filepath.Base(path)+".outside", path); err != nil {
			return err
		}
	case "anchor record hard link":
		if err := os.Link(path, path+".outside"); err != nil {
			return err
		}
	case "anchor state ancestor writable by others":
		if err := os.Chmod(filepath.Dir(path), 0770); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown anchor defect %s", defect)
	}
	if strings.Contains(defect, "record missing") || strings.Contains(defect, "record version") || strings.Contains(defect, "duplicate anchor") {
		if err := os.WriteFile(path, data, 0600); err != nil {
			return err
		}
	}
	if err := w.anchorRecoverySafeSeed(); err != nil {
		return err
	}
	a.volumes = map[string]map[string]any{}
	for name, v := range w.session.api.volumes {
		a.volumes[name] = approvalClone(v)
	}
	return w.recoveryCapture()
}

func (w *world) anchorRecoverySafeSeed() error {
	a := w.anchorLocalRecovery
	token := strings.Repeat("c", 63) + "1"
	path := filepath.Join(filepath.Dir(w.sessionRecordPath()), token+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var r map[string]any
	if err = json.Unmarshal(data, &r); err != nil {
		return err
	}
	r["schema"] = 2
	settings := approvalClone(w.approval.settings)
	uid, gid := strconv.Itoa(os.Geteuid()), strconv.Itoa(os.Getegid())
	object(settings["host"])["uid"], object(settings["host"])["gid"] = uid, gid
	object(settings["manifest"])["containerUser"], object(settings["manifest"])["remoteUser"] = uid, uid
	object(settings["engine"])["id"] = w.doctor.engine.info["ID"]
	checkout := object(settings["checkout"])
	checkout["inode"] = fmt.Sprint(checkout["inode"]) + "1"
	project := approvalHash(map[string]any{"schema": 1, "uid": uid, "checkout": checkout})
	fingerprint := "sha256:" + approvalHash(settings)
	root := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")
	snapshot := map[string]any{"schema": 1, "project": project, "generation": "2", "fingerprint": fingerprint, "settings": settings}
	if err := writeManifestFixture(filepath.Join(root, "snapshots", project, "2.json"), approvalJSON(snapshot), 0600); err != nil {
		return err
	}
	r["project"], r["generation"], r["fingerprint"], r["snapshot"] = project, "2", fingerprint, "snapshots/"+project+"/2.json"
	r["uid"], r["gid"], r["engine_id"] = uid, gid, w.doctor.engine.info["ID"]
	r["anchor"] = map[string]any{"session": a.legacyID, "name": "itos-container-anchor-" + token, "source": "/proc/2147483646/fd/9", "device": checkout["device"], "inode": checkout["inode"], "created": "2026-11-07T16:00:00Z", "stage": "absent"}
	if err := writeManifestFixture(filepath.Join(root, "intents", project, token+".json"), approvalJSON(r), 0600); err != nil {
		return err
	}
	publicPath := filepath.Join(filepath.Dir(filepath.Dir(w.recoveryRecordPath())), a.legacyID, "record.json")
	p := map[string]any{"schema": 1, "id": a.legacyID, "project": project, "intent": "intents/" + project + "/" + token + ".json", "state": "stopped", "workload_exit": 7, "last_problem": nil}
	return os.WriteFile(publicPath, approvalJSON(p), 0600)
}

func (w *world) anchorRecoverySafeEntry() error {
	var report struct{ Sessions []map[string]any }
	if len(w.recoveryLocal.results) == 0 || json.Unmarshal([]byte(w.recoveryLocal.results[0].stdout), &report) != nil {
		return fmt.Errorf("missing status")
	}
	found := false
	for _, e := range report.Sessions {
		if e["id"] == w.anchorLocalRecovery.legacyID {
			found = true
			if e["anchor"] != "absent" || e["resource"] != "absent" || e["cleanup"] != "complete" {
				return fmt.Errorf("safe entry lost: %v", e)
			}
		}
		if e["id"] == w.recoveryLocal.id && e["cleanup"] == "complete" {
			return fmt.Errorf("defective entry claims completion")
		}
	}
	if !found {
		return fmt.Errorf("safe independent anchor omitted")
	}
	return nil
}
