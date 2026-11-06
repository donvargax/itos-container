package features

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type lifecycleStepRegistrar interface {
	Step(expression, function any)
}

// Native lifecycle cases remain @wip until the coordinator records evidence at
// this step commit. No Given invokes a product command or supplies a result.
func (w *world) lifecycleNativeSteps(sc lifecycleStepRegistrar) {
	sc.Step(`^a CI lifecycle checkout with a ready signal-handling probe that exits (\d+)$`, func(child int) error {
		return w.lifecycleFixture("handle", "", "", child, 0)
	})
	sc.Step(`^run launches the probe and the harness sends "([^"]*)" to the controller after the ready frame$`, func(signal string) {
		w.lifecycleSignalAction(signal, false)
	})
	sc.Step(`^the worker records exactly one "([^"]*)" and the product returns the observed exit (\d+) after confirmed cleanup$`, func(signal string, child int) error {
		return errors.Join(w.lifecycleReceipts(signal, child), w.lifecycleComplete())
	})
	sc.Step(`^run reports "run.cancelled" with the first signal separately from the authoritative workload exit and cleanup result$`, w.lifecycleCancelled)
	sc.Step(`^the engine journal has exactly one original signal request targeting only the freshly revalidated owned primary with no early SIGKILL or host process signal$`, func() error {
		return w.lifecycleSignals(false)
	})
	sc.Step(`^the final worker frame and checkout edit survive while the exact worker and temporary storage are absent$`, func() error {
		if !bytes.Contains(w.lifecycle.run.out.bytes(), []byte("slice-12:final\n")) {
			return errors.New("final worker output was lost")
		}
		data, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-12-temporary"))
		if err != nil || !strings.HasPrefix(string(data), "/tmp/slice-12-private-") {
			return errors.New("native worker did not exercise its temporary storage")
		}
		return errors.Join(w.lifecycleComplete(), w.lifecyclePreserved(), w.lifecycleOneLaunch())
	})

	sc.Step(`^a CI lifecycle checkout with a ready signal-recording probe that keeps running and a signal acknowledgement delayed (\d+) seconds$`, func(delay int) error {
		return w.lifecycleFixture("ignore", "", "", 17, delay)
	})
	sc.Step(`^run launches the probe and the harness sends "([^"]*)" followed by repeated SIGINT and SIGTERM during grace$`, func(signal string) {
		w.lifecycleSignalAction(signal, true)
	})
	sc.Step(`^the worker records only the first "([^"]*)" and native observations show it alive before the 10 second grace expires$`, func(signal string) error {
		if !w.lifecycle.aliveBeforeGrace {
			return errors.New("native primary was not observed alive before the original grace deadline")
		}
		return w.lifecycleReceipts(signal, 137)
	})
	sc.Step(`^the journal records exactly one original signal request and one SIGKILL request at the original grace deadline and the product returns the engine-observed exit 137$`, func() error {
		return errors.Join(w.lifecycleSignals(true), w.lifecycleReceipts(w.lifecycle.first, 137))
	})
	sc.Step(`^owned removal and durable absence complete within the original 30 second teardown budget$`, func() error {
		return errors.Join(w.lifecycleBound(w.lifecycle.signalled), w.lifecycleComplete(), w.lifecycleOneLaunch())
	})
	sc.Step(`^no unrelated process resource approval snapshot or checkout edit is changed$`, func() error {
		return errors.Join(w.lifecyclePreserved(), w.lifecycleNoHostSignals())
	})

	sc.Step(`^a CI lifecycle checkout with an exit-17 probe and an unavailable removal barrier$`, func() error {
		return w.lifecycleFixture("cleanup-signals", "", "", 17, 0)
	})
	sc.Step(`^run observes the probe exit and the harness sends SIGINT then SIGTERM during cleanup before restoring removal for an explicit stop$`, w.lifecycleCleanupSignals)
	sc.Step(`^run returns 17 within the original teardown budget with a separate incomplete cleanup result and usable recovery identity$`, func() error {
		if err := w.lifecycleResult(17); err != nil {
			return err
		}
		l := w.lifecycle
		if !l.barrierReached || !strings.Contains(string(l.run.errout.bytes()), "run.cleanup-") ||
			!bytes.Contains(l.run.errout.bytes(), []byte("itos container stop "+w.recovery.id)) {
			return errors.New("run did not retain a separately classified cleanup failure and its public retry")
		}
		return errors.Join(w.lifecycleBound(l.exitObserved), w.lifecycleRetained())
	})
	sc.Step(`^neither signal is forwarded to an already exited primary or causes duplicate unsafe removal or loss of the protected record$`, func() error {
		if len(lifecycleSignalRequests(w.lifecycle.journalAtReturn)) != 0 {
			return errors.New("run forwarded a signal after observing primary exit")
		}
		return errors.Join(w.lifecycleRetained(), w.lifecycleOneLaunch(), w.lifecycleNoHostSignals())
	})
	sc.Step(`^the later stop removes only the original full ID and confirms durable absence without restarting the worker$`, func() error {
		return errors.Join(w.lifecycleComplete(), w.lifecycleOneLaunch())
	})
	sc.Step(`^the checkout edit approvals snapshots and unrelated resources remain intact$`, w.lifecyclePreserved)

	sc.Step(`^a CI lifecycle checkout with a live marker probe and a product-only engine disconnect barrier$`, func() error {
		return w.lifecycleFixture("disconnect", "", "", 17, 0)
	})
	sc.Step(`^run launches the probe and loses its wait and inspection transport before any exit observation and a later status and stop follow restoration$`, w.lifecycleDisconnect)
	sc.Step(`^run returns 75 with "run.engine-lost" and an explicitly unknown workload outcome within bounded teardown$`, func() error {
		if err := w.lifecycleResult(75); err != nil {
			return err
		}
		output := string(w.lifecycle.run.errout.bytes())
		if !strings.Contains(output, "run.engine-lost") || !strings.Contains(output, "unknown") {
			return fmt.Errorf("engine loss did not report its typed failure and unknown workload outcome: %q", output)
		}
		return w.lifecycleBound(w.lifecycle.signalled)
	})
	sc.Step(`^the first result retains ownership and incomplete cleanup rather than a success exit or false absent claim$`, w.lifecycleRetained)
	sc.Step(`^native observations show the surviving worker unchanged and isolated during disconnection$`, func() error {
		l := w.lifecycle
		if l.control == nil || l.heartbeat <= 0 || l.later <= l.heartbeat {
			return errors.New("healthy control or independent post-disconnection worker progress is missing")
		}
		return w.lifecycleBoundary()
	})
	sc.Step(`^the later status and stop revalidate the original resource and confirm owned cleanup without rewriting unknown history as workload success$`, func() error {
		s, err := w.recoveryEntry(w.recovery.status, "failed", "running", "pending")
		if err != nil {
			return err
		}
		if s.WorkloadExit != nil || !s.StopCandidate || s.Controller != "absent" {
			return errors.New("fresh status replaced unknown workload history or lost its stop candidate")
		}
		s, err = w.recoveryEntry(w.recovery.after, "stopped", "absent", "complete")
		if err != nil {
			return err
		}
		if s.WorkloadExit != nil && *s.WorkloadExit == 0 {
			return errors.New("removal alone became successful workload history")
		}
		return errors.Join(w.lifecycleComplete(), w.lifecycleOneLaunch())
	})
	sc.Step(`^checkout edits approvals snapshots and unrelated resources remain intact$`, w.lifecyclePreserved)

	sc.Step(`^a CI lifecycle checkout with a ready signal-handling probe and stream condition "([^"]*)"$`, func(condition string) error {
		return w.lifecycleFixture("streams", condition, "", 17, 0)
	})
	sc.Step(`^run remains live under that condition and the harness sends SIGTERM while leaving the condition in place$`, w.lifecycleStreams)
	sc.Step(`^the probe records SIGTERM and the product completes owned teardown within 30 seconds of cancellation without waiting for the blocked stream$`, func() error {
		return errors.Join(w.lifecycleReceipts("SIGTERM", 17), w.lifecycleBound(w.lifecycle.signalled), w.lifecycleComplete())
	})
	sc.Step(`^when controller diagnostics are writable they name "run.cancelled" and SIGTERM separately from observed exit 17$`, func() error {
		if w.lifecycle.condition == "unread workload stderr" {
			return w.lifecycleBlockedState()
		}
		return w.lifecycleCancelled()
	})
	sc.Step(`^the quiet control remains running past both the call and startup durations until cancellation$`, w.lifecycleQuietVerified)
	sc.Step(`^any incomplete output drain is reported as a stream timeout without changing observed exit 17 or claiming complete delivery$`, func() error {
		l := w.lifecycle
		if l.condition == "unread workload stderr" {
			return w.lifecycleBlockedState()
		}
		if l.pipe != nil && (!l.blocked || !strings.Contains(string(l.run.errout.bytes()), "run.stream-timeout")) {
			return errors.New("blocked final output was not reported as a distinct stream timeout")
		}
		return w.lifecycleReceipts("SIGTERM", 17)
	})
	sc.Step(`^blocked diagnostic output cannot prevent termination and safe failure state survives when storage is available$`, func() error {
		return errors.Join(w.lifecycleBound(w.lifecycle.signalled), w.lifecycleBlockedState())
	})

	sc.Step(`^a CI lifecycle checkout with a trusted controller-kill barrier "([^"]*)"$`, func(barrier string) error {
		return w.lifecycleFixture("crash", "", barrier, 17, 0)
	})
	sc.Step(`^run reaches the barrier and the harness kills only its controller then invokes fresh status and stop after releasing the observation fault$`, w.lifecycleCrash)
	sc.Step(`^fresh status reports controller absent with the original recovery identity and pending or unknown cleanup without inventing workload success or completed cleanup$`, func() error {
		if err := w.lifecycleResult(137); err != nil {
			return err
		}
		if !w.lifecycle.barrierReached {
			return errors.New("controller did not reach its in-flight transition before SIGKILL")
		}
		report, err := recoveryReadReport(w.recovery.status)
		if err != nil {
			return err
		}
		if len(report.Sessions) != 1 {
			return errors.New("fresh status lost or duplicated the interrupted session")
		}
		s := report.Sessions[0]
		if s.ID != w.recovery.id || s.Project != w.approval.project || s.Fingerprint != w.approval.fingerprint ||
			s.Controller != "absent" || !s.StopCandidate || (s.Cleanup != "pending" && s.Cleanup != "unknown") ||
			(s.WorkloadExit != nil && *s.WorkloadExit == 0) {
			return errors.New("fresh status invented successful work/completion or lost original recovery authority")
		}
		return w.lifecycleRetained()
	})
	sc.Step(`^fresh stop reconciles only the original intent daemon snapshot and concrete resource identity and confirms durable absence$`, w.lifecycleComplete)
	sc.Step(`^no replacement create start name-only removal or unrelated resource mutation is attempted$`, func() error {
		return errors.Join(w.lifecycleOneLaunch(), w.lifecyclePreserved())
	})
	sc.Step(`^pre-start interruptions execute no worker instruction and any already running worker retains its offline boundary until removal$`, func() error {
		if w.lifecycle.barrier == "after accepted start before reply" {
			return w.lifecycleBoundary()
		}
		return w.lifecyclePreserved()
	})
}

func (w *world) lifecycleBlockedState() error {
	l := w.lifecycle
	if l.condition != "unread workload stderr" {
		return w.lifecycleSafeState()
	}
	if !l.blocked {
		return errors.New("diagnostic pipe never reached independently measured backpressure")
	}
	var record map[string]any
	if json.Unmarshal([]byte(l.firstRecord.Bytes), &record) != nil || record["last_problem"] == nil {
		return errors.New("unread diagnostics left no protected failure state")
	}
	data, _ := json.Marshal(record["last_problem"])
	if !bytes.Contains(data, []byte("stream-timeout")) && !bytes.Contains(data, []byte("cancelled")) {
		return errors.New("protected failure state does not explain the blocked cancellation/drain")
	}
	return w.lifecycleSafeState()
}

func (w *world) lifecycleQuietVerified() error {
	l := w.lifecycle
	if l.condition != "quiet workload and stdin" {
		if l.quiet == nil || l.quiet.elapsed < 125*time.Second {
			return errors.New("same-path quiet control did not run")
		}
		return l.quiet.err
	}
	if l.ready.IsZero() || l.signalled.Sub(l.ready) < 125*time.Second {
		return errors.New("quiet work did not remain live beyond call and startup durations")
	}
	return l.err
}
