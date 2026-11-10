package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"time"
)

func (w *world) lifecycleLossAction() {
	l, r := w.loss, w.recovery
	r.run = w.recoveryStartWithin(150*time.Second, "run", "--", "/workspace/slice-14-probe", "loss", "/workspace/slice-15-challenge.json")
	// Every product refusal, missing barrier and early exit remains data for
	// Then. Nothing invokes a second worker or substitutes harness cleanup.
	l.err = w.lossAwait(125*time.Second, true, func() (bool, error) {
		if !w.offline.api.running() {
			return false, nil
		}
		w.recoveryDiscover()
		return offlineHex(r.worker) && offlineHex(r.id), r.err
	})
	if l.err != nil {
		return
	}
	state := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")
	files := w.offline.isolation.config.Files
	files["foreign-ownership"] = files["ownership"]
	files["ownership"] = w.sessionRecordPath()
	files["snapshot"] = filepath.Join(state, "snapshots", w.approval.project, "1.json")
	files["registration"] = filepath.Join(state, "sessions", w.recoveryIndexKey(), r.id, "record.json")
	files["admission"] = w.admissionPath(r.id)
	if err := w.lossCollectRound(true); err != nil {
		l.err = err
		return
	}
	if err := r.run.killController(); err != nil {
		l.err = err
		return
	}
	r.run.wait() // the independent tracer reaps its child, reporting SIGKILL
	if r.run.err != nil || r.run.exit != 137 {
		l.err = errors.Join(r.run.err, fmt.Errorf("controller loss was not confirmed (exit %d)", r.run.exit))
		return
	}
	l.dead = time.Now()
	// This call generates the nonce only after confirmed death. No stdin or
	// attach output is used to challenge or observe the surviving worker.
	l.err = w.lossCollectRound(false)
	r.status = w.recoveryStart("status", "--json")
	r.status.wait()
	l.err = errors.Join(l.err, w.lossCollectRound(false))
	report, err := recoveryReadReport(r.status)
	if err == nil && len(report.Sessions) == 1 {
		retry := report.Sessions[0].Retry
		if reflect.DeepEqual(retry, []string{"itos", "container", "stop", r.id}) {
			r.retry = append([]string{}, retry[2:]...)
			a := w.offline.api
			a.mu.Lock()
			l.stopBegin = len(a.requests)
			a.mu.Unlock()
			p := w.recoveryStart(r.retry...)
			p.wait()
			r.stops = append(r.stops, p)
		}
	}
	r.after = w.recoveryStart("status", "--json")
	r.after.wait()
}

func (w *world) lossAwait(limit time.Duration, controllerLive bool, observe func() (bool, error)) error {
	timer := time.NewTimer(limit)
	defer timer.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	var done <-chan struct{}
	if controllerLive {
		done = w.recovery.run.done
	}
	for {
		select {
		case <-done:
			return fmt.Errorf("controller exited before its barrier (exit %d): %s", w.recovery.run.exit, w.recovery.run.errout.bytes())
		case <-timer.C:
			return errors.New("native loss probe exceeded its finite observation deadline")
		case <-ticker.C:
			ready, err := observe()
			if err != nil {
				return err
			}
			if ready {
				return nil
			}
		}
	}
}

func (w *world) lossCollectRound(controllerLive bool) error {
	s, l := w.offline.isolation, w.loss
	r := &lossRound{nonce: isolationNonce()}
	l.rounds = append(l.rounds, r)
	if err := s.lossAliases(r.nonce); err != nil {
		r.err = err
		return err
	}
	r.before = w.lossControls(r.nonce + "b")
	if r.before.err != nil {
		r.err = r.before.err
		return r.err
	}
	c := w.lossConfig(r.nonce)
	path := w.lossChallengePath()
	if err := os.WriteFile(path+".next", approvalJSON(c), 0600); err != nil {
		return err
	}
	r.released = time.Now()
	if err := os.Rename(path+".next", path); err != nil {
		return err
	}
	var err error
	r.challenge, err = sessionReadRecord(path)
	if err != nil {
		return err
	}
	r.err = w.lossAwait(40*time.Second, controllerLive, func() (bool, error) {
		// Independently observe the native primary, even while no report exists.
		if err := w.lossObserveWorker(r); err != nil {
			return false, err
		}
		b, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-15-report"))
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		reports, err := isolationReports(b)
		if err != nil {
			return false, err
		}
		if reports[0].Nonce != r.nonce {
			return false, nil
		}
		if err = lossReportInventory(reports, r.nonce, r.start); err != nil {
			return false, err
		}
		r.reports, r.collected = reports, time.Now()
		return true, nil
	})
	r.after = w.lossControls(r.nonce + "a")
	r.err = errors.Join(r.err, r.after.err, w.lossObserveWorker(r))
	snapshot := s.config.Files["snapshot"]
	r.snapshot, err = sessionReadRecord(snapshot)
	r.err = errors.Join(r.err, err)
	input, err := sessionReadRecord(path)
	if err != nil || input != r.challenge {
		r.err = errors.Join(r.err, err, errors.New("worker changed its challenge input"))
	}
	return r.err
}

func (w *world) lossObserveWorker(r *lossRound) error {
	a := w.offline.api
	observed, err := a.inspect(w.recovery.worker)
	if err != nil {
		return err
	}
	a.mu.Lock()
	err = a.identity(observed, w.recovery.worker)
	if err == nil {
		r.anchor, err = a.observeMountAnchor(observed)
	}
	a.mu.Unlock()
	if err != nil {
		return err
	}
	r.intent, err = sessionReadRecord(w.sessionRecordPath())
	if err != nil {
		return err
	}
	state := object(observed["State"])
	pid, err := strconv.Atoi(fmt.Sprint(state["Pid"]))
	if err != nil || state["Running"] != true || state["StartedAt"] == nil {
		return errors.New("original native worker is not running")
	}
	start := isolationProcessStart(pid)
	if start == "" {
		return errors.New("native worker has no live process start identity")
	}
	if len(w.loss.rounds) > 1 {
		first := w.loss.rounds[0]
		if first.pid != pid || first.start != start || object(first.boundary["State"])["StartedAt"] != state["StartedAt"] {
			return errors.New("surviving worker was restarted or replaced")
		}
	}
	if r.start != "" && (r.start != start || r.pid != pid) {
		return errors.New("worker changed identity during a round")
	}
	r.pid, r.start, r.boundary = pid, start, observed
	return nil
}

func lossReportInventory(reports []isolationReport, nonce, start string) error {
	want := map[string]int{"round": 1, "identity": 1, "filesystem": 1, "host": 1, "network": 2, "dns": 2, "loopback": 1}
	if nonce == "" || start == "" || len(reports) != 9 {
		return errors.New("incomplete fresh loss-round inventory")
	}
	for _, r := range reports {
		if r.Nonce != nonce || want[r.Kind] == 0 || r.Error != "" {
			return errors.New("duplicate, foreign or stale loss-round report")
		}
		want[r.Kind]--
		if r.Kind == "round" || r.Kind == "identity" {
			if r.Values["pid"] != "1" || r.Values["start"] != start {
				return errors.New("report does not identify the original native primary")
			}
		}
	}
	for _, n := range want {
		if n != 0 {
			return errors.New("missing required loss-round operation")
		}
	}
	return nil
}

func lossRecord(data sessionRecord) map[string]any {
	var record map[string]any
	_ = json.Unmarshal([]byte(data.Bytes), &record)
	return record
}
