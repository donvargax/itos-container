package features

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
)

func anchorsProblem(p *recoveryProcess, rule string, code int) error {
	if p == nil {
		return errors.New("fresh refusal command did not run")
	}
	p.wait()
	var result recoveryReport
	if p.err != nil || p.exit != code || json.Unmarshal(p.out.bytes(), &result) != nil || result.Schema != 1 || result.OK {
		return errors.New("fresh command did not report the required structured refusal")
	}
	for _, problem := range result.Problems {
		if problem.Rule == rule && problem.Exit == code {
			return nil
		}
	}
	return errors.New("recovery refusal lacks the required problem rule and classification")
}

func (w *world) anchorsForeignRefused() error {
	if w.recovery.err != nil {
		return w.recovery.err
	}
	if !w.anchorsFailedRelease() || w.anchors.foreign == nil {
		return errors.New("product never reached owned anchor release after confirmed worker absence")
	}
	if len(w.recovery.stops) != 1 {
		return errors.New("initial owned stop missing")
	}
	if err := anchorsProblem(w.recovery.stops[0], "recovery.temporary", 75); err != nil {
		return err
	}
	for _, p := range []*recoveryProcess{w.anchors.foreignStatus, w.anchors.foreignStop} {
		if err := anchorsProblem(p, "recovery.not-owned", 1); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) anchorsForeignPreserved() error {
	s, a := w.anchors, w.offline.api
	if s.foreign == nil {
		return errors.New("foreign native replacement was not established")
	}
	observed, err := a.anchorInspect(s.foreign)
	if err != nil || !reflect.DeepEqual(observed, s.foreign.observed) {
		return errors.New("foreign same-name volume changed")
	}
	if w.doctor.real.read("/containers/"+w.recovery.worker+"/json", nil, 404) != nil {
		return errors.New("original worker remains")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, q := range a.requests[s.foreignIndex:] {
		if anchorsMutates(q) {
			return errors.New("product attempted to mutate or adopt a foreign replacement")
		}
	}
	for _, p := range []*recoveryProcess{s.foreignStatus, s.foreignStop} {
		entry, err := w.anchorsEntry(p, 1)
		if err != nil {
			return err
		}
		if entry.Anchor != "unknown" || entry.Cleanup == "complete" || entry.State == "stopped" {
			return errors.New("foreign replacement was reported as completed cleanup")
		}
	}
	record, err := sessionReadRecord(w.sessionRecordPath())
	if err != nil {
		return err
	}
	if strings.Contains(record.Bytes, "slice-17-foreign-") {
		return errors.New("protected ownership adopted the foreign replacement")
	}
	return nil
}

func (w *world) anchorsPairCompleted() error {
	r, s := w.recovery, w.anchors
	if !s.reached || len(r.stops) != 3 || !r.stops[0].finished.After(r.stops[1].started) {
		return errors.New("two stops did not overlap at the original anchor release")
	}
	if err := w.anchorsCompleted(); err != nil {
		return err
	}
	other := r.second
	if other.anchors.later.Sequence <= other.anchors.initial.Sequence {
		return errors.New("independent worker stopped making progress while A was parked")
	}
	return other.anchorsProbeCheck(other.anchors.later)
}

func (w *world) anchorsPairStatus() error {
	other := w.recovery.second
	if _, err := other.anchorsOwned(); err != nil {
		return err
	}
	entry, err := other.anchorsEntry(other.recovery.status, 0)
	if err != nil {
		return err
	}
	if entry.State != "running" || entry.Resource != "running" || entry.Anchor != "present" || entry.Controller != "present" || entry.Cleanup != "pending" || entry.WorkloadExit != nil || entry.StopCandidate {
		return errors.New("B lost its independent running session or anchor")
	}
	if err := other.anchorsStatusUnchanged(); err != nil {
		return err
	}
	if !other.recoveryNativePresent(true) {
		return errors.New("B's exact worker no longer runs")
	}
	entry, err = w.anchorsEntry(w.recovery.after, 0)
	if err != nil {
		return err
	}
	if entry.State != "stopped" || entry.Anchor != "absent" {
		return errors.New("A status lost its own completion")
	}
	return nil
}

func (w *world) anchorsCrossRefused() error {
	if err := anchorsProblem(w.anchors.cross, "recovery.not-owned", 1); err != nil {
		return err
	}
	other, a := w.recovery.second, w.offline.api
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, q := range a.requests {
		if strings.Contains(q.Path, other.recovery.worker) {
			return errors.New("A contacted B's worker")
		}
		for name := range other.anchor.volumes {
			if strings.Contains(q.Path, name) {
				return errors.New("A contacted B's anchor")
			}
		}
	}
	return nil
}
