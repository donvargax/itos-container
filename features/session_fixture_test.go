package features

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

type sessionResult struct {
	Outcome, Reference, ID, Cleanup, PreflightFailure string
	Handoff                                           bool
	Workload                                          *int
	Events                                            []sessionEvent
}

type sessionEvent struct {
	Operation, Stage, Bytes string
	Failed                  bool
}

type sessionRecord struct {
	Bytes, Inode string
	Mode         uint32
	UID          string
	Mtime        int64
}

type sessionWorld struct {
	api                             *sessionAPI
	token, reference, fault         string
	journal, umask, backend         string
	create, cleanup                 sessionResult
	owned, final                    sessionRecord
	before, after                   map[string]string
	resourcesBefore, resourcesAfter map[string]string
	transcript                      string
	fixtureError                    error
	cleanupStart                    int
}

func (w *world) sessionFixture(real bool, umask string) error {
	if real {
		if err := w.doctorRealCheckout(false); err != nil {
			return err
		}
		if err := w.doctorLocalAPI(false); err != nil {
			return err
		}
		d := w.doctor
		d.engine.info["ID"] = d.engineID
		d.engine.image["Id"] = d.imageID
		w.approval = &approvalWorld{uid: strconv.Itoa(os.Geteuid()), gid: strconv.Itoa(os.Getegid())}
	} else if err := w.approvalCheckout(); err != nil {
		return err
	}
	s := &sessionWorld{umask: umask, backend: "disk"}
	w.session = s
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return err
	}
	s.token = hex.EncodeToString(token[:])
	s.journal = filepath.Join(w.dir, "slice-9-io.jsonl")
	if err := os.WriteFile(s.journal, nil, 0600); err != nil {
		return err
	}
	if err := w.startSessionAPI(); err != nil {
		return err
	}
	if err := w.approvalSeed("1", false); err != nil {
		return err
	}
	s.reference = filepath.ToSlash(filepath.Join("intents", w.approval.project, s.token+".json"))
	for name, text := range map[string]string{
		filepath.Join(w.doctor.checkout, "edit.txt"):    "retain checkout edit",
		filepath.Join(w.doctor.host, "unrelated-state"): "retain unrelated host state",
	} {
		if err := os.WriteFile(name, []byte(text), 0600); err != nil {
			return err
		}
	}
	prior := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "intents", strings.Repeat("e", 64), strings.Repeat("d", 64)+".json")
	if err := writeManifestFixture(prior, []byte(`{"fixture":"retain unrelated intent"}`), 0600); err != nil {
		return err
	}
	if real {
		var err error
		s.resourcesBefore, err = w.doctor.real.snapshotResources()
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *world) sessionRecordPath() string {
	return filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", filepath.FromSlash(w.session.reference))
}

func sessionReadRecord(path string) (sessionRecord, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return sessionRecord{}, nil
	}
	if err != nil {
		return sessionRecord{}, err
	}
	if !info.Mode().IsRegular() {
		return sessionRecord{}, fmt.Errorf("observer found a non-regular intent")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return sessionRecord{}, err
	}
	stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
	return sessionRecord{Bytes: string(data), Mode: uint32(info.Mode().Perm()), UID: fmt.Sprint(stat.FieldByName("Uid")), Inode: fmt.Sprintf("%v:%v", stat.FieldByName("Dev"), stat.FieldByName("Ino")), Mtime: info.ModTime().UnixNano()}, nil
}

func (w *world) sessionProtected() (map[string]string, error) {
	all := map[string]string{}
	for _, root := range []string{w.doctor.checkout, w.doctor.host} {
		entries, err := doctorTree(root)
		if err != nil {
			return nil, err
		}
		intentRoot := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "intents")
		for path, value := range entries {
			if path == w.sessionRecordPath() || path == filepath.Dir(w.sessionRecordPath()) || path == intentRoot {
				continue
			}
			info, err := os.Lstat(path)
			if err != nil {
				return nil, err
			}
			if info.Mode().IsRegular() {
				stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
				value += fmt.Sprintf(":%v:%v:%v:%d", stat.FieldByName("Dev"), stat.FieldByName("Ino"), stat.FieldByName("Nlink"), info.ModTime().UnixNano())
			}
			all[path] = value
		}
	}
	return all, nil
}

func (w *world) sessionRun(recovering bool) error {
	s := w.session
	// Observation failures are data for Then; the action still records early
	// unavailable/preflight outcomes rather than manufacturing a setup failure.
	result, err := w.sessionProbe("create", "", func() error {
		var err error
		s.before, err = w.sessionProtected()
		return err
	})
	if err != nil {
		s.fixtureError = err
		return nil
	}
	s.create = result
	s.owned, err = sessionReadRecord(w.sessionRecordPath())
	if err != nil {
		s.fixtureError = err
		return nil
	}
	if recovering {
		// The named write fault remains active for creation, including any
		// immediate cleanup. Explicit recovery is a fresh subprocess.
		s.fault = ""
	}
	s.cleanupStart = len(s.api.requests)
	s.cleanup, err = w.sessionProbe("cleanup", result.Reference, nil)
	if err != nil {
		s.fixtureError = err
		return nil
	}
	s.final, err = sessionReadRecord(w.sessionRecordPath())
	if err == nil {
		s.after, err = w.sessionProtected()
	}
	if err == nil && w.doctor.real != nil {
		s.resourcesAfter, err = w.doctor.real.snapshotResources()
	}
	s.fixtureError = err
	return nil
}

func (w *world) closeSession() error {
	if w.session == nil || w.session.api == nil {
		return nil
	}
	// Harness cleanup is separate from product evidence and is bounded. Even a
	// failed Then must not strand an independently observed fixture resource.
	err := w.session.api.cleanupObserved()
	return errors.Join(err, w.session.api.server.Close())
}

func (w *world) sessionExpectedRecord(stage, id string) map[string]any {
	a, d, s := w.approval, w.doctor, w.session
	return map[string]any{"schema": float64(1), "project": a.project, "generation": "1", "fingerprint": a.fingerprint,
		"snapshot": "snapshots/" + a.project + "/1.json", "uid": a.uid, "gid": a.gid,
		"engine_endpoint": d.env["DOCKER_HOST"], "engine_id": d.engineID, "name": "itos-container-" + s.token,
		"image_reference": d.image, "image_id": d.imageID, "container_id": id, "stage": stage}
}

func sessionDecodeRecord(bytes string) (map[string]any, error) {
	var record map[string]any
	err := json.Unmarshal([]byte(bytes), &record)
	return record, err
}
