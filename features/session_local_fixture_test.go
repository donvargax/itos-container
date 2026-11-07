package features

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

func (w *world) sessionEach(check func(*world) error) error {
	if len(w.session.cases) == 0 {
		return check(w)
	}
	for _, child := range w.session.cases {
		if err := check(child); err != nil {
			return fmt.Errorf("%s: %w", child.session.backend, err)
		}
	}
	return nil
}

func (w *world) sessionLocalFixture(mode, condition string) error {
	w.session = &sessionWorld{}
	for _, backend := range []string{"memory", "disk"} {
		dir := filepath.Join(w.dir, backend)
		if err := os.Mkdir(dir, 0700); err != nil {
			return err
		}
		child := &world{root: w.root, bin: w.bin, dir: dir}
		if err := child.sessionFixture(false, "0022"); err != nil {
			return err
		}
		w.session.cases = append(w.session.cases, child)
		s := child.session
		s.backend, s.mode, s.condition = backend, mode, condition
		switch mode {
		case "umask":
			s.umask = condition
		case "intent":
			s.fault = "intent " + condition
		case "owned":
			s.fault = "owned-record " + condition
		case "absent":
			s.mode = "recovery"
			s.fault = "absent-record " + condition
		case "reconcile-absent", "reconcile-owned":
			s.mode = "recovery"
			s.fault = strings.TrimPrefix(mode, "reconcile-") + "-record " + condition
		case "token":
			s.fault = condition
		case "inspect":
			s.inspection = condition
		case "compound":
			s.fault = "owned-record file fsync then unavailable"
		case "reply":
			if condition == "lost after server-side success" {
				s.fault = "lost create response"
			} else {
				s.fault = "empty create ID"
			}
		case "delayed":
			s.fault = "delayed create"
			s.following = condition
		case "handoff":
			s.change = condition
		case "identity", "evidence", "pre-remove", "changed", "already-absent", "remove", "storage":
			s.mode = "recovery"
		case "twice", "image":
		default:
			return fmt.Errorf("unknown local fixture mode %q", mode)
		}
		if s.mode == "recovery" {
			if err := child.sessionSeedRecovery(mode, condition); err != nil {
				return err
			}
		}
		if mode == "token" && condition == "generated token record already used" {
			if err := writeManifestFixture(child.sessionRecordPath(), []byte(`{"unrelated":"occupied token"}`), 0600); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *world) sessionSnapshotPath() string {
	return filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "snapshots", w.approval.project, w.sessionGeneration()+".json")
}

func (w *world) sessionGeneration() string {
	if w.approval.generation == "" {
		return "1"
	}
	return w.approval.generation
}
func (w *world) sessionSeedSnapshot() error {
	if err := w.seedControlLocations(); err != nil {
		return err
	}
	a := w.approval
	return writeManifestFixture(w.sessionSnapshotPath(), approvalJSON(map[string]any{"schema": 1, "project": a.project, "generation": "1", "fingerprint": a.fingerprint, "settings": a.settings}), 0600)
}
func (w *world) sessionSeedRecovery(mode, condition string) error {
	s := w.session
	if mode == "identity" && condition == "record UID differs from the current host UID" {
		a := w.approval
		other, _ := strconv.Atoi(a.uid)
		a.uid = strconv.Itoa(other + 1)
		settings := approvalClone(a.settings)
		object(settings["host"])["uid"] = a.uid
		object(settings["manifest"])["containerUser"], object(settings["manifest"])["remoteUser"] = a.uid, a.uid
		a.settings = settings
		a.fingerprint = "sha256:" + approvalHash(settings)
		a.project = approvalHash(map[string]any{"schema": 1, "uid": a.uid, "checkout": settings["checkout"]})
		s.reference = "intents/" + a.project + "/" + s.token + ".json"
	}
	if err := w.sessionSeedSnapshot(); err != nil {
		return err
	}
	s.seedID = strings.Repeat("a", 64)
	container := map[string]any{"Id": s.seedID, "Name": "/itos-container-" + s.token, "Image": w.doctor.imageID, "Config": map[string]any{"Labels": w.sessionLabels()}, "State": map[string]any{"Status": "created", "Running": false, "Pid": float64(0), "StartedAt": "0001-01-01T00:00:00Z"}}
	s.api.local[s.seedID] = container
	s.api.observed[s.seedID] = approvalClone(container)
	record := w.sessionExpectedRecord("owned", s.seedID)
	if mode == "reconcile-absent" || mode == "reconcile-owned" {
		record["stage"], record["container_id"] = "intent", ""
	}
	if mode == "identity" {
		switch condition {
		case "selected daemon ID differs":
			w.doctor.engine.info["ID"] = "other-daemon"
		case "selected socket is replaced before removal":
			s.inspection = "replace socket"
		case "recorded ID has a different name":
			container["Name"] = "/foreign"
		case "matching name has a different image config ID":
			container["Image"] = "sha256:" + strings.Repeat("f", 64)
		case "matching name has a different intent label":
			object(object(container["Config"])["Labels"])["io.itos-container.intent"] = "foreign"
		case "name inspection and ID inspection disagree":
			record["stage"], record["container_id"] = "intent", ""
			s.inspection = "different full container ID"
		case "matching resource is already running":
			sessionMutateInspection(container, "running state")
		case "matching resource has exited after starting":
			sessionMutateInspection(container, "exited state after starting")
		case "created resource has a nonzero PID":
			sessionMutateInspection(container, "created state with nonzero PID")
		case "created resource has a set StartedAt":
			sessionMutateInspection(container, "created state with StartedAt")
		case "matching name and labels without a trusted record", "record UID differs from the current host UID":
		default:
			return fmt.Errorf("unknown identity fixture %q", condition)
		}
	}
	if mode == "storage" {
		sessionMutateInspection(container, condition)
	}
	if mode == "pre-remove" {
		s.inspection = condition
	}
	if mode == "already-absent" {
		delete(s.api.local, s.seedID)
		w.sessionReplacement()
	}
	if mode != "identity" || condition != "matching name and labels without a trusted record" {
		if err := writeManifestFixture(w.sessionRecordPath(), approvalJSON(record), 0600); err != nil {
			return err
		}
	}
	if mode == "evidence" {
		if err := w.sessionEvidenceDefect(condition, record); err != nil {
			return err
		}
	}
	if mode == "changed" {
		if err := w.sessionChange(condition); err != nil {
			return err
		}
	}
	return nil
}
func (w *world) sessionReplacement() {
	c := map[string]any{"Id": strings.Repeat("f", 64), "Name": "/itos-container-" + w.session.token, "Image": "foreign", "Config": map[string]any{"Labels": map[string]any{}}, "State": map[string]any{"Status": "created", "Running": false, "Pid": float64(0), "StartedAt": ""}}
	w.session.api.local[strings.Repeat("f", 64)] = c
}
func (w *world) sessionEvidenceDefect(defect string, record map[string]any) error {
	path := w.sessionRecordPath()
	data := approvalJSON(record)
	switch defect {
	case "intents directory symlink":
		dir := filepath.Dir(filepath.Dir(path))
		outside := filepath.Join(w.doctor.host, "outside-intents")
		if err := os.Rename(dir, outside); err != nil {
			return err
		}
		return os.Symlink(outside, dir)
	case "project directory mode 0777":
		return os.Chmod(filepath.Dir(path), 0777)
	case "record symlink":
		outside := filepath.Join(w.doctor.host, "outside-record")
		if err := os.Rename(path, outside); err != nil {
			return err
		}
		return os.Symlink(outside, path)
	case "record FIFO":
		if err := os.Remove(path); err != nil {
			return err
		}
		return approvalFIFO(path)
	case "record with another hard link":
		return os.Link(path, filepath.Join(w.doctor.host, "outside-link"))
	case "record mode 0644":
		return os.Chmod(path, 0644)
	case "truncated record":
		data = []byte(`{"schema":`)
	case "duplicate container_id field":
		data = append([]byte(`{"container_id":"",`), data[1:]...)
	case "unsupported schema":
		record["schema"] = 2
		data = approvalJSON(record)
	case "intent stage with a container ID":
		record["stage"] = "intent"
		data = approvalJSON(record)
	case "owned stage without a container ID", "absent stage without a container ID":
		record["stage"] = strings.Fields(defect)[0]
		record["container_id"] = ""
		data = approvalJSON(record)
	case "unsupported stage":
		record["stage"] = "running"
		data = approvalJSON(record)
	case "snapshot fingerprint differs":
		record["fingerprint"] = "sha256:" + strings.Repeat("e", 64)
		data = approvalJSON(record)
	case "name differs from filename token":
		record["name"] = "itos-container-" + strings.Repeat("e", 64)
		data = approvalJSON(record)
	case "referenced frozen snapshot missing":
		return os.Remove(w.sessionSnapshotPath())
	case "referenced frozen snapshot symlink":
		outside := filepath.Join(w.doctor.host, "outside-snapshot")
		if err := os.Rename(w.sessionSnapshotPath(), outside); err != nil {
			return err
		}
		return os.Symlink(outside, w.sessionSnapshotPath())
	default:
		return fmt.Errorf("unknown evidence defect %q", defect)
	}
	return os.WriteFile(path, data, 0600)
}
func (w *world) sessionChange(change string) error {
	switch change {
	case "policy now requests forbidden online networking", "checkout policy is now invalid":
		return os.WriteFile(filepath.Join(w.doctor.checkout, "itos-container.yaml"), []byte("version: 1\nnetwork: {mode: online}\n"), 0600)
	case "checkout manifest is now invalid":
		return os.WriteFile(filepath.Join(w.doctor.checkout, ".devcontainer/devcontainer.json"), []byte("{bad"), 0600)
	case "manifest now selects B's cached image":
		m := approvalClone(w.doctor.manifest)
		m["image"] = "fixture.test/other@sha256:" + strings.Repeat("b", 64)
		return os.WriteFile(filepath.Join(w.doctor.checkout, ".devcontainer/devcontainer.json"), approvalJSON(m), 0600)
	case "current approval is missing":
		return os.Remove(w.approval.record)
	case "current approval is corrupt":
		return os.WriteFile(w.approval.record, []byte("{bad"), 0600)
	case "current approval is replaced by B":
		a := w.approval
		settings := approvalClone(a.settings)
		image := "fixture.test/other@sha256:" + strings.Repeat("b", 64)
		object(settings["image"])["reference"], object(settings["image"])["id"] = image, "sha256:"+strings.Repeat("b", 64)
		object(settings["manifest"])["image"] = image
		return os.WriteFile(a.record, approvalJSON(map[string]any{"schema": 1, "project": a.project, "generation": "2", "fingerprint": "sha256:" + approvalHash(settings), "settings": settings}), 0600)
	default:
		return fmt.Errorf("unknown current input change %q", change)
	}
}

func (w *world) sessionLocalRun(action string) error {
	return w.sessionEach(func(w *world) error {
		s := w.session
		err := w.sessionLocalAction(action)
		if err != nil {
			s.fixtureError = err
		}
		return nil // missing effects are asserted in Then, never a setup success oracle
	})
}
func (w *world) sessionCaptureBefore() error {
	var err error
	w.session.before, err = w.sessionProtected()
	if err != nil {
		return err
	}
	w.session.resourcesBefore = map[string]string{}
	for id, c := range w.session.api.local {
		w.session.resourcesBefore[id] = string(approvalJSON(c))
	}
	// Unsafe paths are captured without opening them, so FIFOs cannot hang setup.
	info, e := os.Lstat(w.sessionRecordPath())
	if e == nil && info.Mode().IsRegular() {
		w.session.initial, err = sessionReadRecord(w.sessionRecordPath())
	}
	return err
}
func (w *world) sessionCaptureAfter() error {
	var err error
	w.session.after, err = w.sessionProtected()
	if err != nil {
		return err
	}
	info, e := os.Lstat(w.sessionRecordPath())
	if e == nil && info.Mode().IsRegular() {
		w.session.final, err = sessionReadRecord(w.sessionRecordPath())
	}
	w.session.resourcesAfter = map[string]string{}
	for id, c := range w.session.api.local {
		w.session.resourcesAfter[id] = string(approvalJSON(c))
	}
	return err
}
func (w *world) sessionLocalAction(action string) error {
	s := w.session
	var err error
	if s.mode == "recovery" {
		if err = w.sessionCaptureBefore(); err != nil {
			return err
		}
		s.cleanupStart = len(s.api.requests)
		s.cleanup, err = w.sessionProbe("cleanup", s.reference, nil)
	} else {
		s.create, err = w.sessionProbe("create", "", func() error {
			if s.change != "" {
				if err := w.sessionChange(s.change); err != nil {
					return err
				}
			}
			return w.sessionCaptureBefore()
		})
		if err == nil {
			s.owned, err = sessionReadRecord(w.sessionRecordPath())
		}
		if err == nil && action != "create" {
			if action == "reconstruct" {
				s.fault = ""
			}
			s.cleanupStart = len(s.api.requests)
			s.cleanup, err = w.sessionProbe("cleanup", s.reference, nil)
			s.create.Cleanup = s.cleanup.Outcome
		}
	}
	if err != nil {
		return err
	}
	if err = w.sessionCaptureAfter(); err != nil {
		return err
	}
	if action == "retry" || action == "repeat" {
		s.firstResult, s.firstRecord = s.cleanup, s.final
		s.firstRequests = append([]sessionRequest{}, s.api.requests[s.cleanupStart:]...)
		s.firstResources = map[string]map[string]any{}
		for id, c := range s.api.local {
			s.firstResources[id] = approvalClone(c)
		}
		s.fault, s.following = "", ""
		if s.mode == "recovery" && s.api.local[s.seedID] == nil {
			w.sessionReplacement()
		}
		s.cleanupStart = len(s.api.requests)
		s.cleanup, err = w.sessionProbe("cleanup", s.reference, nil)
		if err != nil {
			return err
		}
		if err = w.sessionCaptureAfter(); err != nil {
			return err
		}
	}
	if action == "two" {
		original := *s
		original.api = &sessionAPI{w: s.api.w, server: s.api.server,
			requests: append([]sessionRequest{}, s.api.requests...), created: append([]string{}, s.api.created...),
			observed: s.api.observed, local: s.api.local, absent: s.api.absent, problem: s.api.problem}
		var token [32]byte
		if _, err = rand.Read(token[:]); err != nil {
			return err
		}
		s.token = hex.EncodeToString(token[:])
		s.reference = "intents/" + w.approval.project + "/" + s.token + ".json"
		s.api.created = nil
		s.api.requests = nil
		if err = os.WriteFile(s.journal, nil, 0600); err != nil {
			return err
		}
		if err = w.sessionLocalAction("cycle"); err != nil {
			return err
		}
		second := *s
		*s = original
		s.second = &second
	}
	return nil
}

func sessionMutateInspection(c map[string]any, condition string) {
	switch condition {
	case "mounted volume":
		c["Mounts"] = []any{map[string]any{"Type": "volume", "Name": "unexpected", "Destination": "/data"}}
	case "configured volume":
		object(c["Config"])["Volumes"] = map[string]any{"/data": map[string]any{}}
	case "different full container ID":
		c["Id"] = strings.Repeat("e", 64)
	case "different name":
		c["Name"] = "/foreign"
	case "different image config ID":
		c["Image"] = "sha256:" + strings.Repeat("e", 64)
	case "different intent label", "different project label", "different fingerprint label":
		object(object(c["Config"])["Labels"])["io.itos-container."+strings.Fields(condition)[1]] = "foreign"
	case "running state":
		object(c["State"])["Status"] = "running"
		object(c["State"])["Running"] = true
	case "exited state after starting":
		object(c["State"])["Status"] = "exited"
		object(c["State"])["StartedAt"] = "2026-01-01T00:00:00Z"
	case "created state with nonzero PID":
		object(c["State"])["Pid"] = float64(1)
	case "created state with StartedAt":
		object(c["State"])["StartedAt"] = "2026-01-01T00:00:00Z"
	case "missing State":
		delete(c, "State")
	}
}

func (w *world) sessionRetained() error {
	s := w.session
	if !reflect.DeepEqual(s.resourcesBefore, s.resourcesAfter) {
		return fmt.Errorf("candidate resources changed")
	}
	if s.initial != s.final {
		return fmt.Errorf("ownership evidence changed")
	}
	return w.sessionUnrelated()
}
