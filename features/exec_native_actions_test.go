package features

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func (w *world) nativeReportAction(exit, seconds int) error {
	if err := w.nativePrimary(); err != nil {
		return err
	}
	n := w.exec.native
	var err error
	n.before, err = w.nativeRecords()
	if err != nil {
		return err
	}
	// These changes are only requests in disposable files. The live grant
	// remains generation one, including its original checkout and identity.
	manifest := approvalClone(w.doctor.manifest)
	manifest["containerUser"], manifest["remoteUser"] = "0", "0"
	manifest["workspaceFolder"] = "/"
	manifest["containerEnv"] = map[string]string{"APP": "changed"}
	if err = os.WriteFile(filepath.Join(w.doctor.checkout, ".devcontainer/devcontainer.json"), approvalJSON(manifest), 0600); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(w.doctor.checkout, "itos-container.yaml"), []byte(egressAllowlistPolicy([]string{"changed.example"}, "")), 0600); err != nil {
		return err
	}
	if err = w.execCurrentChange("replacement approval with wider permissions"); err != nil {
		return err
	}
	if err = w.nativeWrite("exit", strconv.Itoa(exit)); err != nil {
		return err
	}
	c, err := w.nativeClient("report", "report", w.exec.argv)
	if err != nil {
		return err
	}
	if err = nativeAwait(c.p, engineReadinessAllowance, func() bool {
		return json.Unmarshal(bytes.TrimSpace(c.p.out.bytes()), &n.receipt) == nil && n.receipt.Nonce == n.nonce &&
			string(c.p.errout.bytes()) == "live "+n.nonce+" report\n"
	}); err != nil {
		return fmt.Errorf("waiting for reporting exec: %w", err)
	}
	c.ready = time.Now()
	if _, err = c.input.Write(offlineInput); err != nil {
		return err
	}
	if err = c.input.Close(); err != nil {
		return err
	}
	if err = nativeAwait(c.p, engineReadinessAllowance, func() bool {
		data, err := os.ReadFile(w.nativePath("eof"))
		return err == nil && bytes.Equal(data, offlineInput)
	}); err != nil {
		return err
	}
	c.eof = time.Now()
	if !lifecycleUntil(c.p, c.eof.Add(time.Duration(seconds)*time.Second)) || !w.recoveryNativePresent(true) {
		return errors.New("exec or primary ended before release after stdin EOF")
	}
	if err = w.nativeWrite("finish-report", "release"); err != nil {
		return err
	}
	if err = nativeJoin(c.p, time.Now().Add(12*time.Second)); err != nil {
		return err
	}
	return nil
}

func (w *world) nativeSignalAction(signal string) error {
	control := w.recovery.second
	if err := control.nativePrimary(); err != nil {
		return fmt.Errorf("unrelated primary: %w", err)
	}
	var err error
	control.exec.native.before, err = control.nativeRecords()
	if err != nil {
		return err
	}
	if err = w.nativePrimary(); err != nil {
		return err
	}
	c, err := w.nativeWatch("first", "pulse")
	if err != nil {
		return err
	}
	buf, err := os.ReadFile(c.p.pid)
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(string(buf))
	if err != nil || pid <= 0 {
		return errors.New("trace did not identify the exec client")
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	defer func() { _ = process.Release() }()
	first, other := syscall.SIGINT, syscall.SIGTERM
	if signal == "SIGTERM" {
		first, other = other, first
	}
	n := w.exec.native
	n.event = time.Now()
	if err = process.Signal(first); err != nil {
		return err
	}
	for _, offset := range []time.Duration{engineGraceBudget / 4, engineGraceBudget / 2} {
		if !lifecycleUntil(c.p, n.event.Add(offset)) {
			return errors.New("exec client exited before the repeated signal")
		}
		if err = process.Signal(other); err != nil {
			return err
		}
	}
	return w.nativeCollectEnd()
}

func (w *world) nativeBreakAction(stream string) error {
	if err := w.nativePrimary(); err != nil {
		return err
	}
	mode := map[string]string{"exec stdout reader": "stdout", "exec stderr reader": "stderr", "exec attachment transport": "transport"}[stream]
	if mode == "" {
		return fmt.Errorf("unknown exec stream %q", stream)
	}
	c, err := w.nativeWatch("first", mode)
	if err != nil {
		return err
	}
	n := w.exec.native
	n.event = time.Now()
	if mode == "transport" {
		// Close only the reporting exec's upgraded upstream connection. Run's
		// attachment and the observer's independent engine client stay live.
		a := w.offline.api
		a.mu.Lock()
		attachment := n.attachments["/exec/"+nativeCreatedID(a.requests, c.argv, w.recovery.worker)+"/start"]
		a.mu.Unlock()
		if attachment == nil {
			return errors.New("ready exec has no upgraded attachment")
		}
		if err = attachment.Close(); err != nil {
			return err
		}
	} else if err = c.sink.Close(); err != nil {
		return err
	}
	if err = w.nativeWrite("first-write", "write"); err != nil {
		return err
	}
	return w.nativeCollectEnd()
}

func (w *world) nativeConcurrentEnd(event string) error {
	if err := w.nativePrimary(); err != nil {
		return err
	}
	if _, err := w.nativeWatch("first", "pulse"); err != nil {
		return err
	}
	c, err := w.nativeWatch("second", "blocked")
	if err != nil {
		return err
	}
	if err = w.nativeWrite("second-write", "write"); err != nil {
		return err
	}
	if err = nativeAwait(c.p, engineReadinessAllowance, func() bool { return lifecycleFullPipe(c.sink) }); err != nil {
		return fmt.Errorf("backpressure: %w", err)
	}
	n := w.exec.native
	n.event = time.Now()
	switch event {
	case "primary exits 17":
		err = w.nativeWrite("end-primary", "release")
	case "independent concurrent stop":
		n.stop = w.recoveryStart("stop", w.recovery.id)
	default:
		return fmt.Errorf("unknown end event %q", event)
	}
	if err != nil {
		return err
	}
	if err = w.nativeCollectEnd(); err != nil {
		return err
	}
	if n.stop != nil {
		if err = nativeJoin(n.stop, n.event.Add(30*time.Second)); err != nil {
			return err
		}
	}
	n.refused = w.nativeOnce("late")
	return nil
}

// Inspect worker absence while the exec clients may still be draining. A
// harness close of a blocked reader would mask a stuck drain, so keep it open.
func (w *world) nativeCollectEnd() error {
	n := w.exec.native
	deadline := n.event.Add(30 * time.Second)
	if err := nativeJoin(w.recovery.run, deadline); err != nil {
		return err
	}
	if err := w.nativeAbsent(); err != nil {
		return err
	}
	n.cleaned = time.Now()
	if n.cleaned.After(deadline) {
		return errors.New("owned cleanup exceeded thirty seconds")
	}
	for _, c := range n.clients {
		if err := nativeJoin(c.p, deadline); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) nativeRefusalAction(target string) error {
	other := w.recovery.second
	if err := other.nativePrimary(); err != nil {
		return err
	}
	if err := w.nativePrimary(); err != nil {
		return err
	}
	n := w.exec.native
	n.control = other.nativeOnce("control")
	if target == "stopped" {
		if err := w.nativeRelease(); err != nil {
			return err
		}
	}
	var err error
	n.before, err = w.nativeRecords()
	if err != nil {
		return err
	}
	n.event = time.Now()
	// Both invocations point at the target's original state and socket. The
	// foreign row changes only the caller's checkout, so it actually tests
	// project ownership even when the target is discoverable in that state.
	if target == "foreign" {
		n.refused = w.recoveryStartAt(other.doctor.checkout, 45*time.Second, w.bin, nil, "exec", w.recovery.id, "--", nativeExecProbe, "once", "refused")
		n.refused.wait()
	} else {
		n.refused = w.nativeOnce("refused")
	}
	n.after, err = w.nativeRecords()
	return err
}

func nativeCreatedID(entries []offlineRequest, argv []string, worker string) string {
	for _, q := range entries {
		if q.Method != "POST" || q.Path != "/containers/"+worker+"/exec" || q.Status != 201 {
			continue
		}
		args, _ := q.Body["Cmd"].([]any)
		if len(args) != len(argv) {
			continue
		}
		match := true
		for i, a := range argv {
			if args[i] != a {
				match = false
				break
			}
		}
		if match {
			id, _ := q.Observation["Id"].(string)
			return id
		}
	}
	return ""
}

func (w *world) nativeAbsent() error {
	id := w.recovery.worker
	if !offlineHex(id) {
		return errors.New("no owned full worker ID")
	}
	if err := w.doctor.real.read("/containers/"+id+"/json", nil, 404); err != nil {
		return fmt.Errorf("worker still present or unobservable: %w", err)
	}
	return w.doctor.real.read("/volumes/itos-container-anchor-"+w.session.token, nil, 404)
}

func (w *world) nativeEntries() []offlineRequest {
	a := w.offline.api
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.requests)
}

func (w *world) nativeObservedExit(c *nativeExecClient) (int, bool) {
	entries := w.nativeEntries()
	id := nativeCreatedID(entries, c.argv, w.recovery.worker)
	if !offlineHex(id) {
		return 0, false
	}
	for _, r := range entries {
		if r.Path != "/exec/"+id+"/json" || r.At.Before(c.ready) || r.Status != 200 || r.Observation["Running"] != false ||
			r.Observation["ID"] != id || r.Observation["ContainerID"] != w.recovery.worker {
			continue
		}
		if code, ok := r.Observation["ExitCode"].(float64); ok && !r.Cancelled && !r.Replied.IsZero() && !r.Replied.After(c.p.finished) {
			return int(code), true
		}
	}
	return 0, false
}

func (w *world) nativeStatus(state, resource, cleanup string) (recoveryEntry, error) {
	p := w.recoveryStart("status", "--json")
	return w.recoveryEntry(p, state, resource, cleanup)
}

func (w *world) nativeEdit() error {
	for path, want := range map[string]string{w.nativePath("edit"): w.exec.native.nonce, filepath.Join(w.doctor.host, "unrelated-state"): "retain unrelated host state", filepath.Join(w.doctor.checkout, "edit.txt"): "retain checkout edit"} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			return fmt.Errorf("checkout edit or unrelated state changed at %s", filepath.Base(path))
		}
	}
	return nil
}

func (w *world) nativeStopped() error {
	if _, err := w.nativeStatus("stopped", "absent", "complete"); err != nil {
		return err
	}
	return errors.Join(w.nativeAbsent(), w.nativeEdit(), w.nativeResourceInventory())
}

func (w *world) nativeResourceInventory() error {
	after, err := w.doctor.real.snapshotResources()
	if err != nil {
		return err
	}
	before := w.offline.resourcesBefore
	if other := w.recovery.second; other != nil {
		// The independent control started after the baseline and is still
		// running. Exclude only its positively identified anchor and worker.
		after, err = other.recoveryWithoutRetainedAnchor(after)
		if err != nil {
			return err
		}
		ids := []string{other.recovery.worker}
		after, before = recoveryWithoutWorkers(after, ids), recoveryWithoutWorkers(before, ids)
	}
	if !maps.Equal(before, after) {
		return errors.New("session cleanup left a resource or changed unrelated engine state")
	}
	return nil
}

func (w *world) nativeNoRestart() error {
	for _, q := range w.nativeEntries() {
		if q.At.Before(w.exec.native.cleaned) || q.Method == "GET" || q.Method == "HEAD" {
			continue
		}
		return fmt.Errorf("product attempted %s %s after cleanup", q.Method, q.Path)
	}
	return w.nativeStopped()
}

func (w *world) nativeTraceClean() error {
	entries := w.nativeEntries()
	created, started := 0, 0
	for _, q := range entries {
		if q.Method == "POST" && strings.HasSuffix(q.Path, "/exec") {
			created++
		}
		if q.Method == "POST" && strings.HasPrefix(q.Path, "/exec/") && strings.HasSuffix(q.Path, "/start") {
			started++
		}
	}
	if created != 1 || started != 1 {
		return fmt.Errorf("observed %d exec creates and %d starts", created, started)
	}
	return noHostSignals(w.exec.native.clients[0].trace)
}
