package features

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func (w *world) controlExclusionSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a control-state fixture configured to pause P inside publication before its first selected-tree mutation$`, func() error {
		if err := w.controlSetup("absent", "below W root", "distinct daemons"); err != nil {
			return err
		}
		f := w.control
		if err := os.MkdirAll(filepath.Dir(f.p.approval.record), 0700); err != nil {
			return err
		}
		f.p.doctor.env["SLICE_21_TRACE_READY"] = filepath.Join(f.parent, "slice-21-publisher-ready")
		f.p.doctor.env["SLICE_21_TRACE_RELEASE"] = filepath.Join(f.parent, "slice-21-publisher-release")
		worker := f.workers.worlds["W"]
		worker.doctor.env["SLICE_21_BOUNDARY"] = "before admission"
		worker.doctor.env["SLICE_21_READY"] = filepath.Join(f.parent, "slice-21-admission-ready")
		worker.doctor.env["SLICE_21_RELEASE"] = filepath.Join(f.parent, "slice-21-admission-release")
		return nil
	})
	sc.Step(`^an independent W run finishes preflight before P holds publication and then attempts admission from another state root$`, func() { w.control.exclusionRace() })
	sc.Step(`^W reaches admission but obtains no registration or create authority while P holds publication exclusion$`, func() error {
		f := w.control
		if f.err != nil {
			return f.err
		}
		events, err := controlEvents(f.workers.first.process.journal)
		if err != nil {
			return err
		}
		attempt := false
		for _, e := range events[f.admissionEvents:] {
			if e.Op == "flock" && (e.Path == filepath.Join(f.coordination(), "control.lock") || strings.HasSuffix(e.Path, "/admission")) && e.Flags&2 != 0 {
				attempt = true
				if e.Phase == "end" && e.Success {
					return errors.New("worker acquired admission while approval publication was held")
				}
			}
		}
		if !attempt {
			return errors.New("worker never attempted cross-process admission")
		}
		return f.noContenderAuthority()
	})
	sc.Step(`^the waiting W invocation is cancelled before the fixture releases P publication$`, func() {
		f := w.control
		r := f.workers.first
		pid := r.pid()
		process, err := os.FindProcess(pid)
		f.err = errors.Join(f.err, err)
		if err == nil {
			f.err = errors.Join(f.err, process.Signal(os.Interrupt))
			_ = process.Release()
		}
		r.process.wait()
		f.err = errors.Join(f.err, r.process.err)
		f.err = errors.Join(f.err, os.WriteFile(f.p.doctor.env["SLICE_21_TRACE_RELEASE"], []byte("continue"), 0600))
		f.publisher.wait()
		f.err = errors.Join(f.err, f.publisher.err)
		f.p.exit, f.p.stdout, f.p.stderr = f.publisher.exit, string(f.publisher.out.bytes()), string(f.publisher.errout.bytes())
	})
	sc.Step(`^P publishes one complete matching approval after a durable protected location claim and W has no worker or launch continuation$`, func() error {
		f := w.control
		return errors.Join(f.err, f.p.approvalSuccess(), f.p.approvalStored("1"), f.locationDurable(f.p, []*recoveryProcess{f.publisher}), f.noContenderAuthority())
	})
	sc.Step(`^P publication changes only its recorded location and approval while unrelated state remains intact$`, func() error {
		f := w.control
		now, err := f.inventory()
		if err != nil {
			return err
		}
		managed := filepath.Dir(filepath.Dir(filepath.Dir(f.p.approval.record)))
		binding := filepath.Join(f.coordination(), approvalHash(managed)+".json")
		for path, value := range now {
			if path == f.p.approval.record || path == binding {
				continue
			}
			if old, ok := f.before[path]; ok && old == value {
				continue
			}
			info, err := os.Lstat(path)
			if err == nil && info.IsDir() && (path == f.coordination() || strings.HasPrefix(binding, path+"/")) {
				continue
			}
			return errors.New("publication changed unrelated state at " + path)
		}
		for path := range f.before {
			if _, ok := now[path]; !ok {
				return errors.New("publication deleted unrelated state")
			}
		}
		return nil
	})
}

func controlAwaitFile(path string) error {
	return overlapAwait("control-state transaction barrier", 12*time.Second, func() bool { data, _ := os.ReadFile(path); return string(data) == "ready" })
}

func (f *controlFixture) exclusionRace() {
	p, worker := f.p, f.workers.worlds["W"]
	f.p = worker
	r := &overlapRun{w: worker, engine: f.workers.engines["W"]}
	r.process = f.traceCommand("run", "--", "slice-21-workload")
	f.workers.first = r
	f.workers.runs = append(f.workers.runs, r)
	f.p = p
	f.err = errors.Join(f.err, controlAwaitFile(worker.doctor.env["SLICE_21_READY"]))
	events, err := controlEvents(r.process.journal)
	f.err = errors.Join(f.err, err)
	f.admissionEvents = len(events)
	f.publisher = f.traceCommand("approve", "--json", "--fingerprint", p.approval.fingerprint)
	f.err = errors.Join(f.err, controlAwaitFile(p.doctor.env["SLICE_21_TRACE_READY"]))
	f.before, err = f.inventory()
	f.err = errors.Join(f.err, err)
	f.err = errors.Join(f.err, os.WriteFile(worker.doctor.env["SLICE_21_RELEASE"], []byte("continue"), 0600))
	f.err = errors.Join(f.err, overlapAwait("contender's actual admission lock attempt", 10*time.Second, func() bool {
		events, _ := controlEvents(r.process.journal)
		for _, e := range events[f.admissionEvents:] {
			if e.Op == "flock" && e.Phase == "end" && (e.Path == filepath.Join(f.coordination(), "control.lock") || strings.HasSuffix(e.Path, "/admission")) && e.Flags&2 != 0 {
				return true
			}
		}
		return false
	}))
}

func (f *controlFixture) noContenderAuthority() error {
	r := f.workers.first
	r.discover()
	if f.workers.creates(r) != 0 || r.worker != "" || r.id != "" {
		return errors.New("waiting contender obtained registration or worker authority")
	}
	path := filepath.Join(r.w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", r.w.recoveryIndexKey())
	entries, err := os.ReadDir(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(entries) != 0 {
		return errors.New("cancelled contender retained public registration " + strconv.Itoa(len(entries)))
	}
	return nil
}
