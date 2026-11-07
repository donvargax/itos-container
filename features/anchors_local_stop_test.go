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

func (w *world) anchorRecoveryStopSteps(sc *godog.ScenarioContext) {
	sc.Step(`^an anchor recovery fixture with an owned "([^"]*)" worker volume "([^"]*)" observed exit "([^"]*)" and changed current "([^"]*)"$`, w.anchorRecoveryStopSeed)
	sc.Step(`^fresh anchor stop processes run twice and status follows through both local backends$`, w.anchorRecoveryStops)
	sc.Step(`^both stops confirm worker absence before anchor absence and durable completion with exit zero and workload exit "([^"]*)"$`, func(exit string) error {
		return errors.Join(w.anchorRecoveryStopResult(0, exit), w.anchorRecoveryStopResult(1, exit), w.anchorRecoveryReleaseOrder(), w.anchorRecoveryDurable(), w.anchorRecoveryMemoryResult())
	})
	sc.Step(`^running workers receive only the bounded owned termination sequence and anchor deletion never requests force or other volume removal$`, w.anchorRecoverySignals)
	sc.Step(`^the second stop reloads and syncs completion without another mutation completed-record rewrite or volume allocation$`, w.anchorRecoveryRepeated)
	sc.Step(`^status reports stopped resource absent anchor absent cleanup complete and retry null for that session$`, func() error { return w.anchorRecoveryStopResult(2, w.anchorLocalRecovery.exit) })
	sc.Step(`^anchor recovery preserves checkout edits approvals snapshots replacements and unrelated resources without reading current inputs$`, w.anchorRecoveryPreserved)
}

func (w *world) anchorRecoveryStopSeed(worker, volume, exit, input string) error {
	if worker == "recorded full ID absent" {
		worker = "absent"
	}
	if err := w.anchorRecoverySeed("failed", worker, volume, "pending", exit); err != nil {
		return err
	}
	a := w.anchorLocalRecovery
	a.mode = "stop"
	a.fault = input
	root := w.doctor.checkout
	w.recoveryLocal.invocationDir = w.dir
	w.doctor.env["ITOS_ROOT"] = root
	switch input {
	case "policy missing":
		if err := os.Remove(filepath.Join(root, "itos-container.yaml")); err != nil {
			return err
		}
	case "manifest invalid":
		if err := os.WriteFile(filepath.Join(root, ".devcontainer/devcontainer.json"), []byte("{invalid"), 0600); err != nil {
			return err
		}
	case "approval superseded":
		if err := w.sessionChange("current approval is replaced by B"); err != nil {
			return err
		}
	case "Git directory replaced":
		if err := os.Rename(filepath.Join(root, ".git"), filepath.Join(root, ".git.original")); err != nil {
			return err
		}
		if err := os.Mkdir(filepath.Join(root, ".git"), 0700); err != nil {
			return err
		}
	case "checkout path replaced":
		a.original = root + ".original"
		if err := os.Rename(root, a.original); err != nil {
			return err
		}
		if err := os.Mkdir(root, 0700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(root, "replacement"), []byte("untouched replacement"), 0600); err != nil {
			return err
		}
	case "checkout inaccessible":
	default:
		return fmt.Errorf("unknown anchor input change %s", input)
	}
	if err := w.recoveryCapture(); err != nil {
		return err
	}
	// Observe only product access after fixture substitutions are complete.
	if _, err := w.recoveryLocal.watch.finish(); err != nil {
		return err
	}
	if err := w.anchorRecoveryWatch(); err != nil {
		return err
	}
	if input == "checkout inaccessible" {
		if err := os.Chmod(root, 0); err != nil {
			return err
		}
		w.doctor.cleanup = append(w.doctor.cleanup, func() { _ = os.Chmod(root, 0700) })
	}
	return nil
}

func (w *world) anchorRecoveryWatch() error {
	fd, err := sessionInotifyInit()
	if err != nil {
		return err
	}
	watch := &sessionReadWatch{file: os.NewFile(uintptr(fd), "slice-17-anchor-inputs"), paths: map[int32]string{}}
	root := w.doctor.checkout
	if w.anchorLocalRecovery.original != "" {
		root = w.anchorLocalRecovery.original
	}
	for _, path := range []string{filepath.Join(root, "itos-container.yaml"), filepath.Join(root, ".devcontainer/devcontainer.json"), w.approval.record} {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			continue
		}
		id, err := sessionInotifyAdd(fd, path)
		if err != nil {
			_ = watch.file.Close()
			return err
		}
		watch.paths[int32(id)] = path
	}
	w.recoveryLocal.watch = watch
	w.doctor.cleanup = append(w.doctor.cleanup, func() { _ = watch.file.Close() })
	return nil
}

func (w *world) anchorRecoveryStops() error {
	a := w.anchorLocalRecovery
	for i := 0; i < 2; i++ {
		err := w.recoveryTracedInvocation("stop", "--json", w.recoveryLocal.id)
		a.problem = errors.Join(a.problem, err)
		if i == 0 {
			a.firstRequests = len(w.sessionRequests())
			a.firstRecords, err = doctorTree(filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container"))
			a.problem = errors.Join(a.problem, err)
		}
	}
	a.problem = errors.Join(a.problem, w.recoveryInvocation("status", "--json"))
	if a.fault == "checkout inaccessible" {
		info, err := os.Stat(w.doctor.checkout)
		if err != nil || info.Mode().Perm() != 0 {
			a.problem = errors.Join(a.problem, fmt.Errorf("recovery repaired inaccessible checkout: %v", err))
		}
		if err = os.Chmod(w.doctor.checkout, 0700); err != nil {
			a.problem = errors.Join(a.problem, err)
		}
	}
	return w.anchorRecoveryMemory()
}

func (w *world) anchorRecoveryEntryAt(index, code int) (map[string]any, error) {
	if w.anchorLocalRecovery.problem != nil {
		return nil, w.anchorLocalRecovery.problem
	}
	if index >= len(w.recoveryLocal.results) {
		return nil, fmt.Errorf("missing recovery process %d", index)
	}
	r := w.recoveryLocal.results[index]
	if _, err := recoveryLocalReport(r, code); err != nil {
		return nil, err
	}
	var report struct{ Sessions []map[string]any }
	if err := json.Unmarshal([]byte(r.stdout), &report); err != nil {
		return nil, err
	}
	for _, e := range report.Sessions {
		if e["id"] == w.recoveryLocal.id {
			return e, nil
		}
	}
	return nil, fmt.Errorf("anchor session omitted")
}

func (w *world) anchorRecoveryStopResult(index int, exit string) error {
	e, err := w.anchorRecoveryEntryAt(index, 0)
	if err != nil {
		return err
	}
	for key, want := range map[string]string{"state": "stopped", "resource": "absent", "anchor": "absent", "cleanup": "complete"} {
		if e[key] != want {
			return fmt.Errorf("stop %d %s = %v, want %s", index, key, e[key], want)
		}
	}
	if e["retry"] != nil || e["stop_candidate"] != false {
		return fmt.Errorf("completed cleanup retains retry")
	}
	if exit == "unknown" {
		if e["workload_exit"] != nil {
			return fmt.Errorf("stop fabricated workload exit")
		}
	} else {
		n, _ := strconv.Atoi(exit)
		if e["workload_exit"] != float64(n) {
			return fmt.Errorf("stop lost observed exit")
		}
	}
	return nil
}

func (w *world) anchorRecoveryReleaseOrder() error {
	a := w.anchorLocalRecovery
	confirmed, removed, released := false, false, false
	for _, q := range w.sessionRequests() {
		if q.Method == "DELETE" && q.Path == "/containers/"+w.session.seedID {
			removed = true
			confirmed = false
		}
		if q.Method == "GET" && q.Path == "/containers/"+w.session.seedID+"/json" && q.Status == 404 {
			confirmed = true
		}
		if q.Method == "DELETE" && strings.HasPrefix(q.Path, "/volumes/") {
			if !confirmed || q.Path != "/volumes/"+a.name || strings.Contains(q.Query, "force=true") {
				return fmt.Errorf("anchor released without full-ID absence or with foreign authority")
			}
			released = true
		}
	}
	if !confirmed || (a.worker != "absent" && !removed) || (a.volume == "present" && !released) {
		return fmt.Errorf("missing worker-before-anchor release: confirmed=%t removed=%t released=%t", confirmed, removed, released)
	}
	if w.session.api.volumes[a.name] != nil {
		return fmt.Errorf("owned anchor remains")
	}
	return nil
}
func (w *world) anchorRecoveryDurable() error {
	r := approvalClone(w.anchorLocalRecovery.intent)
	r["stage"] = "absent"
	object(r["anchor"])["stage"] = "absent"
	if err := recoveryDurableFile(w.sessionRecordPath(), r, w.recoveryLocal.processes); err != nil {
		return err
	}
	var exit any
	if w.anchorLocalRecovery.exit != "unknown" {
		exit, _ = strconv.Atoi(w.anchorLocalRecovery.exit)
	}
	p := map[string]any{"schema": 1, "id": w.recoveryLocal.id, "project": w.approval.project, "intent": w.session.reference, "state": "stopped", "workload_exit": exit, "last_problem": nil}
	return recoveryDurableFile(w.recoveryRecordPath(), p, w.recoveryLocal.processes)
}
func (w *world) anchorRecoverySignals() error {
	term, kill := 0, 0
	for _, q := range w.sessionRequests() {
		if strings.HasSuffix(q.Path, "/kill") {
			if q.Path != "/containers/"+w.session.seedID+"/kill" {
				return fmt.Errorf("foreign worker signalled")
			}
			if strings.Contains(q.Query, "SIGTERM") {
				term++
			}
			if strings.Contains(q.Query, "SIGKILL") {
				kill++
			}
		}
		if q.Method == "DELETE" && (strings.Contains(q.Query, "force=true") || strings.Contains(q.Query, "v=true")) {
			return fmt.Errorf("forced or recursive volume removal")
		}
	}
	running := strings.HasPrefix(w.anchorLocalRecovery.worker, "running")
	if (running && term != 1) || (!running && term != 0) || (w.recoveryLocal.ignoreTERM && kill != 1) || (!w.recoveryLocal.ignoreTERM && kill != 0) {
		return fmt.Errorf("wrong owned termination sequence %d/%d", term, kill)
	}
	return nil
}
func (w *world) anchorRecoveryRepeated() error {
	for _, q := range w.sessionRequests()[w.anchorLocalRecovery.firstRequests:] {
		if q.Method != "GET" && q.Method != "HEAD" {
			return fmt.Errorf("repeat mutated resources: %s %s", q.Method, q.Path)
		}
	}
	after, err := doctorTree(filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container"))
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(after, w.anchorLocalRecovery.firstRecords) {
		return fmt.Errorf("repeat rewrote completed records")
	}
	return w.anchorRecoveryDurable()
}
func (w *world) anchorRecoveryPreserved() error {
	if err := w.anchorRecoveryNoReads(); err != nil {
		return err
	}
	before := w.recoveryLocal.immutable
	if err := w.recoveryCapture(); err != nil {
		return err
	}
	if !reflect.DeepEqual(before, w.recoveryLocal.immutable) {
		return fmt.Errorf("anchor cleanup changed unrelated files")
	}
	a := w.anchorLocalRecovery
	if !reflect.DeepEqual(a.volumes["unrelated-anchor"], w.session.api.volumes["unrelated-anchor"]) {
		return fmt.Errorf("unrelated volume changed")
	}
	if w.session.api.local[strings.Repeat("f", 64)] == nil {
		return fmt.Errorf("unrelated worker removed")
	}
	for _, q := range w.sessionRequests() {
		if q.Method == "POST" && (q.Path == "/containers/create" || q.Path == "/volumes/create" || strings.HasSuffix(q.Path, "/start") || strings.Contains(q.Path, "/exec")) {
			return fmt.Errorf("recovery launched or allocated work")
		}
		if q.Method == "DELETE" && q.Path != "/containers/"+w.session.seedID && q.Path != "/volumes/"+a.name {
			return fmt.Errorf("foreign resource deleted")
		}
	}
	return nil
}
