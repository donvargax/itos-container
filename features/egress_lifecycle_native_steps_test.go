package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

type egressLifeNative struct {
	ending, edit, id, path string
	recorder               *egressLifeRecorder
	run                    *egressNativeRun
	journal                []egressLifeRequest
	beforeStatus           map[string]string
}

func (w *world) egressLifeNativeSteps(sc lifecycleStepRegistrar) {
	sc.Step(`^a CI egress lifecycle probe with a retained checkout edit and ending "([^"]*)"$`, w.egressLifeEndingFixture)
	sc.Step(`^unrelated control resources and an independent allowed TLS endpoint$`, func() error {
		return w.egressEndpoints(func(*egressNative) [][]string { return [][]string{{"lifecycle", "public", "api.example.com"}} })
	})
	sc.Step(`^run starts the probe exchanges its control and last refused CONNECT then receives that ending$`, w.egressLifeEndingRun)
	sc.Step(`^the real worker receipts and engine exit require process exit (\d+) with only the expected cancellation diagnostic$`, w.egressLifeEndingResult)
	sc.Step(`^product teardown confirms worker absence then stopped proxy then committed log then proxy egress network internal network and anchor absence$`, w.egressLifeEndingOrder)
	sc.Step(`^the last acknowledged refusal is in the private published log before the proxy is removed$`, w.egressLifeEndingLog)
	sc.Step(`^fresh status reports that log with one stored record and cleanup complete without a proxy-failure diagnostic$`, w.egressLifeEndingStatus)
	sc.Step(`^the checkout edit original approval and unrelated resources survive unchanged$`, w.egressLifeEndingPreserved)
}

func (w *world) egressLifeEndingFixture(ending string) error {
	switch ending {
	case "primary exits zero", "primary exits 17", "controller receives SIGINT", "controller receives SIGTERM":
	default:
		return fmt.Errorf("unknown lifecycle ending %q", ending)
	}
	if err := w.egressNativeCheckout([]string{"api.example.com"}, []string{"api.example.com"}); err != nil {
		return err
	}
	recorder, err := w.egressLifeRecord()
	if err != nil {
		return err
	}
	w.egress.lifeNative = &egressLifeNative{ending: ending, edit: "retained-edit-" + egressNonce(), recorder: recorder}
	return nil
}

func (w *world) egressLifeEndingRun() error {
	n, err := w.egressNativeWorld()
	if err != nil {
		return err
	}
	l := w.egress.lifeNative
	exit := 17
	if l.ending == "primary exits zero" {
		exit = 0
	}
	ending := egressStep{Op: "barrier", Name: "ending"}
	if strings.HasPrefix(l.ending, "controller receives") {
		ending.Op = "wait-signal"
	}
	steps := []egressStep{{Op: "edit", Nonce: l.edit}, n.canonicalControl(), {Op: "raw", Method: "CONNECT", Target: "last-refusal.example:443"}, ending}
	run := n.newRun(steps, exit)
	l.run = run
	if err := run.start(); err != nil {
		return err
	}
	if run.await("attach", 150*time.Second) {
		run.observe("attach")
		run.attach(run.observed["attach"])
		if err := run.release("attach"); err != nil {
			return err
		}
		if run.await("ending", 90*time.Second) {
			run.observe("ending")
			if o := run.observed["ending"]; o != nil && o.proxy != nil {
				l.id = o.proxy.Config.Labels["io.itos-container.session"]
				l.path = w.egressStatePath("sessions", w.recoveryIndexKey(), l.id, "egress-refusals.jsonl")
				l.recorder.mu.Lock()
				l.recorder.logPath = l.path
				l.recorder.mu.Unlock()
			}
			switch l.ending {
			case "controller receives SIGINT":
				run.err = errors.Join(run.err, run.cmd.Process.Signal(os.Interrupt))
			case "controller receives SIGTERM":
				run.err = errors.Join(run.err, run.cmd.Process.Signal(syscall.SIGTERM))
			default:
				run.err = errors.Join(run.err, run.release("ending"))
			}
		}
	}
	run.finish(45 * time.Second)
	l.journal = l.recorder.snapshot()
	n.controlAll()
	// Status is a separate process in each format, after the controller has
	// gone and before the harness's identity-checked cleanup safety net.
	l.beforeStatus, err = doctorTree(w.egressStatePath())
	if err != nil {
		run.err = errors.Join(run.err, err)
	}
	return w.egressInvoke([]string{"status"}, []string{"status", "--json"})
}

func (w *world) egressLifeEndingResult(exit int) error {
	l, n := w.egress.lifeNative, w.egress.native
	run := l.run
	if run == nil || !run.reached["attach"] || !run.reached["ending"] {
		return errors.New("worker did not reach both native lifecycle barriers")
	}
	if err := run.cleaned(); err != nil {
		return fmt.Errorf("%w\n%s", err, run.describe())
	}
	if run.exit != exit {
		return fmt.Errorf("process exit %d, want %d", run.exit, exit)
	}
	o := run.observed["ending"]
	if o == nil || o.err != nil || o.worker == nil || o.proxy == nil || !o.worker.State.Running || !o.proxy.State.Running {
		return errors.New("original worker and proxy were not independently observed running")
	}
	if !egressLifeWaitExit(l.journal, o.worker.ID, exit) {
		return errors.New("exit lacks a completed native worker wait reply")
	}
	if err := errors.Join(n.exchanged(run, 1), n.endpointsIntact()); err != nil {
		return err
	}
	refusals := run.results("raw")
	if len(refusals) != 1 || !refusals[0].OK || refusals[0].Status != 403 || refusals[0].Body != 0 || !refusals[0].Closed {
		return errors.New("last refusal lacks an acknowledged empty 403 and closure")
	}
	diagnostic := string(run.stderr.bytes())
	if strings.HasPrefix(l.ending, "controller receives ") {
		want := strings.TrimPrefix(l.ending, "controller receives ")
		receipt, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-28-signal"))
		if err != nil || string(receipt) != want || !strings.Contains(diagnostic, "run.cancelled") || !strings.Contains(diagnostic, want) {
			return errors.New("cancellation lost the first signal receipt or diagnostic")
		}
	} else if strings.Contains(diagnostic, "run.cancelled") {
		return errors.New("ordinary workload exit was misclassified as cancellation")
	}
	for _, failure := range []string{"run.proxy-exited", "run.proxy-failed", "run.cleanup", "run.call-timeout", "run.teardown-timeout"} {
		if strings.Contains(diagnostic, failure) {
			return fmt.Errorf("unexpected lifecycle diagnosis %s", failure)
		}
	}
	return nil
}

func (w *world) egressLifeEndingOrder() error {
	l := w.egress.lifeNative
	o := l.run.observed["ending"]
	if o == nil || o.worker == nil || o.proxy == nil {
		return errors.New("missing independently observed full IDs")
	}
	anchor := ""
	for _, mount := range o.worker.Mounts {
		if mount.Destination == "/workspace" {
			anchor = mount.Name
		}
	}
	if anchor == "" {
		return errors.New("worker's original anchor was not inspected")
	}
	var deletes []string
	for _, request := range l.journal {
		if request.method == "DELETE" {
			deletes = append(deletes, request.path)
		}
	}
	wantDeletes := []string{"/containers/" + o.worker.ID, "/containers/" + o.proxy.ID, "/networks/" + o.networks["egress-network"].ID, "/networks/" + o.networks["internal-network"].ID, "/volumes/" + anchor}
	if !slices.Equal(deletes, wantDeletes) {
		return errors.New("owned removal order differs or contains an extra deletion")
	}
	position := 0
	seek := func(method, path string, status int, predicate func(egressLifeRequest) bool) bool {
		for position < len(l.journal) {
			r := l.journal[position]
			position++
			if r.method == method && r.path == path && r.status == status && (predicate == nil || predicate(r)) {
				return true
			}
		}
		return false
	}
	if !seek("DELETE", "/containers/"+o.worker.ID, 204, nil) || !seek("GET", "/containers/"+o.worker.ID+"/json", 404, nil) {
		return errors.New("worker removal and confirmed absence were not observed")
	}
	if !seek("POST", "/containers/"+o.proxy.ID+"/kill", 204, func(r egressLifeRequest) bool { q, _ := url.ParseQuery(r.query); return q.Get("signal") == "SIGKILL" }) {
		return errors.New("proxy was not killed after worker absence")
	}
	if !seek("POST", "/containers/"+o.proxy.ID+"/wait", 200, func(r egressLifeRequest) bool { return !r.finished.IsZero() }) {
		return errors.New("proxy stop wait did not finish before copying")
	}
	if !seek("GET", "/containers/"+o.proxy.ID+"/json", 200, func(r egressLifeRequest) bool {
		var v struct {
			State struct {
				Running bool
				Status  string
			}
		}
		return json.Unmarshal(r.response, &v) == nil && !v.State.Running && v.State.Status == "exited"
	}) {
		return errors.New("proxy absence of execution was not revalidated")
	}
	if !seek("GET", "/containers/"+o.proxy.ID+"/logs", 200, func(r egressLifeRequest) bool {
		q, _ := url.ParseQuery(r.query)
		return q.Get("follow") != "1" && q.Get("follow") != "true" && !r.finished.IsZero()
	}) {
		return errors.New("final copy was not a completed stopped stdout read")
	}
	if !seek("DELETE", "/containers/"+o.proxy.ID, 204, func(r egressLifeRequest) bool { return r.logErr == nil && len(r.logBefore) != 0 }) || !seek("GET", "/containers/"+o.proxy.ID+"/json", 404, nil) {
		return errors.New("proxy removal preceded committed refusal evidence or lacked absence confirmation")
	}
	for _, kind := range []string{"egress-network", "internal-network"} {
		network, ok := o.networks[kind]
		if !ok || !seek("DELETE", "/networks/"+network.ID, 204, nil) || !seek("GET", "/networks/"+network.ID, 404, nil) {
			return fmt.Errorf("%s removal/absence is out of order", kind)
		}
	}
	if !seek("DELETE", "/volumes/"+anchor, 204, nil) || !seek("GET", "/volumes/"+anchor, 404, nil) {
		return errors.New("anchor removal/absence did not follow the egress resources")
	}
	return nil
}

func (w *world) egressLifeEndingLog() error {
	l := w.egress.lifeNative
	o := l.run.observed["ending"]
	if o == nil || o.proxy == nil {
		return errors.New("missing original proxy")
	}
	var saved []byte
	for _, r := range l.journal {
		if r.method == "DELETE" && r.path == "/containers/"+o.proxy.ID {
			if r.logErr != nil || !r.logMode.IsRegular() || r.logMode.Perm() != 0600 {
				return errors.New("log was not privately published before proxy removal")
			}
			saved = r.logBefore
		}
	}
	if bytes.Count(saved, []byte{'\n'}) != 1 {
		return errors.New("published log does not contain exactly the last refusal")
	}
	var r map[string]any
	if json.Unmarshal(bytes.TrimSpace(saved), &r) != nil || r["schema"] != float64(1) || r["seq"] != float64(2) || r["event"] != "refused" || r["method"] != "CONNECT" || r["target"] != "last-refusal.example" || r["port"] != "443" || r["reason"] != "not-listed" || len(r) != 8 {
		return errors.New("last refusal metadata was changed or dropped")
	}
	if stamp, ok := r["time"].(string); !ok || !strings.HasSuffix(stamp, "Z") {
		return errors.New("refusal lacks UTC time")
	} else if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
		return err
	}
	now, err := os.ReadFile(l.path)
	if err != nil || !bytes.Equal(saved, now) {
		return errors.New("published refusal bytes changed after proxy removal")
	}
	return nil
}

func (w *world) egressLifeEndingStatus() error {
	l := w.egress.lifeNative
	plain, structured, err := w.egressStatusResults()
	if err != nil {
		return err
	}
	if plain.result.exit != 0 || structured.result.exit != 0 {
		return fmt.Errorf("fresh status exits %d/%d", plain.result.exit, structured.result.exit)
	}
	var report struct{ Sessions []map[string]any }
	if json.Unmarshal([]byte(structured.result.stdout), &report) != nil || len(report.Sessions) != 1 {
		return errors.New("fresh status lost the original session")
	}
	entry := report.Sessions[0]
	want := map[string]any{"path": l.path, "state": "stored", "records": 1, "missing": []any{}}
	if entry["id"] != l.id || entry["cleanup"] != "complete" || !sameJSON(object(entry["egress"])["refusal_log"], want) {
		return errors.New("fresh status has no stored one-record refusal report after cleanup")
	}
	if !strings.Contains(plain.result.stdout, l.path) || !strings.Contains(plain.result.stdout, "stored") || !strings.Contains(plain.result.stdout, "1") {
		return errors.New("plain status omits the same stored path/count")
	}
	for _, result := range []egressResult{plain, structured} {
		if !strings.Contains(result.result.stdout, egressLimitations[3]) || strings.Contains(result.result.stdout+result.result.stderr, "run.proxy-") {
			return errors.New("status lost the log limitation or invented a proxy failure")
		}
	}
	after, err := doctorTree(w.egressStatePath())
	if err != nil || !reflect.DeepEqual(after, l.beforeStatus) {
		return errors.New("fresh status changed protected state")
	}
	return nil
}

func (w *world) egressLifeEndingPreserved() error {
	l, n := w.egress.lifeNative, w.egress.native
	edit, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-28-edit"))
	if err != nil || string(edit) != l.edit {
		return errors.New("workload checkout edit was lost")
	}
	if err := w.approvalRecordUnchanged(); err != nil {
		return err
	}
	if err := n.endpointsIntact(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, network := range n.networks {
		if _, err := n.cli.NetworkInspect(ctx, network.id, client.NetworkInspectOptions{}); err != nil {
			return errors.New("unrelated control network was removed")
		}
	}
	for _, r := range l.journal {
		if r.method != "DELETE" && r.method != "POST" {
			continue
		}
		for _, endpoint := range n.endpoints {
			if strings.Contains(r.path, endpoint.id) {
				return errors.New("product changed an unrelated endpoint")
			}
		}
		for _, network := range n.networks {
			if strings.Contains(r.path, network.id) {
				return errors.New("product changed an unrelated network")
			}
		}
	}
	return w.runValidateSnapshot(l.run.snapshot, "1")
}
