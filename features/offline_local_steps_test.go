package features

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/cucumber/godog"
)

func (w *world) offlineLocalSteps(sc *godog.ScenarioContext) {
	sc.Step(`^an offline run contract fixture whose pre-start inspection has "([^"]*)"$`, func(defect string) error { return w.offlineLocalFixture(defect, false) })
	sc.Step(`^an offline run contract fixture with persistent owned-record fsync failure$`, func() error { return w.offlineLocalFixture("", true) })
	sc.Step(`^the offline contract attempts launch through both fake and disk SDK adapters$`, func() error { return w.offlineLocalRun(false) })
	sc.Step(`^the offline contract attempts launch and later retries cleanup after clearing the fault$`, func() error { return w.offlineLocalRun(true) })
	sc.Step(`^each offline run reports "([^"]*)" with exit (\d+) before any start$`, func(rule string, code int) error {
		return w.sessionEach(func(w *world) error {
			s := w.session
			if err := w.offlineLocalObserved(); err != nil {
				return err
			}
			if s.create.Code != code || !strings.Contains(s.create.Diagnostic, rule+" ") || s.create.Outcome != strings.TrimPrefix(rule, "run.") {
				return fmt.Errorf("offline refusal got exit %d outcome %q, want exit %d rule %s", s.create.Code, s.create.Outcome, code, rule)
			}
			return w.offlineNoLaunch()
		})
	})
	sc.Step(`^each offline run attempted one create but no entrypoint, exec or workload instruction$`, func() error {
		return w.sessionEach(func(w *world) error {
			if len(w.session.api.created) != 1 {
				return fmt.Errorf("expected exactly one independently observed create")
			}
			return w.offlineNoLaunch()
		})
	})
	sc.Step(`^each offline run cleans only the independently revalidated owned unstarted worker$`, func() error { return w.sessionEach((*world).offlineLocalCleanup) })
	sc.Step(`^each first offline run reports ownership publication failure with no start or usable launch handoff$`, func() error {
		return w.sessionEach(func(w *world) error {
			if err := w.offlineLocalObserved(); err != nil {
				return err
			}
			r := w.session.create
			if r.Code != 75 || r.Outcome != "ownership-write-failed" || r.Cleanup != "ownership-write-failed" || r.ID != "" || r.Handoff || r.Workload != nil || !strings.Contains(r.Diagnostic, "run.ownership-write-failed ") {
				return fmt.Errorf("ownership publication failure allowed a launch handoff or lost its primary result: %+v", r)
			}
			return w.offlineNoLaunch()
		})
	})
	sc.Step(`^each first offline run retains the complete protected intent and independently observed unstarted worker$`, func() error {
		return w.sessionEach(func(w *world) error {
			s := w.session
			if len(s.api.created) != 1 || s.create.Reference != s.reference {
				return fmt.Errorf("first run lost its created worker or intent reference")
			}
			if err := w.sessionExactRecord(s.firstRecord, "intent", ""); err != nil {
				return err
			}
			if err := w.sessionDurability(s.create.Events, "intent", ""); err != nil {
				return err
			}
			if len(s.firstResources) != 1 || object(s.firstResources[s.api.created[0]]["State"])["Status"] != "created" {
				return fmt.Errorf("first run did not retain its unstarted worker")
			}
			for _, r := range s.firstRequests {
				if r.Method == "DELETE" {
					return fmt.Errorf("removal preceded durable ownership")
				}
			}
			failures := 0
			for _, e := range s.create.Events {
				if e.Stage == "owned" && e.Operation == "file fsync" && e.Failed {
					failures++
				}
			}
			if failures < 2 {
				return fmt.Errorf("persistent fault did not reach both launch and cleanup publication")
			}
			return nil
		})
	})
	sc.Step(`^each later cleanup persists the reconciled ID before removal and confirms absence without starting work$`, func() error { return w.sessionEach((*world).offlineLocalCleanup) })
	sc.Step(`^the original run failure is not replaced by later cleanup success$`, func() error {
		return w.sessionEach(func(w *world) error {
			if w.session.create.Outcome != "ownership-write-failed" || w.session.create.Code != 75 || w.session.cleanup.Outcome != "cleanup-complete" {
				return fmt.Errorf("cleanup erased the original failure or did not complete")
			}
			return nil
		})
	})
}

func (w *world) offlineLocalFixture(defect string, fault bool) error {
	if err := w.sessionLocalFixture("twice", ""); err != nil {
		return err
	}
	return w.sessionEach(func(w *world) error {
		s := w.session
		s.offline, s.boundary = true, defect
		if fault {
			s.fault = "owned-record file fsync"
		}
		dir := filepath.Join(w.dir, "slice-10-control")
		if err := os.Mkdir(dir, 0700); err != nil {
			return err
		}
		control := &world{root: w.root, bin: w.bin, dir: dir}
		s.offlineControl = control
		if err := control.sessionFixture(false, "0022"); err != nil {
			return err
		}
		control.session.backend, control.session.offline = s.backend, true
		return nil
	})
}
func (w *world) offlineLocalRun(retry bool) error {
	return w.sessionEach(func(w *world) error {
		s := w.session
		s.fixtureError = w.offlineLocalAction(retry)
		control := s.offlineControl
		control.session.fixtureError = control.offlineLocalAction(false)
		return nil // Every early refusal is judged in Then, never in setup.
	})
}
func (w *world) offlineLocalAction(retry bool) error {
	s := w.session
	var err error
	s.create, err = w.sessionProbe("run", "", w.sessionCaptureBefore)
	if err != nil {
		return err
	}
	if err = w.sessionCaptureAfter(); err != nil {
		return err
	}
	s.cleanup = sessionResult{Outcome: s.create.Cleanup, Reference: s.create.Reference}
	if !retry {
		return nil
	}
	s.firstRecord = s.final
	s.firstRequests = append([]sessionRequest{}, s.api.requests...)
	s.firstResources = map[string]map[string]any{}
	for id, c := range s.api.local {
		s.firstResources[id] = approvalClone(c)
	}
	s.fault = ""
	s.cleanupStart = len(s.api.requests)
	s.cleanup, err = w.sessionProbe("run-cleanup", s.reference, nil)
	if err != nil {
		return err
	}
	return w.sessionCaptureAfter()
}
func (w *world) offlineLocalObserved() error {
	s := w.session
	if s.fixtureError != nil {
		return fmt.Errorf("offline fixture: %w; %s", s.fixtureError, s.transcript)
	}
	control := s.offlineControl
	if control == nil {
		return fmt.Errorf("missing positive control")
	}
	if control.session.fixtureError != nil {
		return fmt.Errorf("positive control: %w; %s", control.session.fixtureError, control.session.transcript)
	}
	r := control.session.create
	if r.Code != 0 || r.Outcome != "" || r.Workload == nil || *r.Workload != 0 || r.Cleanup != "cleanup-complete" {
		return fmt.Errorf("same-backend positive control did not execute and clean: exit %d outcome %q cleanup %q diagnostic %q", r.Code, r.Outcome, r.Cleanup, r.Diagnostic)
	}
	starts := 0
	if s.backend == "memory" {
		for _, e := range r.RunEvents {
			if e == "start" {
				starts++
			}
		}
	} else {
		for _, q := range control.session.api.requests {
			if q.Method == "POST" && strings.HasSuffix(q.Path, "/start") {
				starts++
			}
		}
	}
	if starts != 1 {
		return fmt.Errorf("positive control must start exactly one worker")
	}
	return control.offlineLocalCleanup()
}
func (w *world) offlineNoLaunch() error {
	s := w.session
	if s.create.Workload != nil || s.create.ID != "" {
		return fmt.Errorf("refused worker launched")
	}
	for _, e := range s.create.RunEvents {
		if e == "start" || e == "attach" {
			return fmt.Errorf("refused worker reached %s", e)
		}
	}
	for _, q := range s.api.requests {
		if q.Method == "POST" && q.Path != "/containers/create" {
			return fmt.Errorf("refused worker attempted %s", q.Path)
		}
	}
	return nil
}
func (w *world) offlineLocalCleanup() error {
	s := w.session
	if s.fixtureError != nil {
		return s.fixtureError
	}
	if s.cleanup.Outcome != "cleanup-complete" || len(s.api.created) != 1 {
		return fmt.Errorf("owned cleanup did not complete after exactly one create")
	}
	id := s.api.created[0]
	if !s.api.absent[id] || s.api.local[id] != nil {
		return fmt.Errorf("full ID absence not independently confirmed")
	}
	inspected, removed, absent := false, false, false
	for _, q := range s.api.requests {
		if q.Method == "GET" && q.Path == "/containers/"+id+"/json" {
			if q.Status == 200 {
				inspected = true
			}
			if removed && q.Status == 404 {
				absent = true
			}
		}
		if q.Method == "DELETE" {
			if removed || !inspected || q.Path != "/containers/"+id {
				return fmt.Errorf("cleanup removed an unverified or different resource")
			}
			if err := w.sessionExactRecord(q.Record, "owned", id); err != nil {
				return err
			}
			if err := w.offlineOwnedDurability(q.Events, id); err != nil {
				return err
			}
			removed = true
		}
	}
	if !removed || !absent {
		return fmt.Errorf("cleanup missing removal or later full ID absence")
	}
	if err := w.sessionExactRecord(s.final, "absent", id); err != nil {
		return err
	}
	events, err := sessionReadEvents(s.journal)
	if err != nil {
		return err
	}
	if err = w.sessionDurability(events, "absent", id); err != nil {
		return err
	}
	return w.sessionUnrelated()
}

// Foreground cleanup re-syncs an already published owned record. Check the
// complete publication and any later re-sync in journal order; choosing the last
// fsync and the earlier rename independently would misorder this valid sequence.
func (w *world) offlineOwnedDurability(events []sessionEvent, id string) error {
	written, synced, published, durable := false, false, false, false
	for _, e := range events {
		if e.Stage != "owned" {
			continue
		}
		record, err := sessionDecodeRecord(e.Bytes)
		if err != nil || !reflect.DeepEqual(record, w.sessionExpectedRecord("owned", id)) {
			return fmt.Errorf("owned durability event has different bindings")
		}
		if e.Failed {
			durable, synced = false, false
			continue
		}
		switch e.Operation {
		case "write":
			written, synced, published, durable = true, false, false, false
		case "file fsync":
			synced, durable = true, false
		case "atomic publication":
			if !written || !synced {
				return fmt.Errorf("owned publication preceded write or fsync")
			}
			published = true
		case "parent fsync":
			durable = published && synced
		}
	}
	if !durable {
		return fmt.Errorf("owned removal preceded completed publication or re-sync")
	}
	return nil
}
