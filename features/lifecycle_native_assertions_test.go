package features

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

func (w *world) lifecycleResult(code int) error {
	l := w.lifecycle
	if l != nil && l.err != nil {
		return l.err
	}
	if l == nil || l.run == nil {
		return errors.New("no lifecycle invocation was observed")
	}
	if l.run.exit != code {
		return fmt.Errorf("lifecycle run returned %d, expected %d; stderr %q", l.run.exit, code, l.run.errout.bytes())
	}
	return nil
}

func (w *world) lifecycleReceipts(signal string, child int) error {
	if err := w.lifecycleResult(child); err != nil {
		return err
	}
	l := w.lifecycle
	data, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-12-receipts"))
	if err != nil || string(data) != signal+"\n" || l.ready.IsZero() || !l.signalled.After(l.ready) {
		return errors.New("native worker did not record exactly the first signal after readiness")
	}
	for _, e := range l.journalAtReturn {
		if strings.HasSuffix(e.Path, "/wait") && e.Status == 200 && !e.Cancelled && !e.Replied.IsZero() &&
			e.Observation["StatusCode"] == float64(child) {
			return nil
		}
	}
	return errors.New("exit code was not backed by an authoritative native wait reply")
}

func (w *world) lifecycleCancelled() error {
	l := w.lifecycle
	if l.run == nil || l.err != nil {
		return errors.Join(l.err, errors.New("no successful lifecycle observation"))
	}
	diagnostic := string(l.run.errout.bytes())
	if !strings.Contains(diagnostic, "run.cancelled") || !strings.Contains(diagnostic, l.first) || l.first == "" {
		return fmt.Errorf("cancellation is not reported separately with its first signal: %q", diagnostic)
	}
	return w.lifecycleSafeState()
}

func lifecycleSignalRequests(entries []offlineRequest) []offlineRequest {
	var signals []offlineRequest
	for _, e := range entries {
		if e.Method == "POST" && (strings.HasSuffix(e.Path, "/kill") || strings.HasSuffix(e.Path, "/stop")) {
			signals = append(signals, e)
		}
	}
	return signals
}

func (w *world) lifecycleSignals(escalate bool) error {
	l := w.lifecycle
	entries := lifecycleSignalRequests(l.journalAtReturn)
	want := 1
	if escalate {
		want = 2
	}
	if len(entries) != want {
		return fmt.Errorf("signal request count = %d, expected %d", len(entries), want)
	}
	for i, e := range entries {
		query, err := url.ParseQuery(e.Query)
		signal := l.first
		if i == 1 {
			signal = "SIGKILL"
		}
		if err != nil || e.Path != "/containers/"+w.recovery.worker+"/kill" || len(query) != 1 || query.Get("signal") != signal {
			return errors.New("signal did not target the original full ID with the literal first signal/escalation")
		}
		if e.At.Before(l.signalled) {
			return errors.New("worker signal preceded controller cancellation")
		}
		if i == 1 && (e.At.Sub(l.signalled) < engineGraceBudget || e.At.Sub(l.signalled) > engineGraceBudget+engineGraceBudget/5) {
			return fmt.Errorf("SIGKILL at %s, expected original %s grace (at most %s scheduling tolerance)", e.At.Sub(l.signalled), engineGraceBudget, engineGraceBudget/5)
		}
	}
	if escalate && !l.aliveBeforeGrace {
		return errors.New("independent native observation did not find the primary alive before grace expired")
	}
	if l.delay > 0 && entries[0].Replied.Sub(entries[0].At) < l.delay {
		return errors.New("signal acknowledgement delay did not execute")
	}
	return errors.Join(w.lifecycleRevalidated(l.journalAtReturn), w.lifecycleNoHostSignals())
}

func (w *world) lifecycleNoHostSignals() error {
	data, err := os.ReadFile(w.lifecycle.signalTrace)
	if err != nil {
		return err
	}
	// Go's runtime uses SIGURG for asynchronous preemption. Any controller
	// syscall issuing INT/TERM/KILL to a host PID/group is a forbidden action;
	// observer signal-forward entries describe delivery TO the controller.
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "signal-forward") {
			continue
		}
		for _, ending := range []string{", 2)", ", 9)", ", 15)"} {
			if strings.Contains(line, ending) {
				return errors.New("controller attempted a host-process signal")
			}
		}
	}
	return nil
}

func (w *world) lifecycleRevalidated(entries []offlineRequest) error {
	if err := w.anchorRevalidated(entries); err != nil {
		return err
	}
	worker := w.recovery.worker
	inspected, checked := false, false
	for _, e := range entries {
		if e.Method == "GET" && e.Path == "/info" && e.Status == 200 {
			checked = true
		}
		if e.Method == "GET" && e.Path == "/containers/"+worker+"/json" && e.Status == 200 && e.Observation["Id"] == worker {
			inspected = true
		}
		if e.Method == "DELETE" && strings.HasPrefix(e.Path, "/volumes/") {
			// The resource-specific oracle above checks the complete volume tuple.
			inspected, checked = false, false
			continue
		}
		mutation := e.Method == "DELETE" || (e.Method == "POST" && (strings.HasSuffix(e.Path, "/kill") || strings.HasSuffix(e.Path, "/stop")))
		if !mutation {
			continue
		}
		var record map[string]any
		if !checked || !inspected || !offlineHex(worker) || !strings.HasPrefix(e.Path, "/containers/"+worker) ||
			json.Unmarshal([]byte(e.Record.Bytes), &record) != nil || record["container_id"] != worker ||
			record["project"] != w.approval.project || e.Record.Mode != 0600 ||
			w.offline.api.identity(e.Observation, worker) != nil {
			return errors.New("mutation lacked fresh daemon, protected record and full-ID revalidation")
		}
		inspected, checked = false, false
	}
	return nil
}

func (w *world) lifecycleBound(origin time.Time) error {
	l := w.lifecycle
	if origin.IsZero() || l.returned.IsZero() {
		return errors.Join(l.err, errors.New("missing lifecycle teardown start or return observation"))
	}
	if l.returned.Before(origin) || l.returned.Sub(origin) > 32*time.Second {
		return fmt.Errorf("controller teardown elapsed %s; required at most 30 s plus 2 s scheduling tolerance", l.returned.Sub(origin))
	}
	return l.err
}

func (w *world) lifecycleComplete() error {
	r, l := w.recovery, w.lifecycle
	if l.err != nil {
		return l.err
	}
	completing := l.run
	if l.mode == "crash" || l.mode == "disconnect" || l.mode == "cleanup-signals" {
		if len(r.stops) != 1 || r.stops[0].err != nil || r.stops[0].exit != 0 {
			return errors.New("fresh public stop did not confirm cleanup successfully")
		}
		completing = r.stops[0]
	}
	if !offlineHex(r.worker) || w.doctor.real.read("/containers/"+r.worker+"/json", nil, 404) != nil {
		return errors.New("independent observer did not confirm the original worker absent before harness cleanup")
	}
	s, err := w.recoveryEntry(r.after, "stopped", "absent", "complete")
	if err != nil {
		return err
	}
	if s.Retry != nil || s.StopCandidate || s.Anchor != "absent" {
		return errors.New("confirmed cleanup retained a pending recovery retry")
	}
	processes := append([]*recoveryProcess{l.run}, r.stops...)
	state := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")
	if err := recoveryDurableFile(filepath.Join(state, filepath.FromSlash(w.session.reference)), w.sessionExpectedRecord("absent", r.worker), processes); err != nil {
		return err
	}
	path := filepath.Join(state, "sessions", w.recoveryIndexKey(), r.id, "record.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var record map[string]any
	if json.Unmarshal(data, &record) != nil || record["id"] != r.id || record["intent"] != w.session.reference ||
		record["project"] != w.approval.project || record["state"] != "stopped" {
		return errors.New("completion record lost the original session/intent binding")
	}
	if err := recoveryDurableFile(path, record, processes); err != nil {
		return err
	}
	a := w.offline.api
	a.mu.Lock()
	entries := append([]offlineRequest(nil), a.requests...)
	a.mu.Unlock()
	removed, confirmed := false, false
	for _, e := range entries {
		if e.Method == "DELETE" && e.Path == "/containers/"+r.worker && e.Status == 204 {
			removed = true
		}
		if removed && e.Method == "GET" && e.Path == "/containers/"+r.worker+"/json" && e.Status == 404 &&
			!e.Cancelled && !e.Replied.IsZero() && !e.At.Before(completing.started) && e.Replied.Before(completing.finished) {
			confirmed = true
		}
	}
	if !removed || !confirmed {
		return errors.New("product did not remove and freshly confirm absence of the original full ID")
	}
	return errors.Join(w.lifecycleRevalidated(entries), w.anchorReleased(entries))
}

func (w *world) lifecycleRetained() error {
	l := w.lifecycle
	var record map[string]any
	if json.Unmarshal([]byte(l.firstRecord.Bytes), &record) != nil || l.firstRecord.Mode != 0600 ||
		record["id"] != w.recovery.id || record["intent"] != w.session.reference || record["state"] == "stopped" {
		return errors.New("first invocation lost its protected recovery identity or claimed completed cleanup")
	}
	if l.mode == "disconnect" && record["workload_exit"] != nil {
		return errors.New("transport loss invented an observed workload exit")
	}
	stage, id := "owned", w.recovery.worker
	if l.barrier == "after accepted create before reply" {
		stage, id = "intent", ""
	}
	if err := w.sessionExactRecord(l.firstIntent, stage, id); err != nil {
		return err
	}
	return w.lifecycleSafeState()
}

func (w *world) lifecycleSafeState() error {
	state := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")
	return filepath.WalkDir(state, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, marker := range []string{"SLICE_12_SYNTHETIC_", "/workspace/slice-12-probe", "slice-12:final"} {
			if bytes.Contains(data, []byte(marker)) {
				return errors.New("protected state retained raw workload argv/output or transport payload")
			}
		}
		return nil
	})
}

func (w *world) lifecyclePreserved() error {
	r := w.recovery
	current, err := w.lifecycleProtected()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(r.before, current) {
		return errors.New("lifecycle changed unrelated checkout files, approval or host state")
	}
	prestart := w.lifecycle.barrier == "after accepted create before reply"
	data, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-12-worker-edit"))
	if prestart {
		if !errors.Is(err, os.ErrNotExist) {
			return errors.New("pre-start interruption executed a worker instruction")
		}
	} else if err != nil || string(data) != "slice-12 retained edit" {
		return errors.New("lifecycle lost the worker checkout edit")
	}
	path := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "snapshots", w.approval.project, "1.json")
	snapshot, err := sessionReadRecord(path)
	if err != nil || snapshot != r.snapshot {
		return errors.New("lifecycle changed the original frozen snapshot")
	}
	resources, err := w.doctor.real.snapshotResources()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(recoveryWithoutWorkers(resources, []string{r.worker}), recoveryWithoutWorkers(w.offline.resourcesBefore, []string{r.worker})) {
		return errors.New("lifecycle changed unrelated native resources or image references")
	}
	return w.lifecycleSafeState()
}

func (w *world) lifecycleOneLaunch() error {
	a, r := w.offline.api, w.recovery
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := w.anchorRevalidated(a.requests); err != nil {
		return err
	}
	creates, starts, removes := 0, 0, 0
	for _, e := range a.requests {
		if e.Status == 403 {
			return errors.New("recorder refused an unsafe product request")
		}
		if e.Method == "POST" && e.Path == "/containers/create" {
			creates++
		}
		if e.Method == "POST" && strings.HasSuffix(e.Path, "/start") {
			starts++
		}
		if e.Method == "DELETE" {
			if strings.HasPrefix(e.Path, "/volumes/") {
				continue // Fully checked by anchorRevalidated, not a worker removal.
			}
			if e.Path != "/containers/"+r.worker {
				return errors.New("removal targeted a name or unrelated worker")
			}
			if e.Status == 204 {
				removes++
			}
		}
	}
	wantStarts := 1
	if w.lifecycle.barrier == "after accepted create before reply" {
		wantStarts = 0
	}
	if creates != 1 || starts != wantStarts || removes != 1 || len(a.created) != 1 {
		return fmt.Errorf("create/start/successful remove = %d/%d/%d; expected 1/%d/1", creates, starts, removes, wantStarts)
	}
	return a.problem
}

func (w *world) lifecycleBoundary() error {
	l := w.lifecycle
	if !l.survivedLoss || len(l.initialBoundary) == 0 || len(l.lostBoundary) == 0 {
		return errors.New("independent observations did not find the same worker alive after loss")
	}
	for _, key := range []string{"Id", "Image", "Name", "Config", "HostConfig", "Mounts", "NetworkSettings"} {
		if !reflect.DeepEqual(l.initialBoundary[key], l.lostBoundary[key]) {
			return fmt.Errorf("native %s changed across controller transport/liveness loss", key)
		}
	}
	if !w.offline.api.safeObserved(l.lostBoundary) {
		return errors.New("surviving worker no longer has the frozen offline boundary")
	}
	return isolationNetworkBoundary(l.lostBoundary)
}
