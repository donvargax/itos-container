package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/moby/moby/client"
)

// These fixtures lose exactly one original process. All product traffic still
// reaches the daemon through the recorder; independent inspection and the
// harness's kill use the native client, never a fabricated successful reply.
type egressLifeRuntime struct {
	response, id, path, edit  string
	recorder                  *egressLifeRecorder
	run                       *egressNativeRun
	journal                   []egressLifeRequest
	original                  *egressObserved
	killedAt                  time.Time
	killErr                   error
	statusBefore, statusAfter egressResult
	saved                     []byte
}

func (w *world) egressLifeRuntimeSteps(sc lifecycleStepRegistrar) {
	sc.Step(`^a CI egress failure probe whose worker "([^"]*)" after a successful tunnel and one refusal$`, w.egressLifeRuntimeFixture)
	sc.Step(`^independent direct-route controls remain reachable outside the worker$`, w.egressLifeRouteControls)
	sc.Step(`^run starts the probe and the harness kills only its independently identified proxy$`, w.egressLifeProxyLoss)
	sc.Step(`^native observations show the original running worker and proxy before the kill and run reports run.proxy-exited$`, w.egressLifeProxyExit)
	sc.Step(`^the worker receives the existing termination sequence with observed workload exit (\d+) and process exit (\d+)$`, w.egressLifeTermination)
	sc.Step(`^worker probes during the remaining grace deliver no direct-route nonce while the outside controls still succeed$`, func() error { return w.egressLifeRoutesDenied("grace") })
	sc.Step(`^no network attachment fallback proxy restart or additional worker start is requested$`, func() error { return egressLifeRuntimeRoutes(w.egress.lifeRuntime.journal) })
	sc.Step(`^fresh status reports the original one-record log after confirmed reverse owned cleanup$`, func() error { return w.egressLifeRuntimeStored(1) })
	sc.Step(`^a CI egress controller-loss probe with allowed and refused names and independently reachable route controls$`, func() error {
		if err := w.egressLifeRuntimeFixture("controller loss"); err != nil {
			return err
		}
		return w.egressLifeRouteControls()
	})
	sc.Step(`^run starts it the worker records one refusal and the harness kills only the controller$`, w.egressLifeControllerLoss)
	sc.Step(`^the original worker and proxy continue under the same frozen allowlist with no restart or widened route$`, w.egressLifeFrozenAfterLoss)
	sc.Step(`^the worker completes its allowed controls and second refusal while all direct-route attempts remain denied$`, w.egressLifeLossProbes)
	sc.Step(`^fresh status inspects the abandoned session and a fresh stop removes it before status runs again$`, w.egressLifeLostStop)
	sc.Step(`^the first status reports controller absent with a pending log and the exact stop candidate$`, w.egressLifeLostPending)
	sc.Step(`^the final status reports the original two-record log with cleanup complete after reverse full-ID removal$`, func() error { return w.egressLifeRuntimeStored(2) })
	sc.Step(`^the log contains both acknowledged refusals and unrelated resources and checkout edits are preserved$`, w.egressLifeLostPreserved)
}

func (w *world) egressLifeRuntimeFixture(response string) error {
	switch response {
	case "exits zero on SIGTERM", "exits 17 on SIGTERM", "ignores SIGTERM until kill", "controller loss":
	default:
		return fmt.Errorf("unknown runtime response %q", response)
	}
	// These scenarios judge the product's unchanged grace, call and teardown
	// budgets, so the world's product is the unmodified-budget build.
	if err := w.egressLifeFullProduct(); err != nil {
		return err
	}
	if err := w.egressNativeCheckout([]string{"api.example.com"}, []string{"api.example.com"}); err != nil {
		return err
	}
	if err := w.egressEndpoints(func(*egressNative) [][]string { return [][]string{{"runtime", "public", "api.example.com"}} }); err != nil {
		return err
	}
	recorder, err := w.egressLifeRecord()
	if err != nil {
		return err
	}
	w.egress.lifeRuntime = &egressLifeRuntime{response: response, recorder: recorder, edit: "runtime-edit-" + egressNonce()}
	return w.approvalCapture()
}

// Controls dial both fixture addresses from outside the worker. Each performs
// verified TLS and leaves its own nonce receipt; an unavailable IPv6 route is
// a failed control, never evidence that the worker's attempt was blocked.
func (w *world) egressLifeRouteControls() error {
	n, err := w.egressNativeWorld()
	if err != nil {
		return err
	}
	e := n.endpoints["runtime"]
	if e == nil || !e.address6.IsValid() {
		return errors.New("missing dual-stack route control")
	}
	n.control(e, "api.example.com")
	six := *e
	six.address, six.sent, six.controls = e.address6, nil, nil
	n.control(&six, "api.example.com")
	e.sent, e.controls = append(e.sent, six.sent...), append(e.controls, six.controls...)
	return nil
}

func (w *world) egressLifeDirectSteps(phase string) []egressStep {
	e := w.egress.native.endpoints["runtime"]
	return []egressStep{
		{Op: "direct", Mode: phase, Method: "tcp4", Target: net.JoinHostPort(e.address.String(), "443"), Name: "api.example.com", Nonce: "direct4-" + phase + "-" + egressNonce()},
		{Op: "direct", Mode: phase, Method: "tcp6", Target: net.JoinHostPort(e.address6.String(), "443"), Name: "api.example.com", Nonce: "direct6-" + phase + "-" + egressNonce()},
	}
}

func (w *world) egressLifeRuntimeStart(steps []egressStep, exit int) error {
	l, n := w.egress.lifeRuntime, w.egress.native
	l.run = n.newRun(append([]egressStep{{Op: "edit", Nonce: l.edit}}, steps...), exit)
	if err := l.run.start(); err != nil {
		return err
	}
	if !l.run.await("attach", 150*time.Second) {
		return nil
	}
	l.run.observe("attach")
	l.run.attach(l.run.observed["attach"])
	if err := l.run.release("attach"); err != nil {
		return err
	}
	if !l.run.await("loss", 90*time.Second) {
		return nil
	}
	l.run.observe("loss")
	l.original = l.run.observed["loss"]
	if l.original != nil && l.original.proxy != nil {
		l.id = l.original.proxy.Config.Labels["io.itos-container.session"]
		l.path = w.egressStatePath("sessions", w.recoveryIndexKey(), l.id, "egress-refusals.jsonl")
		l.recorder.mu.Lock()
		l.recorder.logPath = l.path
		l.recorder.mu.Unlock()
	}
	return nil
}

func (w *world) egressLifeProxyLoss() error {
	l, n := w.egress.lifeRuntime, w.egress.native
	exit := 0
	if l.response == "exits 17 on SIGTERM" {
		exit = 17
	}
	steps := []egressStep{n.canonicalControl(), {Op: "raw", Method: "CONNECT", Target: "before-loss.example:443"}, {Op: "wait-signal", Name: "loss"}}
	steps = append(steps, w.egressLifeDirectSteps("grace")...)
	if l.response == "ignores SIGTERM until kill" {
		steps = append(steps, egressStep{Op: "hold-after-signal"})
	}
	if err := w.egressLifeRuntimeStart(steps, exit); err != nil {
		return err
	}
	if l.original != nil && l.original.proxy != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		l.killedAt = time.Now()
		_, l.killErr = n.cli.ContainerKill(ctx, l.original.proxy.ID, client.ContainerKillOptions{Signal: "SIGKILL"})
		cancel()
	}
	l.run.finish(45 * time.Second)
	l.journal = l.recorder.snapshot()
	if err := w.egressLifeRouteControls(); err != nil {
		return err
	}
	if err := w.egressInvoke([]string{"status", "--json"}); err != nil {
		return err
	}
	l.statusAfter = w.egress.results[len(w.egress.results)-1]
	return nil
}

func egressLifeRunningPair(o *egressObserved) error {
	if o == nil || o.err != nil || o.worker == nil || o.proxy == nil || o.worker.State == nil || o.proxy.State == nil || !o.worker.State.Running || !o.proxy.State.Running || len(o.networks) != 2 {
		return errors.New("original running worker, proxy and both networks were not independently observed")
	}
	return nil
}

func (w *world) egressLifeProxyExit() error {
	l := w.egress.lifeRuntime
	if err := errors.Join(egressLifeRunningPair(l.original), l.killErr, l.run.err); err != nil {
		return err
	}
	if l.killedAt.IsZero() || l.run.timedOut || !strings.Contains(string(l.run.stderr.bytes()), "run.proxy-exited") {
		return fmt.Errorf("runtime proxy loss was not reported\n%s", l.run.describe())
	}
	refused := l.run.results("raw")
	if len(refused) != 1 || !refused[0].OK || refused[0].Status != 403 || refused[0].Body != 0 || !refused[0].Closed {
		return errors.New("pre-loss refusal was not acknowledged")
	}
	return w.egress.native.exchanged(l.run, 1)
}

func (w *world) egressLifeTermination(exit, process int) error {
	l := w.egress.lifeRuntime
	if l.original == nil || l.original.worker == nil {
		return errors.New("missing original worker")
	}
	if l.run.exit != process || !egressLifeWaitExit(l.journal, l.original.worker.ID, exit) {
		return fmt.Errorf("worker/controller exit is not %d/%d", exit, process)
	}
	receipt, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-28-signal"))
	if err != nil || string(receipt) != "SIGTERM" {
		return errors.New("worker did not record the controller's SIGTERM")
	}
	return egressLifeSignalOrder(l.journal, l.original.worker.ID, l.killedAt, exit == 137)
}

// Judge real signal requests against the product's unchanged grace, including
// the lower bound: an early KILL must not pass merely because exit was 137.
// The request recorder runs after the controller starts its signal call; allow
// 100 ms for that observation lag, not a shorter product grace.
func egressLifeSignalOrder(journal []egressLifeRequest, id string, after time.Time, forced bool) error {
	var term, kill time.Time
	count := 0
	for _, r := range journal {
		if r.method != "POST" || r.path != "/containers/"+id+"/kill" {
			continue
		}
		count++
		q, _ := url.ParseQuery(r.query)
		if r.status != 204 || r.at.Before(after) {
			return errors.New("signal failed or preceded proxy loss")
		}
		switch q.Get("signal") {
		case "SIGTERM":
			if !term.IsZero() || !kill.IsZero() {
				return errors.New("duplicate or late SIGTERM")
			}
			term = r.at
		case "SIGKILL":
			if term.IsZero() || !kill.IsZero() || r.at.Sub(term) < 10*time.Second-100*time.Millisecond {
				return errors.New("SIGKILL did not respect the product grace")
			}
			kill = r.at
		default:
			return errors.New("unexpected worker signal")
		}
	}
	if term.IsZero() || forced != !kill.IsZero() || (forced && count != 2) || (!forced && count != 1) {
		return errors.New("incomplete termination sequence")
	}
	return nil
}

func (w *world) egressLifeRoutesDenied(phases ...string) error {
	l, n := w.egress.lifeRuntime, w.egress.native
	e := n.endpoints["runtime"]
	if err := e.controlled(); err != nil {
		return err
	}
	receipts, err := e.receipts()
	if err != nil {
		return err
	}
	for _, phase := range phases {
		planned, completed := 0, 0
		for _, s := range l.run.steps {
			if s.Op != "direct" || s.Mode != phase {
				continue
			}
			planned++
			if slices.ContainsFunc(receipts, func(r egressReceipt) bool { return r.Nonce == s.Nonce }) {
				return errors.New("direct-route nonce reached its endpoint")
			}
			for _, r := range l.run.results("direct") {
				if r.Mode != phase || r.Target != s.Target {
					continue
				}
				completed++
				if r.OK || r.Error == "" || r.Reply != "" || r.Peer != "" {
					return errors.New("direct-route probe reached an outside service")
				}
			}
		}
		if planned != 2 || completed != 2 {
			return fmt.Errorf("phase %s did not complete both direct-route probes", phase)
		}
	}
	return e.onlyExchanges(false)
}

func egressLifeRuntimeRoutes(journal []egressLifeRequest) error {
	// The startup oracle permits exactly one original proxy start and its
	// one owned egress attachment. Remove only the single original worker
	// create/start pair, then use that oracle for every other mutation.
	var rest []egressLifeRequest
	worker, creates, starts := "", 0, 0
	created, proxyStarts := map[string]int{}, 0
	for _, r := range journal {
		if r.method == "POST" && strings.HasSuffix(r.path, "/create") {
			created[r.path+egressLifeCreateKind(&r)]++
		}
		if r.method == "POST" && strings.HasSuffix(r.path, "/start") && r.path != "/containers/"+worker+"/start" {
			proxyStarts++
		}
		if r.method == "POST" && r.path == "/containers/create" && egressLifeCreateKind(&r) != "proxy" {
			creates++
			var reply struct {
				ID string `json:"Id"`
			}
			if r.status != 201 || json.Unmarshal(r.response, &reply) != nil || reply.ID == "" {
				return errors.New("worker create was not independently recorded")
			}
			worker = reply.ID
			continue
		}
		if worker != "" && r.method == "POST" && r.path == "/containers/"+worker+"/start" {
			starts++
			if r.status != 204 {
				return errors.New("worker start failed")
			}
			continue
		}
		if worker != "" && r.method == "POST" && (r.path == "/containers/"+worker+"/wait" || r.path == "/containers/"+worker+"/kill" || r.path == "/containers/"+worker+"/attach") {
			continue
		}
		rest = append(rest, r)
	}
	for _, count := range created {
		if count != 1 {
			return errors.New("runtime requested duplicate resources")
		}
	}
	if len(created) != 5 || proxyStarts != 1 || creates != 1 || starts != 1 {
		return errors.New("worker was absent, replaced or restarted")
	}
	return egressLifeStartupRoutes(rest)
}

func (w *world) egressLifeRuntimeStored(records int) error {
	l := w.egress.lifeRuntime
	if err := egressLifeStoredStatus(l.statusAfter, l.id, l.path, "stored", records); err != nil {
		return err
	}
	if err := egressLifeReverseCreated(l.journal); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	remaining, err := w.egress.native.owned(ctx)
	if err != nil || len(remaining) != 0 {
		return errors.New("original resources remain after cleanup")
	}
	data, err := os.ReadFile(l.path)
	if err != nil {
		return err
	}
	l.saved = bytes.Clone(data)
	if l.original == nil || l.original.proxy == nil {
		return errors.New("missing original proxy")
	}
	if err := egressLifePublishedBeforeRemoval(l.journal, l.original.proxy.ID, data); err != nil {
		return err
	}
	if bytes.Count(data, []byte{'\n'}) != records {
		return errors.New("stored count disagrees with refusal bytes")
	}
	for i, line := range bytes.Split(bytes.TrimSuffix(data, []byte{'\n'}), []byte{'\n'}) {
		var r map[string]any
		if json.Unmarshal(line, &r) != nil || r["seq"] != float64(i+2) || r["event"] != "refused" || r["reason"] != "not-listed" || r["target"] != "before-loss.example" {
			return errors.New("acknowledged refusal was lost or changed")
		}
	}
	return nil
}

func (w *world) egressLifeControllerLoss() error {
	l, n := w.egress.lifeRuntime, w.egress.native
	steps := []egressStep{n.canonicalControl(), {Op: "raw", Method: "CONNECT", Target: "before-loss.example:443"}}
	steps = append(steps, w.egressLifeDirectSteps("before")...)
	steps = append(steps, egressStep{Op: "barrier", Name: "loss"}, n.canonicalControl(), egressStep{Op: "raw", Method: "CONNECT", Target: "before-loss.example:443"})
	steps = append(steps, w.egressLifeDirectSteps("after")...)
	steps = append(steps, egressStep{Op: "wait-signal", Name: "abandoned"})
	if err := w.egressLifeRuntimeStart(steps, 0); err != nil {
		return err
	}
	if l.original != nil && l.original.worker != nil && l.original.proxy != nil {
		l.killedAt = time.Now()
		l.killErr = l.run.cmd.Process.Kill()
		l.run.finish(10 * time.Second)
		// A pending live log is inspected by a different process, with no
		// dependency on the killed controller's in-memory observations.
		if err := w.egressInvoke([]string{"status", "--json"}); err != nil {
			return err
		}
		l.statusBefore = w.egress.results[len(w.egress.results)-1]
		if err := l.run.release("loss"); err != nil {
			return err
		}
		if egressLifeAwaitWorker(w.doctor.checkout, "abandoned", 90*time.Second) {
			l.run.reached["abandoned"] = true
			l.run.observe("abandoned")
		}
		data, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-27-report.json"))
		if err == nil {
			err = json.Unmarshal(data, &l.run.report)
		}
		l.run.err = errors.Join(l.run.err, err)
	} else {
		l.run.finish(10 * time.Second)
	}
	l.journal = l.recorder.snapshot()
	return w.egressLifeRouteControls()
}

// Controller exit is expected in this case, so the ordinary await (which
// stops waiting on run.done) cannot observe continued worker progress.
func egressLifeAwaitWorker(checkout, name string, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(checkout, "slice-27-barrier-"+name)); err == nil {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return false
}

func (w *world) egressLifeFrozenAfterLoss() error {
	l, n := w.egress.lifeRuntime, w.egress.native
	if err := errors.Join(egressLifeRunningPair(l.original), egressLifeRunningPair(l.run.observed["abandoned"]), l.run.err, l.killErr); err != nil {
		return err
	}
	if l.run.cmd == nil || l.run.cmd.ProcessState == nil {
		return errors.New("controller death was not observed")
	}
	status, ok := l.run.cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
		return errors.New("controller did not die from the harness's SIGKILL")
	}
	after := l.run.observed["abandoned"]
	if l.original.worker.ID != after.worker.ID || l.original.proxy.ID != after.proxy.ID || l.original.worker.State.StartedAt != after.worker.State.StartedAt || l.original.proxy.State.StartedAt != after.proxy.State.StartedAt || l.original.worker.RestartCount != after.worker.RestartCount || l.original.proxy.RestartCount != after.proxy.RestartCount || !reflect.DeepEqual(l.original.worker.NetworkSettings, after.worker.NetworkSettings) || !reflect.DeepEqual(l.original.proxy.Config, after.proxy.Config) || !reflect.DeepEqual(l.original.proxy.NetworkSettings, after.proxy.NetworkSettings) {
		return errors.New("controller loss changed original resources or frozen proxy grant")
	}
	if err := n.frozenProxy(after, []string{"api.example.com"}, w.approval.fingerprint); err != nil {
		return err
	}
	return egressLifeRuntimeRoutes(l.journal)
}

func (w *world) egressLifeLossProbes() error {
	l, n := w.egress.lifeRuntime, w.egress.native
	if !l.run.reached["abandoned"] {
		return errors.New("worker made no progress after controller loss")
	}
	if err := n.exchanged(l.run, 2); err != nil {
		return err
	}
	refused := l.run.results("raw")
	if len(refused) != 2 {
		return errors.New("both acknowledged refusals are required")
	}
	for _, r := range refused {
		if !r.OK || r.Status != 403 || r.Body != 0 || !r.Closed {
			return errors.New("worker's refusal was not acknowledged")
		}
	}
	return w.egressLifeRoutesDenied("before", "after")
}

func (w *world) egressLifeLostStop() error {
	l := w.egress.lifeRuntime
	if l.id == "" {
		return nil
	}
	if err := w.egressInvoke([]string{"status", "--json"}, []string{"stop", l.id, "--json"}, []string{"status", "--json"}); err != nil {
		return err
	}
	results := w.egress.results
	l.statusBefore, l.statusAfter = results[len(results)-3], results[len(results)-1]
	l.journal = l.recorder.snapshot()
	return nil
}

func (w *world) egressLifeLostPending() error {
	l := w.egress.lifeRuntime
	entry, problems, err := egressLifeEntry(l.statusBefore, l.id)
	if err != nil {
		return err
	}
	want := map[string]any{"path": l.path, "state": "pending", "records": nil, "missing": []any{}}
	if l.statusBefore.result.exit != 0 || len(problems) != 0 || entry["controller"] != "absent" || entry["stop_candidate"] != true || entry["workload_exit"] != nil || entry["resource"] != "running" || !sameJSON(entry["retry"], []string{"itos", "container", "stop", l.id}) || !sameJSON(object(entry["egress"])["refusal_log"], want) {
		return errors.New("abandoned status lost controller absence, pending evidence or exact stop candidate")
	}
	return nil
}

func (w *world) egressLifeLostPreserved() error {
	l, n := w.egress.lifeRuntime, w.egress.native
	now, err := os.ReadFile(l.path)
	if err != nil || !bytes.Equal(l.saved, now) {
		return errors.New("stored refusal bytes changed")
	}
	edit, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-28-edit"))
	if err != nil || string(edit) != l.edit {
		return errors.New("checkout edit was lost")
	}
	if err := w.approvalRecordUnchanged(); err != nil {
		return err
	}
	if err := n.endpointsIntact(); err != nil {
		return err
	}
	for _, r := range l.journal {
		if r.method != "DELETE" && r.method != "POST" {
			continue
		}
		for _, endpoint := range n.endpoints {
			if strings.Contains(r.path, endpoint.id) {
				return errors.New("product mutated an unrelated endpoint")
			}
		}
		for _, network := range n.networks {
			if strings.Contains(r.path, network.id) {
				return errors.New("product mutated an unrelated network")
			}
		}
	}
	return nil
}

// A post-cleanup file alone cannot prove preservation happened before the
// source vanished. The recorder observes bytes and mode at the real deletion.
func egressLifePublishedBeforeRemoval(journal []egressLifeRequest, proxy string, saved []byte) error {
	for _, r := range journal {
		if r.method == "DELETE" && r.path == "/containers/"+proxy && r.status == 204 {
			if r.logErr != nil || !r.logMode.IsRegular() || r.logMode.Perm() != 0600 || len(saved) == 0 || !bytes.Equal(r.logBefore, saved) {
				return errors.New("proxy removal preceded private committed refusal evidence")
			}
			return nil
		}
	}
	return errors.New("no original proxy removal was observed")
}
