package features

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func (w *world) recoveryRaceSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a recovery fixture with a barrier before "([^"]*)" and substitution "([^"]*)"$`, func(mutation, change string) error {
		resource := "running"
		if mutation == "remove" {
			resource = "created"
		}
		if mutation == "SIGKILL" {
			resource = "running ignoring SIGTERM"
		}
		if err := w.recoverySeed("failed", resource, "unknown"); err != nil {
			return err
		}
		l := w.recoveryLocal
		l.mutation, l.substitution = mutation, change
		l.inspectReady, l.inspectRelease = make(chan struct{}), make(chan struct{})
		return nil
	})
	sc.Step(`^stop reaches the barrier and the fixture substitutes that authority before releasing it$`, func() error {
		l := w.recoveryLocal
		p := w.recoveryStart("stop", "--json", l.id)
		l.barrierReached = recoveryBarrier(l.inspectReady, p)
		if l.barrierReached {
			w.session.api.mu.Lock()
			err := w.recoverySubstitute()
			w.session.api.mu.Unlock()
			close(l.inspectRelease)
			if err != nil {
				return err
			}
		}
		return w.recoveryCollect(p)
	})
	sc.Step(`^stop returns recovery.not-owned with exit 1 before the barred mutation$`, func() error {
		l := w.recoveryLocal
		if !l.barrierReached {
			return fmt.Errorf("stop did not reach the earlier successful inspection")
		}
		if err := recoveryHasProblem(l.results[0], "recovery.not-owned", 1); err != nil {
			return err
		}
		for _, q := range w.session.api.requests {
			if l.mutation == "remove" && q.Method == "DELETE" || strings.Contains(q.Query, l.mutation) {
				return fmt.Errorf("stop crossed the substituted authority boundary")
			}
		}
		return nil
	})
	sc.Step(`^the replacement resource or path is untouched and incomplete recovery remains discoverable$`, func() error {
		if w.session.api.local[w.session.seedID] == nil {
			return fmt.Errorf("replacement resource was removed")
		}
		data, err := os.ReadFile(w.recoveryRecordPath())
		if err != nil {
			return err
		}
		var r map[string]any
		if json.Unmarshal(data, &r) != nil || r["state"] == "stopped" {
			return fmt.Errorf("substituted authority lost its incomplete record")
		}
		if w.recoveryLocal.substitution == "protected record replacement" && r["intent"] != "intents/"+w.approval.project+"/"+strings.Repeat("c", 64)+".json" {
			return fmt.Errorf("replacement record overwritten")
		}
		if w.recoveryLocal.substitution == "protected intent replacement" {
			data, err := os.ReadFile(w.sessionRecordPath())
			if err != nil {
				return err
			}
			if !recoveryJSONEqual(string(data), w.sessionExpectedRecord("owned", strings.Repeat("f", 64))) {
				return fmt.Errorf("replacement intent overwritten")
			}
		}
		return nil
	})
	sc.Step(`^a recovery fixture with fault "([^"]*)" at the first stop's completion boundary$`, func(fault string) error {
		if err := w.recoverySeed("failed", "created", "7"); err != nil {
			return err
		}
		w.recoveryLocal.fault = fault
		if fault == "lost remove response and inspection unavailable" {
			w.recoveryLocal.lostRemove = true
		}
		if fault == "controller death after removal" {
			w.recoveryLocal.afterRemoveReady, w.recoveryLocal.afterRemoveRelease = make(chan struct{}), make(chan struct{})
		}
		return nil
	})
	sc.Step(`^stop is attempted, the fault is cleared, a same-named foreign worker is seeded and a fresh process retries stop$`, func() error {
		l := w.recoveryLocal
		if l.fault == "controller death after removal" {
			ready := l.afterRemoveReady
			p := w.recoveryStart("stop", "--json", l.id)
			l.barrierReached = recoveryBarrier(ready, p)
			if l.barrierReached {
				if err := p.killController(); err != nil {
					return err
				}
				close(l.afterRemoveRelease)
			}
			if err := w.recoveryCollect(p); err != nil {
				return err
			}
		} else if l.lostRemove {
			if err := w.recoveryTracedInvocation("stop", "--json", l.id); err != nil {
				return err
			}
		} else {
			if err := w.recoveryProbeInvoke("stop", l.id); err != nil {
				return err
			}
		}
		w.session.api.mu.Lock()
		l.lostRemove, l.unavailable = false, false
		w.sessionReplacement()
		w.session.api.mu.Unlock()
		return w.recoveryTracedInvocation("stop", "--json", l.id)
	})
	sc.Step(`^the first stop reaches recorded-ID removal before reporting recovery.temporary or losing its controller at that boundary without reporting successful durable completion$`, func() error {
		l := w.recoveryLocal
		r := l.results[0]
		if l.fault == "controller death after removal" {
			if !l.barrierReached || r.exit == 0 {
				return fmt.Errorf("controller did not die after removal")
			}
		} else if err := recoveryHasProblem(r, "recovery.temporary", 75); err != nil {
			return err
		}
		for _, q := range w.session.api.requests {
			if q.Method == "DELETE" && q.Path == "/containers/"+w.session.seedID {
				return nil
			}
		}
		return fmt.Errorf("first stop never removed the recorded worker")
	})
	sc.Step(`^the later stop confirms the original full ID absent and publishes durable completion while preserving the replacement$`, func() error {
		if _, err := w.recoveryLocalEntry(w.recoveryLocal.results[1], "stopped", "absent", "complete", "7"); err != nil {
			return err
		}
		return w.recoveryLocalDurable()
	})
	sc.Step(`^the session remains discoverable throughout with its original observed or unknown workload exit$`, w.recoveryImmutable)
	sc.Step(`^a recovery fixture with a running owned worker and a first-stop removal barrier$`, func() error {
		if err := w.recoverySeed("running", "running", "unknown"); err != nil {
			return err
		}
		w.recoveryLocal.removeReady, w.recoveryLocal.removeRelease = make(chan struct{}), make(chan struct{})
		return nil
	})
	sc.Step(`^two independent stop processes overlap and the first barrier is released$`, func() error {
		l := w.recoveryLocal
		ready := l.removeReady
		first := w.recoveryStart("stop", "--json", l.id)
		l.barrierReached = recoveryBarrier(ready, first)
		if !l.barrierReached {
			return w.recoveryCollect(first)
		}
		second := w.recoveryStart("stop", "--json", l.id)
		status := w.recoveryStart("status", "--json")
		// Wait for the independent second process to reach the session lock. The
		// held file itself proves exclusion; no sleep or fixture count is authority.
		if err := w.recoveryWaitPID(second); err != nil {
			return err
		}
		close(l.removeRelease)
		for _, p := range []*recoveryProcess{first, second, status} {
			if err := w.recoveryCollect(p); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^both stops complete with one serialized termination and removal sequence followed by confirmed absence$`, func() error {
		l := w.recoveryLocal
		if !l.barrierReached || len(l.results) != 3 {
			return fmt.Errorf("independent stops did not overlap at removal")
		}
		for _, r := range l.results[:2] {
			if _, err := w.recoveryLocalEntry(r, "stopped", "absent", "complete", "unknown"); err != nil {
				return err
			}
		}
		if !l.processes[0].finished.After(l.processes[1].started) {
			return fmt.Errorf("stops did not overlap")
		}
		return w.recoveryLocalSignals()
	})
	sc.Step(`^the waiting stop reloads durable completion without a second signal remove or completion rewrite$`, func() error {
		if err := w.recoveryLocalDurable(); err != nil {
			return err
		}
		current, err := sessionReadRecord(w.recoveryRecordPath())
		if err != nil {
			return err
		}
		events, err := recoveryReadSyncs(w.recoveryLocal.processes[1].journal)
		if err != nil {
			return err
		}
		for _, e := range events {
			if !e.Directory && strings.Contains(e.Path, "/sessions/") && e.Data != "" && e.Inode != current.Inode {
				return fmt.Errorf("waiting stop rewrote completion")
			}
		}
		return nil
	})
	sc.Step(`^a concurrent status returns a consistent session or recovery.temporary within its budget without writing state$`, func() error {
		l := w.recoveryLocal
		r := l.results[2]
		if r.exit == 75 {
			return recoveryHasProblem(r, "recovery.temporary", 75)
		}
		if _, err := w.recoveryLocalEntry(r, "stopped", "absent", "complete", "unknown"); err != nil {
			return err
		}
		events, err := recoveryReadSyncs(l.processes[2].journal)
		if err != nil {
			return err
		}
		if len(events) != 0 {
			return fmt.Errorf("status repaired durability")
		}
		return nil
	})
	sc.Step(`^a recovery fixture with equal-basename projects A and B, two sessions in A and one in B$`, func() error {
		if err := w.recoverySeed("running", "running", "unknown"); err != nil {
			return err
		}
		if err := w.recoveryAdditional("c"); err != nil {
			return err
		}
		child := &world{root: w.root, bin: w.bin, dir: filepath.Join(w.dir, "project-b")}
		if err := os.Mkdir(child.dir, 0700); err != nil {
			return err
		}
		if err := child.recoverySeed("running", "running", "unknown"); err != nil {
			return err
		}
		w.recoveryLocal.second = child
		w.doctor.cleanup = append(w.doctor.cleanup, func() { _ = child.closeRecoveryNative(); _ = child.closeSession(); child.closeDoctor() })
		if filepath.Base(child.doctor.checkout) != filepath.Base(w.doctor.checkout) {
			return fmt.Errorf("fixture project basenames differ")
		}
		// The ID in B is distinct so an A lookup must refuse, not target A's ID.
		old := filepath.Dir(child.recoveryRecordPath())
		child.recoveryLocal.id = strings.Repeat("d", 64)
		if err := os.Rename(old, filepath.Dir(child.recoveryRecordPath())); err != nil {
			return err
		}
		data, err := os.ReadFile(child.recoveryRecordPath())
		if err != nil {
			return err
		}
		var record map[string]any
		_ = json.Unmarshal(data, &record)
		record["id"] = child.recoveryLocal.id
		if err = os.WriteFile(child.recoveryRecordPath(), approvalJSON(record), 0600); err != nil {
			return err
		}
		if err = child.recoveryCapture(); err != nil {
			return err
		}
		return w.recoveryCapture()
	})
	sc.Step(`^A's first stop is paused holding its session lock while A's second session is inspected and stopped and B is inspected$`, func() error {
		l := w.recoveryLocal
		l.removeReady, l.removeRelease = make(chan struct{}), make(chan struct{})
		ready := l.removeReady
		first := w.recoveryStart("stop", "--json", l.id)
		l.barrierReached = recoveryBarrier(ready, first)
		l.processes = append(l.processes, first)
		if !l.barrierReached {
			return w.recoveryCollect(first)
		}
		// Status is project-wide and may time out on A1; stopping A2 must still
		// proceed independently of that operation and the project transaction lock.
		status := w.recoveryStart("status", "--json")
		if err := w.recoveryTracedInvocation("stop", "--json", strings.Repeat("c", 64)); err != nil {
			return err
		}
		if err := l.second.recoveryInvocation("status", "--json"); err != nil {
			return err
		}
		if err := w.recoveryInvocation("stop", "--json", l.second.recoveryLocal.id); err != nil {
			return err
		}
		l.beforeRequests = len(l.second.session.api.requests)
		close(l.removeRelease)
		if err := w.recoveryCollect(first); err != nil {
			return err
		}
		return w.recoveryCollect(status)
	})
	sc.Step(`^A's second stop completes before the first lock is released and B lists only its own still-running session$`, func() error {
		l := w.recoveryLocal
		if !l.barrierReached || len(l.results) < 4 {
			return fmt.Errorf("first stop did not hold its session lock")
		}
		if _, err := recoveryLocalReport(l.results[0], 0); err != nil {
			return err
		}
		if !l.processes[1].finished.Before(l.processes[0].finished) {
			return fmt.Errorf("A2 was blocked behind A1")
		}
		_, err := l.second.recoveryLocalEntry(l.second.recoveryLocal.results[0], "running", "running", "pending", "unknown")
		return err
	})
	sc.Step(`^stopping B's ID from A reports recovery.not-owned with exit 1 and does not contact B's resource$`, func() error {
		if err := recoveryHasProblem(w.recoveryLocal.results[1], "recovery.not-owned", 1); err != nil {
			return err
		}
		if len(w.recoveryLocal.second.session.api.requests) != w.recoveryLocal.beforeRequests {
			return fmt.Errorf("A contacted B")
		}
		return nil
	})
	sc.Step(`^releasing A's first stop preserves B's worker and both projects' checkout edits approvals and snapshots$`, func() error {
		l := w.recoveryLocal
		if err := l.second.recoveryUnchanged(); err != nil {
			return err
		}
		for _, path := range []string{w.approval.record, w.sessionSnapshotPath(), filepath.Join(w.doctor.checkout, "edit.txt")} {
			before := l.before[path]
			now, err := doctorTree(filepath.Dir(path))
			if err != nil || !reflect.DeepEqual(before, now[path]) {
				return fmt.Errorf("independent cleanup changed approvals or checkout")
			}
		}
		return nil
	})
}

func recoveryBarrier(ready <-chan struct{}, p *recoveryProcess) bool {
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	select {
	case <-ready:
		return true
	case <-p.done:
		return false
	case <-timer.C:
		return false
	}
}
func (w *world) recoveryCollect(p *recoveryProcess) error {
	p.wait()
	if p.err != nil {
		return p.err
	}
	w.recoveryLocal.processes = append(w.recoveryLocal.processes, p)
	w.recoveryLocal.results = append(w.recoveryLocal.results, doctorResult{exit: p.exit, stdout: string(p.out.bytes()), stderr: string(p.errout.bytes())})
	return nil
}
func (w *world) recoveryWaitPID(p *recoveryProcess) error {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(p.pid); err == nil {
			return nil
		}
		time.Sleep(time.Millisecond)
	}
	return fmt.Errorf("independent stop did not start")
}
func (w *world) recoverySubstitute() error {
	l := w.recoveryLocal
	switch l.substitution {
	case "socket inode replacement":
		path := strings.TrimPrefix(w.doctor.env["DOCKER_HOST"], "unix://")
		if err := os.Rename(path, path+".old"); err != nil {
			return err
		}
		listener, err := net.Listen("unix", path)
		if err != nil {
			return err
		}
		go func() { _ = w.session.api.server.Serve(listener) }()
	case "daemon identity replacement":
		w.doctor.engine.info["ID"] = "substituted-daemon"
	case "resource identity change":
		w.session.api.local[w.session.seedID]["Name"] = "/foreign"
	case "protected record replacement":
		data, err := os.ReadFile(w.recoveryRecordPath())
		if err != nil {
			return err
		}
		var record map[string]any
		_ = json.Unmarshal(data, &record)
		record["intent"] = "intents/" + w.approval.project + "/" + strings.Repeat("c", 64) + ".json"
		next := w.recoveryRecordPath() + ".next"
		if err = os.WriteFile(next, approvalJSON(record), 0600); err != nil {
			return err
		}
		return os.Rename(next, w.recoveryRecordPath())
	case "protected intent replacement":
		path := w.sessionRecordPath()
		next := path + ".next"
		if err := os.WriteFile(next, approvalJSON(w.sessionExpectedRecord("owned", strings.Repeat("f", 64))), 0600); err != nil {
			return err
		}
		return os.Rename(next, path)
	default:
		return fmt.Errorf("unknown substitution")
	}
	return nil
}
