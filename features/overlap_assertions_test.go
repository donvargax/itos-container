package features

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

func (f *overlapFixture) refused(r *overlapRun) error {
	if f.err != nil {
		return f.err
	}
	if r == nil {
		return errors.New("nested contender was not invoked")
	}
	got := r.result()
	if !r.ended() || got.exit != 1 || got.stdout != "" || !strings.Contains(got.stderr, "run.checkout-overlap") {
		return fmt.Errorf("nested contender must report run.checkout-overlap, exit 1 and empty stdout before create; got exit %d, created=%t, stderr=%q", got.exit, r.worker != "", got.stderr)
	}
	if r.process.err != nil {
		return r.process.err
	}
	text := strings.ToLower(got.stderr)
	if !strings.Contains(text, "itos-container:") || !strings.Contains(text, "overlap") || !strings.Contains(text, "checkout") ||
		(!strings.Contains(text, "finish") && !strings.Contains(text, "stop") && !strings.Contains(text, "inspect")) || !strings.Contains(text, "original") {
		return errors.New("overlap diagnostic lacks prefixed explanation and original-checkout recovery guidance")
	}
	for _, forbidden := range []string{"approve", "--force", "wait-for-exit", r.marker, "slice-16-workload", doctorCanary} {
		if strings.Contains(text, strings.ToLower(forbidden)) {
			return errors.New("overlap diagnostic suggests unsafe recovery or exposes workload/engine data")
		}
	}
	if r.worker != "" {
		return errors.New("refused invocation created a worker")
	}
	if r.engine != nil {
		for _, q := range r.engine.journal() {
			if q.pid == r.pid() && q.method != "GET" && q.method != "HEAD" {
				return errors.New("refused invocation mutated the engine")
			}
		}
	} else {
		a := r.w.offline.api
		a.mu.Lock()
		defer a.mu.Unlock()
		for _, q := range a.requests {
			if q.Method != "GET" && q.Method != "HEAD" {
				return errors.New("refused native invocation mutated the engine")
			}
		}
		if len(a.created) != 0 {
			return errors.New("refused native invocation created a worker")
		}
		if _, err := os.Stat(filepath.Join(r.w.doctor.checkout, r.marker)); !os.IsNotExist(err) {
			return errors.New("refused native workload left its marker")
		}
	}
	return nil
}

func (r *overlapRun) unchanged() error {
	for path, before := range map[string]sessionRecord{r.record: r.public, r.intent: r.owned, r.w.sessionSnapshotPath(): r.snapshot} {
		now, err := sessionReadRecord(path)
		if err != nil || before.Bytes == "" || now != before {
			return errors.New("contender changed the blocker's protected ownership or snapshot")
		}
	}
	return nil
}

func (f *overlapFixture) live(r *overlapRun) error {
	if f.err != nil {
		return f.err
	}
	if r == nil || !offlineHex(r.worker) || !r.active() {
		return errors.New("admitted workload is not independently active with its original full ID")
	}
	if f.native {
		before := r.heartbeat
		if before == 0 {
			data, _ := os.ReadFile(filepath.Join(r.w.doctor.checkout, r.marker))
			_, _ = fmt.Sscan(string(data), &before)
		}
		if err := overlapAwait("native heartbeat advances", 3*time.Second, func() bool {
			data, _ := os.ReadFile(filepath.Join(r.w.doctor.checkout, r.marker))
			n := 0
			_, _ = fmt.Sscan(string(data), &n)
			if n > before && r.active() {
				r.heartbeat = n
				return true
			}
			return false
		}); err != nil {
			return err
		}
	}
	return nil
}

func (f *overlapFixture) complete(r *overlapRun) error {
	if f.err != nil {
		return f.err
	}
	e := r.stopEntry
	if r.stop == nil || e.ID != r.id || e.State != "stopped" || e.Resource != "absent" || e.Cleanup != "complete" || e.Project != r.w.approval.project || e.Fingerprint != r.w.approval.fingerprint || e.Retry != nil || e.StopCandidate {
		return errors.New("product stop did not confirm complete cleanup of the original session")
	}
	removed, confirmed := false, false
	if r.engine != nil {
		for _, q := range r.engine.journal() {
			if q.method == "DELETE" && q.path == "/containers/"+r.worker && q.status == 204 {
				removed = true
			}
			if removed && q.method == "GET" && q.path == "/containers/"+r.worker+"/json" && q.status == 404 {
				confirmed = true
			}
		}
		r.engine.mu.Lock()
		c := r.engine.workers[r.worker]
		absent := c != nil && c.removed
		r.engine.mu.Unlock()
		if !absent {
			return errors.New("full worker ID still exists after product cleanup")
		}
	} else {
		a := r.w.offline.api
		a.mu.Lock()
		for _, q := range a.requests {
			if q.Method == "DELETE" && q.Path == "/containers/"+r.worker && q.Status == 204 {
				removed = true
			}
			if removed && q.Method == "GET" && q.Path == "/containers/"+r.worker+"/json" && q.Status == 404 {
				confirmed = true
			}
		}
		a.mu.Unlock()
		if r.w.doctor.real.read("/containers/"+r.worker+"/json", nil, 404) != nil {
			return errors.New("native full worker ID remains after cleanup")
		}
	}
	if !removed || !confirmed {
		return errors.New("product did not freshly confirm full-ID absence after removal")
	}
	var exit any
	if e.WorkloadExit != nil {
		exit = *e.WorkloadExit
	}
	public := map[string]any{"schema": 1, "id": r.id, "project": r.w.approval.project, "intent": filepath.ToSlash(strings.TrimPrefix(r.intent, filepath.Join(r.w.doctor.env["XDG_STATE_HOME"], "itos-container")+string(os.PathSeparator))), "state": "stopped", "workload_exit": exit, "last_problem": nil}
	intent, err := sessionDecodeRecord(r.owned.Bytes)
	if err != nil {
		return err
	}
	intent["stage"], intent["container_id"] = "absent", r.worker
	if err = recoveryDurableFile(r.intent, intent, []*recoveryProcess{r.stop}); err != nil {
		return err
	}
	return recoveryDurableFile(r.record, public, []*recoveryProcess{r.stop})
}

func (f *overlapFixture) checkpoint(r *overlapRun) {
	if r.active() && !r.paused {
		f.remember(overlapAwait("public running state", 3*time.Second, func() bool {
			r.discover()
			record, _ := sessionReadRecord(r.record)
			data, _ := sessionDecodeRecord(record.Bytes)
			return data["state"] == "running" || r.ended()
		}))
	}
	f.remember(r.capture())
}

func (f *overlapFixture) prepared() error {
	if f.err != nil {
		return f.err
	}
	r := f.first
	if r == nil || !r.paused || r.ended() || !offlineHex(r.id) {
		return errors.New("first run did not remain at its publication barrier")
	}
	syncs, err := recoveryReadSyncs(r.process.journal)
	if err != nil {
		return err
	}
	if !recoverySynced(r.public, r.record, syncs) || !recoverySynced(r.owned, r.intent, syncs) {
		return errors.New("barrier preceded durable registration or intent publication")
	}
	return nil
}

func (f *overlapFixture) pausedOwned() error {
	if err := f.prepared(); err != nil {
		return err
	}
	r := f.first
	r.engine.mu.Lock()
	defer r.engine.mu.Unlock()
	c := r.engine.workers[r.worker]
	if c == nil || c.removed || object(c.value["State"])["Status"] != "created" || r.result().stdout != "" {
		return errors.New("post-create barrier did not retain exactly one unstarted owned worker")
	}
	owned, err := sessionDecodeRecord(r.owned.Bytes)
	if err != nil || owned["stage"] != "owned" || owned["container_id"] != r.worker {
		return errors.New("post-create barrier lost durable full-ID ownership")
	}
	return nil
}

func (f *overlapFixture) staleStatus() error {
	if err := f.live(f.contender); err != nil {
		return err
	}
	r := f.first
	p := r.w.recoveryStart("status", "--json")
	p.wait()
	report, err := recoveryReadReport(p)
	if err != nil {
		return err
	}
	if len(report.Sessions) != 1 {
		return errors.New("original status lost its completed session")
	}
	e := report.Sessions[0]
	if e.ID != r.id || e.State != "stopped" || e.Resource != "absent" || e.Cleanup != "complete" || e.WorkloadExit != nil {
		return errors.New("stale continuation changed completion or invented an unstarted workload exit")
	}
	return f.contender.unchanged()
}

func (f *overlapFixture) allRefused() error {
	for _, r := range f.refusals {
		if err := f.refused(r); err != nil {
			return err
		}
	}
	return nil
}
func (f *overlapFixture) controlsLive() error {
	for _, r := range f.controls {
		if err := f.live(r); err != nil {
			return err
		}
	}
	return f.preserved()
}

func (f *overlapFixture) sameOutcomes() error {
	for _, r := range []*overlapRun{f.first, f.second} {
		if err := f.complete(r); err != nil {
			return err
		}
		if r.stopEntry.WorkloadExit != nil && *r.stopEntry.WorkloadExit != 7 {
			return errors.New("cleanup replaced observed workload outcome")
		}
		before, _ := sessionDecodeRecord(r.public.Bytes)
		if before["workload_exit"] != nil && !reflect.DeepEqual(before["workload_exit"], r.stopEntry.WorkloadExit) {
			return errors.New("cleanup lost prior workload result")
		}
	}
	return f.preserved()
}
