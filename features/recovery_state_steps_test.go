package features

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

func (w *world) recoveryStateSteps(sc *godog.ScenarioContext) {
	w.recoveryCheckoutSteps(sc)
	sc.Step(`^a recovery fixture with a safe checkout and no host state directory$`, func() error {
		if err := w.approvalCheckout(); err != nil {
			return err
		}
		w.recoveryLocal = &recoveryLocal{}
		if err := os.RemoveAll(w.doctor.env["XDG_STATE_HOME"]); err != nil {
			return err
		}
		return w.recoveryCapture()
	})
	sc.Step(`^recovery status is invoked in plain and JSON modes through each local backend$`, func() error {
		if err := w.recoveryInvocation("status"); err != nil {
			return err
		}
		return w.recoveryInvocation("status", "--json")
	})
	sc.Step(`^every status succeeds with an empty owned-session list and no problems$`, func() error {
		if len(w.recoveryLocal.results) != 2 || w.recoveryLocal.results[0].exit != 0 || !strings.Contains(strings.ToLower(w.recoveryLocal.results[0].stdout), "no owned sessions") {
			return fmt.Errorf("empty status did not succeed with an explicit empty list")
		}
		r, err := recoveryLocalReport(w.recoveryLocal.results[1], 0)
		if err != nil {
			return err
		}
		if len(r.Sessions) != 0 {
			return fmt.Errorf("status invented sessions")
		}
		return nil
	})
	sc.Step(`^no recovery invocation creates state, connects to an engine or executes a checkout helper$`, func() error {
		if w.doctor.engine.listener.connections.Load() != 0 {
			return fmt.Errorf("empty status contacted engine")
		}
		return w.recoveryUnchanged()
	})
	sc.Step(`^a recovery fixture with recorded phase "([^"]*)" resource "([^"]*)" and observed workload exit "([^"]*)"$`, w.recoverySeed)
	sc.Step(`^recovery status is invoked with --json, inherited JSON and inherited JSON overridden by --no-json$`, func() error {
		if err := w.recoveryInvocation("status", "--json"); err != nil {
			return err
		}
		w.doctor.env["ITOS_JSON"] = "1"
		if err := w.recoveryInvocation("status"); err != nil {
			return err
		}
		return w.recoveryInvocation("status", "--no-json")
	})
	sc.Step(`^every status reports state "([^"]*)" resource "([^"]*)" cleanup "([^"]*)" and workload exit "([^"]*)"$`, func(state, resource, cleanup, exit string) error {
		for _, r := range w.recoveryLocal.results[:2] {
			e, err := w.recoveryLocalEntry(r, state, resource, cleanup, exit)
			if err != nil {
				return err
			}
			if e.Controller != "absent" {
				return fmt.Errorf("unheld controller lease was not absent")
			}
		}
		return nil
	})
	sc.Step(`^JSON status has the specified schema and original project and fingerprint while plain status reports the same facts$`, func() error {
		l := w.recoveryLocal
		r := l.results[2]
		if r.exit != 0 || json.Valid([]byte(r.stdout)) {
			return fmt.Errorf("--no-json did not override inherited JSON")
		}
		report, err := recoveryLocalReport(l.results[0], 0)
		if err != nil {
			return err
		}
		if len(report.Sessions) != 1 {
			return fmt.Errorf("status did not report exactly one session")
		}
		for _, fact := range []string{l.id, w.approval.project, w.approval.fingerprint, report.Sessions[0].State, l.resource, l.exit} {
			if !strings.Contains(r.stdout, fact) {
				return fmt.Errorf("plain status omitted %s", fact)
			}
		}
		return nil
	})
	sc.Step(`^recovery inspection leaves every record and resource unchanged$`, w.recoveryUnchanged)
	sc.Step(`^a recovery fixture with an owned "([^"]*)" worker and observed workload exit "([^"]*)"$`, func(resource, exit string) error {
		phase := "failed"
		if resource == "durable absent" {
			phase = "stopped"
		}
		return w.recoverySeed(phase, resource, exit)
	})
	sc.Step(`^stop is invoked twice and status follows through each local backend$`, func() error {
		for _, command := range []string{"stop", "stop", "status"} {
			args := []string{command, "--json"}
			if command == "stop" {
				args = append(args, w.recoveryLocal.id)
			}
			if err := w.recoveryTracedInvocation(args...); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^both stops return zero with cleanup complete and workload exit "([^"]*)" after confirmed full-ID absence and durable completion$`, func(exit string) error {
		for _, r := range w.recoveryLocal.results {
			if _, err := w.recoveryLocalEntry(r, "stopped", "absent", "complete", exit); err != nil {
				return err
			}
		}
		return w.recoveryLocalDurable()
	})
	sc.Step(`^running-worker stop attempts send SIGTERM once and escalate only after the bounded grace and fresh ownership inspection$`, w.recoveryLocalSignals)
	sc.Step(`^the repeated stop performs no engine mutation or completed-record rewrite$`, func() error {
		l := w.recoveryLocal
		if len(l.processes) < 2 {
			return fmt.Errorf("no second stop")
		}
		for _, q := range w.session.api.requests {
			if q.Method == "DELETE" && strings.HasSuffix(q.Path, w.session.seedID) {
				if q.Query != "" && strings.Contains(q.Query, "v=true") {
					return fmt.Errorf("stop deleted volumes")
				}
			}
		}
		// Every second-process syscall is observed independently. A completion
		// re-sync uses the first process's final inode and bytes.
		path := w.recoveryRecordPath()
		info, err := sessionReadRecord(path)
		if err != nil {
			return err
		}
		syncs, err := recoveryReadSyncs(l.processes[1].journal)
		if err != nil {
			return err
		}
		for _, s := range syncs {
			if !s.Directory && strings.Contains(s.Path, "/sessions/") && s.Data != "" && s.Inode != info.Inode {
				return fmt.Errorf("repeated stop rewrote completion")
			}
		}
		return nil
	})
	sc.Step(`^only the originally owned worker is removed while checkout edits approvals snapshots and unrelated resources remain$`, w.recoveryImmutable)
	sc.Step(`^an independently owned recovery fixture whose current "([^"]*)" is "([^"]*)"$`, w.recoveryChangedInput)
	sc.Step(`^status and stop are invoked through each local backend$`, w.recoveryStatusStop)
	sc.Step(`^status reports the original fingerprint and stop completes using the original protected snapshot$`, func() error {
		if _, err := w.recoveryLocalEntry(w.recoveryLocal.results[0], "failed", "created", "pending", "unknown"); err != nil {
			return err
		}
		_, err := w.recoveryLocalEntry(w.recoveryLocal.results[1], "stopped", "absent", "complete", "unknown")
		return err
	})
	sc.Step(`^recovery neither parses the changed request nor prompts approves executes helpers or acquires images$`, func() error {
		if len(w.recoveryLocal.reads) != 0 {
			return fmt.Errorf("recovery opened mutable startup inputs")
		}
		for _, q := range w.session.api.requests {
			if strings.HasPrefix(q.Path, "/images/") || q.Path == "/containers/create" {
				return fmt.Errorf("recovery used launch preflight or acquisition")
			}
		}
		return w.recoveryImmutable()
	})
	sc.Step(`^a recovery fixture with a registered intent-only operation and a delayed create invisible by name$`, func() error {
		if err := w.recoverySeed("preparing", "created", "unknown"); err != nil {
			return err
		}
		w.recoveryLocal.mode = "delayed"
		if err := os.WriteFile(w.sessionRecordPath(), approvalJSON(w.sessionExpectedRecord("intent", "")), 0600); err != nil {
			return err
		}
		delete(w.session.api.local, w.session.seedID)
		return w.recoveryCapture()
	})
	sc.Step(`^status and stop run before visibility and a fresh stop retries after the fixture exposes the matching worker$`, func() error {
		if err := w.recoveryStatusStop(); err != nil {
			return err
		}
		w.session.api.mu.Lock()
		w.session.api.local[w.session.seedID] = approvalClone(w.session.api.observed[w.session.seedID])
		w.session.api.mu.Unlock()
		return w.recoveryTracedInvocation("stop", "--json", w.recoveryLocal.id)
	})
	sc.Step(`^the first status and stop report recovery.temporary with unknown resource and no absent or stopped claim$`, func() error {
		for _, r := range w.recoveryLocal.results[:2] {
			report, err := recoveryLocalReport(r, 75)
			if err != nil {
				return err
			}
			if len(report.Problems) != 1 || report.Problems[0].Rule != "recovery.temporary" || len(report.Sessions) != 1 || report.Sessions[0].Resource != "unknown" || report.Sessions[0].Cleanup != "unknown" {
				return fmt.Errorf("uncertain create became absence or lost typed problem")
			}
		}
		return nil
	})
	sc.Step(`^the later stop reconciles matching name and full ID and durably records ownership before removal and confirmed completion$`, func() error {
		if _, err := w.recoveryLocalEntry(w.recoveryLocal.results[2], "stopped", "absent", "complete", "unknown"); err != nil {
			return err
		}
		return w.recoveryLocalDurable()
	})
	sc.Step(`^no recovery attempt creates starts duplicates or adopts an unmatched worker$`, w.recoveryImmutable)
}

func (w *world) recoverySeed(phase, resource, exit string) error {
	if err := w.sessionFixture(false, "0022"); err != nil {
		return err
	}
	if err := w.sessionSeedRecovery("recovery", ""); err != nil {
		return err
	}
	l := &recoveryLocal{id: strings.Repeat("b", 64), phase: phase, resource: resource, exit: exit, ignoreTERM: resource == "running ignoring SIGTERM"}
	w.recoveryLocal = l
	w.recovery = &recoveryNative{mode: "local", id: l.id, worker: w.session.seedID}
	c := w.session.api.local[w.session.seedID]
	state := object(c["State"])
	switch resource {
	case "running", "running ignoring SIGTERM":
		state["Status"], state["Running"], state["Pid"], state["StartedAt"] = "running", true, float64(123), "2026-01-01T00:00:00Z"
	case "exited":
		state["Status"], state["StartedAt"] = "exited", "2026-01-01T00:00:00Z"
	case "absent", "already absent", "durable absent":
		delete(w.session.api.local, w.session.seedID)
	case "created":
	default:
		return fmt.Errorf("unknown recovery resource %s", resource)
	}
	if phase == "stopped" {
		if err := os.WriteFile(w.sessionRecordPath(), approvalJSON(w.sessionExpectedRecord("absent", w.session.seedID)), 0600); err != nil {
			return err
		}
	}
	var observed any
	if exit != "unknown" {
		n, err := strconv.Atoi(exit)
		if err != nil {
			return err
		}
		observed = n
	}
	record := map[string]any{"schema": 1, "id": l.id, "project": w.approval.project, "intent": w.session.reference, "state": phase, "workload_exit": observed, "last_problem": nil}
	if err := writeManifestFixture(w.recoveryRecordPath(), approvalJSON(record), 0600); err != nil {
		return err
	}
	for _, name := range []string{"controller.lock", "operation.lock"} {
		if err := os.WriteFile(filepath.Join(filepath.Dir(w.recoveryRecordPath()), name), nil, 0600); err != nil {
			return err
		}
	}
	w.sessionReplacement()
	return w.recoveryCapture()
}
func (w *world) recoveryRecordPath() string {
	return filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", w.recoveryIndexKey(), w.recoveryLocal.id, "record.json")
}
func (w *world) recoveryCapture() error {
	all := map[string]string{}
	for _, root := range []string{w.doctor.checkout, w.doctor.host} {
		tree, err := doctorTree(root)
		if err != nil {
			return err
		}
		for path, value := range tree {
			all[path] = value
		}
	}
	w.recoveryLocal.before = all
	w.recoveryLocal.immutable = map[string]string{}
	for path, value := range all {
		if w.recoveryLocal.runFixture && strings.HasPrefix(path, filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")) {
			rel, _ := filepath.Rel(filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container"), path)
			if rel == "sessions" || strings.HasPrefix(rel, "sessions/") || rel == "intents" || strings.HasPrefix(rel, "intents/") || rel == "snapshots" {
				continue
			}
		}
		if w.session != nil && (path == w.sessionRecordPath() || path == w.recoveryRecordPath()) {
			continue
		}
		w.recoveryLocal.immutable[path] = value
	}
	return nil
}
func (w *world) recoveryUnchanged() error {
	before := w.recoveryLocal.before
	if err := w.recoveryCapture(); err != nil {
		return err
	}
	if !reflect.DeepEqual(before, w.recoveryLocal.before) {
		return fmt.Errorf("read-only recovery changed protected or checkout state")
	}
	for _, q := range w.sessionRequests() {
		if q.Method != "GET" && q.Method != "HEAD" {
			return fmt.Errorf("status mutated an engine resource")
		}
	}
	return nil
}
func (w *world) sessionRequests() []sessionRequest {
	if w.session == nil {
		return nil
	}
	return w.session.api.requests
}
func (w *world) recoveryImmutable() error {
	before := w.recoveryLocal.immutable
	if err := w.recoveryCapture(); err != nil {
		return err
	}
	if !reflect.DeepEqual(before, w.recoveryLocal.immutable) {
		return fmt.Errorf("recovery changed unrelated state")
	}
	for _, q := range w.session.api.requests {
		if (!w.recoveryLocal.runFixture && (q.Path == "/containers/create" || strings.HasSuffix(q.Path, "/start"))) || (q.Method == "DELETE" && q.Path != "/containers/"+w.session.seedID) {
			return fmt.Errorf("recovery launched or removed another resource")
		}
	}
	if w.session.api.local[strings.Repeat("f", 64)] == nil {
		return fmt.Errorf("recovery removed same-named foreign replacement")
	}
	return nil
}
func recoveryLocalReport(r doctorResult, code int) (recoveryReport, error) {
	var report recoveryReport
	if r.exit != code || json.Unmarshal([]byte(r.stdout), &report) != nil || report.Schema != 1 || report.OK != (code == 0) || report.Problems == nil || report.Sessions == nil {
		return report, fmt.Errorf("recovery result did not satisfy schema-1 and exit %d: %+v", code, r)
	}
	if code == 0 && len(report.Problems) != 0 {
		return report, fmt.Errorf("successful recovery contains problems")
	}
	for _, p := range report.Problems {
		if p.Rule == "" || p.Exit == 0 || p.Message == "" || p.Fix == "" {
			return report, fmt.Errorf("incomplete recovery problem")
		}
	}
	return report, nil
}
func (w *world) recoveryLocalEntry(r doctorResult, state, resource, cleanup, exit string) (recoveryEntry, error) {
	report, err := recoveryLocalReport(r, 0)
	if err != nil {
		return recoveryEntry{}, err
	}
	if len(report.Sessions) != 1 {
		return recoveryEntry{}, fmt.Errorf("expected exactly one owned session")
	}
	e := report.Sessions[0]
	gotExit := "unknown"
	if e.WorkloadExit != nil {
		gotExit = strconv.Itoa(*e.WorkloadExit)
	}
	if e.ID != w.recoveryLocal.id || e.Project != w.approval.project || e.Fingerprint != w.approval.fingerprint || e.State != state || e.Resource != resource || e.Cleanup != cleanup || gotExit != exit {
		return e, fmt.Errorf("recovery facts differ: %+v exit %s; want %s/%s/%s/%s", e, gotExit, state, resource, cleanup, exit)
	}
	if cleanup == "complete" {
		if e.Retry != nil || e.StopCandidate {
			return e, fmt.Errorf("complete session retains retry")
		}
	} else if !reflect.DeepEqual(e.Retry, []string{"itos", "container", "stop", e.ID}) {
		return e, fmt.Errorf("pending session lacks exact retry argv")
	}
	return e, nil
}
func (w *world) recoveryTracedInvocation(args ...string) error {
	p := w.recoveryStart(args...)
	p.wait()
	if p.err != nil {
		return p.err
	}
	w.recoveryLocal.processes = append(w.recoveryLocal.processes, p)
	w.recoveryLocal.results = append(w.recoveryLocal.results, doctorResult{exit: p.exit, stdout: string(p.out.bytes()), stderr: string(p.errout.bytes())})
	return nil
}
func (w *world) recoveryLocalDurable() error {
	if w.session.api.local[w.session.seedID] != nil {
		return fmt.Errorf("worker remained after completed cleanup")
	}
	if err := recoveryDurableFile(w.sessionRecordPath(), w.sessionExpectedRecord("absent", w.session.seedID), w.recoveryLocal.processes); err != nil {
		return err
	}
	var exit any
	if w.recoveryLocal.exit != "unknown" {
		exit, _ = strconv.Atoi(w.recoveryLocal.exit)
	}
	want := map[string]any{"schema": 1, "id": w.recoveryLocal.id, "project": w.approval.project, "intent": w.session.reference, "state": "stopped", "workload_exit": exit, "last_problem": nil}
	return recoveryDurableFile(w.recoveryRecordPath(), want, w.recoveryLocal.processes)
}
func (w *world) recoveryLocalSignals() error {
	term, kill, remove := 0, 0, 0
	for _, q := range w.session.api.requests {
		if q.Method == "DELETE" {
			remove++
		}
		if strings.HasSuffix(q.Path, "/kill") {
			if strings.Contains(q.Query, "SIGTERM") {
				term++
			}
			if strings.Contains(q.Query, "SIGKILL") {
				kill++
			}
		}
	}
	running := strings.HasPrefix(w.recoveryLocal.resource, "running")
	if running && term != 1 || !running && term != 0 || (w.recoveryLocal.ignoreTERM && kill != 1) || (!w.recoveryLocal.ignoreTERM && kill != 0) || remove > 1 {
		return fmt.Errorf("incorrect termination sequence %d/%d/%d", term, kill, remove)
	}
	return nil
}
func (w *world) recoveryStatusStop() error {
	if err := w.recoveryInvocation("status", "--json"); err != nil {
		return err
	}
	if err := w.recoveryTracedInvocation("stop", "--json", w.recoveryLocal.id); err != nil {
		return err
	}
	if w.recoveryLocal.watch != nil {
		return w.recoveryFinishWatch()
	}
	return nil
}
func (w *world) recoveryChangedInput(input, change string) error {
	if err := w.recoverySeed("failed", "created", "unknown"); err != nil {
		return err
	}
	path := w.approval.record
	if input == "policy" {
		path = filepath.Join(w.doctor.checkout, "itos-container.yaml")
	}
	if input == "manifest" {
		path = filepath.Join(w.doctor.checkout, ".devcontainer/devcontainer.json")
	}
	if change == "missing" {
		if err := os.Remove(path); err != nil {
			return err
		}
	} else {
		data := []byte("{invalid-slice-11-request")
		if input == "approval" && change == "superseded" {
			if err := w.sessionChange("current approval is replaced by B"); err != nil {
				return err
			}
		} else if err := os.WriteFile(path, data, 0600); err != nil {
			return err
		}
	}
	if err := w.recoveryCapture(); err != nil {
		return err
	}
	fd, err := sessionInotifyInit()
	if err != nil {
		return err
	}
	watch := &sessionReadWatch{file: os.NewFile(uintptr(fd), "slice-11-inputs"), paths: map[int32]string{}}
	for _, p := range []string{filepath.Join(w.doctor.checkout, "itos-container.yaml"), filepath.Join(w.doctor.checkout, ".devcontainer/devcontainer.json"), w.approval.record} {
		if _, err := os.Lstat(p); os.IsNotExist(err) {
			continue
		}
		n, err := sessionInotifyAdd(fd, p)
		if err != nil {
			return err
		}
		watch.paths[int32(n)] = p
	}
	w.recoveryLocal.watch = watch
	w.doctor.cleanup = append(w.doctor.cleanup, func() { _ = watch.file.Close() })
	return nil
}
