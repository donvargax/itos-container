package features

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

func (w *world) execNativeSteps(sc lifecycleStepRegistrar) {
	one := func() error { return w.nativeExecFixture(false) }
	two := func() error { return w.nativeExecFixture(true) }
	sc.Step("^a CI exec checkout with frozen subdirectory settings and a trusted reporting probe$", one)
	sc.Step("^a CI exec checkout with a stubborn primary and stubborn exec probe plus an unrelated running session$", two)
	sc.Step("^a CI exec checkout with a stubborn primary and a continuously reporting exec probe$", one)
	sc.Step("^a CI exec checkout with a gated primary and two long-running exec probes$", one)
	sc.Step("^two CI exec checkouts with independent synthetic grants and trusted probes$", two)

	sc.Step(`^run reaches its primary barrier and exec receives that array and finite stdin with exit (\d+) after (\d+) seconds$`, func(exit, seconds int) { w.exec.native.err = w.nativeReportAction(exit, seconds) })
	sc.Step(`^the native exec receipt has the exact argv and frozen settings with timely separate streams and exit (\d+)$`, w.nativeReportResult)
	sc.Step("^stdin EOF leaves the primary and exec alive until the probe finishes and all output tails arrive$", w.nativeTails)
	sc.Step("^the native exec has the primary's restricted identity and namespaces and cannot write the image root$", w.nativeIsolation)
	sc.Step("^a second exec succeeds in the same still-running session without changing its grant or primary result$", w.nativeSecond)
	sc.Step("^releasing the primary returns its own exit 17 and confirms owned cleanup with checkout edits preserved$", w.nativeFinished)

	sc.Step(`^run and exec reach their barriers and the exec client receives "([^"]*)" followed by repeated signals$`, func(signal string) { w.exec.native.err = w.nativeSignalAction(signal) })
	sc.Step(`^native exec readiness precedes one "([^"]*)" to the owned primary and bounded escalation and session cleanup$`, w.nativeSignals)
	sc.Step(`^the exec client reports cancellation and session-wide teardown with its observed exec exit or the unknown-exit fallback (\d+)$`, w.nativeCancelled)
	sc.Step("^neither exec nor its descendants remain after cleanup and the unrelated session keeps making progress$", w.nativeProgress)
	sc.Step("^checkout edits and unrelated state survive and fresh status confirms the target session is stopped$", w.nativeStopped)

	sc.Step(`^run and exec reach their barriers and the harness breaks "([^"]*)" while keeping engine control available$`, func(stream string) { w.exec.native.err = w.nativeBreakAction(stream) })
	sc.Step("^the started exec and primary are terminated and owned cleanup completes within thirty seconds of detected disconnect$", w.nativeDisconnected)
	sc.Step("^the exec client returns a nonzero observed exec exit or wrapper 75 without claiming successful execution$", w.nativeDisconnectExit)
	sc.Step("^closing the attachment alone is not counted as termination and no helper exec or host PID signal is used$", func() error { return errors.Join(w.nativeAbsent(), w.nativeTraceClean()) })

	sc.Step(`^both execs reach their barriers and the session ends by "([^"]*)" while one exec output drain is blocked$`, func(event string) { w.exec.native.err = w.nativeConcurrentEnd(event) })
	sc.Step("^both native execs started and session cleanup completes within thirty seconds without waiting for their natural exit$", w.nativeTwoEnded)
	sc.Step("^both exec results remain separate from the primary and stop results and output drain is bounded by ten seconds$", w.nativeDrained)
	sc.Step(`^every exec descendant is gone and a subsequent exec is refused with "([^"]*)"$`, func(rule string) error { return errors.Join(w.nativeDescendantsStopped(), w.nativeRefused(rule)) })
	sc.Step("^the checkout edit survives and no session resource is restarted or leaked$", w.nativeNoRestart)

	sc.Step(`^the product attempts exec against a "([^"]*)" session and a live owned control$`, func(target string) { w.exec.native.err = w.nativeRefusalAction(target) })
	sc.Step(`^the target exec reports "([^"]*)" with exit 1 and the owned control returns its exact probe receipt and exit 23$`, func(rule string) error {
		if err := w.exec.native.err; err != nil {
			return err
		}
		return errors.Join(w.nativeRefused(rule), w.recovery.second.nativeCheckOnce(w.exec.native.control, "control"))
	})
	sc.Step("^no probe ran in the refused target and its resources and records were neither adopted nor changed$", w.nativeRefusalPreserved)
}

func (w *world) nativeReportResult(exit int) error {
	n := w.exec.native
	if n.err != nil {
		return n.err
	}
	c, r := n.clients[0], n.receipt
	if c.p.exit != exit || !slices.Equal(r.Argv, w.exec.argv) || r.Cwd != "/workspace/subdir" || r.App != offlineAPP ||
		r.Home != "/tmp" || r.Tmp != "/tmp" || r.EnvHash == "" || r.EnvHash != n.primary.EnvHash {
		return fmt.Errorf("reporting exec did not preserve literal argv, frozen settings and exit %d (observed %d)", exit, c.p.exit)
	}
	return w.nativeTails()
}

func (w *world) nativeTails() error {
	n := w.exec.native
	c := n.clients[0]
	first, rest, ok := bytes.Cut(c.p.out.bytes(), []byte("\n"))
	var r nativeExecReceipt
	if !ok || json.Unmarshal(first, &r) != nil || r.Nonce != n.nonce || c.ready.IsZero() || !c.eof.After(c.ready) ||
		string(rest) != "stdout tail "+n.nonce+"\n" || string(c.p.errout.bytes()) != "live "+n.nonce+" report\nstderr tail "+n.nonce+"\n" ||
		!w.recoveryNativePresent(true) {
		return errors.New("exec and primary did not survive stdin EOF with both exact stream tails")
	}
	return nil
}

func (w *world) nativeIsolation() error {
	n := w.exec.native
	r, p := n.receipt, n.primary
	identity, err := doctorCheckoutIdentity(w.doctor.checkout)
	if err != nil {
		return err
	}
	if strconv.Itoa(r.UID) != w.approval.uid || strconv.Itoa(r.GID) != w.approval.gid || r.UID == 0 || r.UID != p.UID || r.GID != p.GID ||
		!r.ReadOnly || !r.Denied || r.Status["NoNewPrivs"] != "1" ||
		r.MountNS == "" || r.MountNS != p.MountNS || r.NetNS == "" || r.NetNS != p.NetNS ||
		r.WorkspaceInode == 0 || r.WorkspaceInode != p.WorkspaceInode || strconv.FormatUint(r.WorkspaceInode, 10) != fmt.Sprint(identity["inode"]) ||
		r.WorkspaceDevice != p.WorkspaceDevice {
		return errors.New("exec did not report the original non-root identity, restrictions and mounted checkout")
	}
	for _, key := range []string{"CapInh", "CapPrm", "CapEff", "CapBnd", "CapAmb"} {
		n, err := strconv.ParseUint(r.Status[key], 16, 64)
		if err != nil || n != 0 {
			return fmt.Errorf("exec did not report zero %s", key)
		}
	}
	return w.nativeEdit()
}

func (w *world) nativeSecond() error {
	p := w.nativeOnce("second")
	if err := w.nativeCheckOnce(p, "second"); err != nil {
		return err
	}
	after, err := w.nativeRecords()
	if err != nil {
		return err
	}
	if !slices.Equal(after, w.exec.native.before) {
		return errors.New("exec changed the original registration, intent or frozen grant")
	}
	entry, err := w.nativeStatus("running", "running", "pending")
	if err != nil {
		return err
	}
	if entry.WorkloadExit != nil || entry.Controller != "present" {
		return errors.New("exec changed the running primary's result or controller")
	}
	return nil
}

func (w *world) nativeFinished() error {
	if err := w.nativeRelease(); err != nil {
		return err
	}
	entry, err := w.nativeStatus("stopped", "absent", "complete")
	if err != nil {
		return err
	}
	if w.recovery.run.exit != 17 || entry.WorkloadExit == nil || *entry.WorkloadExit != 17 {
		return errors.New("the primary's own exit 17 was lost")
	}
	return errors.Join(w.nativeAbsent(), w.nativeEdit(), w.nativeResourceInventory())
}

func (w *world) nativeSignals(signal string) error {
	n := w.exec.native
	if n.err != nil {
		return n.err
	}
	c := n.clients[0]
	var kills []offlineRequest
	for _, q := range w.nativeEntries() {
		if q.Method == "POST" && strings.HasSuffix(q.Path, "/kill") {
			if q.Path != "/containers/"+w.recovery.worker+"/kill" || q.Status != 204 {
				return errors.New("signal did not target the original full worker ID")
			}
			kills = append(kills, q)
		}
	}
	if len(kills) != 2 {
		return fmt.Errorf("got %d engine signals, want one first signal and one escalation", len(kills))
	}
	first, err := url.ParseQuery(kills[0].Query)
	last, err2 := url.ParseQuery(kills[1].Query)
	elapsed := kills[1].At.Sub(n.event)
	if err != nil || err2 != nil || first.Get("signal") != signal || last.Get("signal") != "SIGKILL" || c.ready.IsZero() || n.event.Before(c.ready) ||
		kills[0].At.Before(n.event) || elapsed < engineGraceBudget || elapsed > engineGraceBudget+engineEscalationTolerance {
		return errors.New("signal order or the original escalation deadline was lost")
	}
	return errors.Join(w.nativeTraceClean(), w.nativeStopped())
}

func (w *world) nativeCancelled(fallback int) error {
	n := w.exec.native
	c := n.clients[0]
	code, observed := w.nativeObservedExit(c)
	if !observed {
		code = fallback
	}
	diag := strings.ToLower(string(c.p.errout.bytes()))
	if c.p.exit != code || !strings.Contains(diag, "cancel") || !strings.Contains(diag, "session") {
		return fmt.Errorf("exec cancellation lacked session-wide diagnostics or exit %d (got %d)", code, c.p.exit)
	}
	return nil
}

func (w *world) nativeDescendantsStopped() error {
	if err := w.nativeAbsent(); err != nil {
		return err
	}
	counts := make([]int, len(w.exec.native.clients))
	for i, c := range w.exec.native.clients {
		counts[i] = w.nativeTick(c.name + "-child")
		if counts[i] < 1 {
			return fmt.Errorf("no descendant progress for exec %s", c.name)
		}
	}
	time.Sleep(200 * time.Millisecond)
	for i, c := range w.exec.native.clients {
		if w.nativeTick(c.name+"-child") != counts[i] {
			return fmt.Errorf("exec %s descendant progressed after cleanup", c.name)
		}
	}
	return nil
}

func (w *world) nativeProgress() error {
	if err := w.nativeDescendantsStopped(); err != nil {
		return err
	}
	other := w.recovery.second
	before := other.nativeTick("primary-tick")
	if err := nativeAwait(other.recovery.run, 5*time.Second, func() bool { return other.nativeTick("primary-tick") > before }); err != nil {
		return err
	}
	records, err := other.nativeRecords()
	if err != nil {
		return err
	}
	if !slices.Equal(records, other.exec.native.before) || !other.recoveryNativePresent(true) {
		return errors.New("unrelated session changed during teardown")
	}
	return other.nativeEdit()
}

func (w *world) nativeDisconnected() error {
	n := w.exec.native
	if n.err != nil {
		return n.err
	}
	c := n.clients[0]
	if c.ready.IsZero() || n.event.Before(c.ready) || n.cleaned.Before(n.event) || n.cleaned.Sub(n.event) > 30*time.Second {
		return errors.New("disconnect cleanup was not observed inside its original deadline")
	}
	if c.mode != "transport" && w.nativeTick(c.name+"-attempt") < 1 {
		return errors.New("the ready probe never attempted output on the broken sink")
	}
	return errors.Join(w.nativeDescendantsStopped(), w.nativeStopped())
}

func (w *world) nativeDisconnectExit() error {
	c := w.exec.native.clients[0]
	code, ok := w.nativeObservedExit(c)
	if !ok || code == 0 {
		code = 75
	}
	if c.p.exit != code {
		return fmt.Errorf("disconnected exec returned %d, expected %d", c.p.exit, code)
	}
	return nil
}

func (w *world) nativeTwoEnded() error {
	n := w.exec.native
	if n.err != nil {
		return n.err
	}
	if len(n.clients) != 2 || n.cleaned.Sub(n.event) > 30*time.Second {
		return errors.New("both execs and bounded session cleanup were not observed")
	}
	for _, c := range n.clients {
		if c.ready.IsZero() || c.ready.After(n.event) {
			return fmt.Errorf("missing readiness for exec %s before session end", c.name)
		}
	}
	return w.nativeAbsent()
}

func (w *world) nativeDrained() error {
	n := w.exec.native
	status, err := w.nativeStatus("stopped", "absent", "complete")
	if err != nil {
		return err
	}
	if n.stop != nil {
		if n.stop.exit != 0 {
			return fmt.Errorf("concurrent stop exited %d", n.stop.exit)
		}
	} else if w.recovery.run.exit != 17 || status.WorkloadExit == nil || *status.WorkloadExit != 17 {
		return errors.New("primary release did not retain exit 17")
	}
	for _, c := range n.clients {
		code, ok := w.nativeObservedExit(c)
		if !ok {
			code = 75
			if !bytes.Contains(c.p.errout.bytes(), []byte("exec.workload-exit-unknown")) {
				return fmt.Errorf("exec %s did not report the missing exit", c.name)
			}
		}
		if c.p.exit != code {
			return fmt.Errorf("exec %s returned %d, want its own outcome %d", c.name, c.p.exit, code)
		}
	}
	// The full pipe has had no reader since before the primary exited. Its
	// ten-second drain starts at exit, observed on run's own engine wait.
	var stopped time.Time
	for _, q := range w.nativeEntries() {
		if q.Path == "/containers/"+w.recovery.worker+"/wait" && q.Status == 200 && q.Observation["StatusCode"] != nil && !q.Replied.IsZero() {
			if stopped.IsZero() || q.Replied.Before(stopped) {
				stopped = q.Replied
			}
		}
		if q.Path == "/containers/"+w.recovery.worker+"/json" && q.Status == 200 && q.State["Running"] == false && q.At.After(n.event) && !q.Replied.IsZero() && (stopped.IsZero() || q.Replied.Before(stopped)) {
			stopped = q.Replied
		}
	}
	if stopped.IsZero() || n.clients[1].p.finished.After(stopped.Add(10*time.Second+engineEscalationTolerance)) {
		return errors.New("blocked exec output drain exceeded its exit-relative ten-second budget")
	}
	return nil
}

func (w *world) nativeRefused(rule string) error {
	p := w.exec.native.refused
	if p == nil || p.err != nil || p.exit != 1 || len(p.out.bytes()) != 0 || !bytes.Contains(p.errout.bytes(), []byte(rule)) {
		return fmt.Errorf("target exec did not return %s and exit 1", rule)
	}
	return nil
}

func (w *world) nativeRefusalPreserved() error {
	n := w.exec.native
	if _, err := os.Stat(w.nativePath("once")); !os.IsNotExist(err) {
		return errors.New("refused probe left a receipt in its target")
	}
	if !slices.Equal(n.before, n.after) {
		return errors.New("refused exec changed target session records")
	}
	for _, q := range w.nativeEntries() {
		if q.At.Before(n.event) || q.Method == "GET" || q.Method == "HEAD" {
			continue
		}
		return fmt.Errorf("refused exec attempted %s %s", q.Method, q.Path)
	}
	state, err := sessionDecodeRecord(n.before[0].Bytes)
	if err != nil {
		return err
	}
	if state["state"] == "stopped" {
		return w.nativeStopped()
	}
	if state["state"] != "running" || !w.recoveryNativePresent(true) {
		return errors.New("refused live target was not preserved")
	}
	return w.nativeEdit()
}
