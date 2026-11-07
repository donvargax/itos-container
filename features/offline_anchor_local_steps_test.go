package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

func (w *world) anchorLocalSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a mount-anchor contract fixture with approved settings and a writable checkout$`, func() error { return w.anchorLocalFixture("") })
	sc.Step(`^a mount-anchor contract fixture with (?:persistent publication|source|post-start detector|worker startup) fault "([^"]*)"$`, w.anchorLocalFixture)
	sc.Step(`^a mount-anchor contract fixture with a zero-exit workload and cleanup fault "([^"]*)"$`, w.anchorLocalFixture)
	sc.Step(`^the registered run completes through both fake and disk SDK mount-anchor adapters$`, w.offlineLaunchAction)
	sc.Step(`^the registered run and its fault-free control execute through both mount-anchor backends$`, w.offlineLaunchAction)
	sc.Step(`^each run durably records anchor intent before allocation and verified concrete anchor ownership before worker create$`, func() error { return w.anchorLocalEach(anchorLocalOrder) })
	sc.Step(`^each worker mounts its owned local volume at /workspace with copying disabled and the exact descriptor-backed driver options$`, func() error { return w.anchorLocalEach(anchorLocalMount) })
	sc.Step(`^each worker consumes only that anchor backed by the validated checkout inode with the unchanged offline boundary$`, func() error { return w.anchorLocalEach(anchorLocalBoundary) })
	sc.Step(`^each controller holds the validated descriptor through the start response and immediately checks the mounted device and inode using the freshly inspected worker PID$`, func() error { return w.anchorLocalEach(anchorLocalDetector) })
	sc.Step(`^each owned volume is retained until confirmed worker absence and released before durable session completion$`, func() error { return w.anchorLocalEach(anchorLocalRelease) })
	sc.Step(`^each run returns zero with the checkout edit preserved and no owned ephemeral resource remaining$`, func() error { return w.anchorLocalEach(anchorLocalSuccess) })
	sc.Step(`^mount-anchor operations leave approval snapshots outside targets and unrelated resources unchanged$`, func() error { return w.anchorLocalEach((*world).offlineEffects) })
	sc.Step(`^each faulted run reports "([^"]*)" with exit (\d+) and no worker create or start$`, func(rule string, code int) error {
		return w.anchorLocalEach(func(w *world) error {
			if err := anchorLocalResult(w, rule, code); err != nil {
				return err
			}
			return anchorLocalNo(w, "create", "start")
		})
	})
	sc.Step(`^anchor allocation occurs only after durable intent and no failed publication yields a usable source$`, func() error {
		return w.anchorLocalEach(func(w *world) error {
			if err := anchorLocalNo(w, "create", "start"); err != nil {
				return err
			}
			if anchorEventIndex(w, "anchor allocate") >= 0 {
				return anchorLocalIntent(w)
			}
			return nil
		})
	})
	sc.Step(`^every possibly allocated anchor is accounted for by confirmed release or retained protected ownership with incomplete cleanup$`, func() error { return w.anchorLocalEach(anchorLocalAccounted) })
	sc.Step(`^each fault-free control launches through its owned anchor and confirms complete cleanup$`, func() error {
		return w.anchorLocalEach(func(w *world) error {
			c := w.session.offlineControl
			for _, check := range []func(*world) error{anchorLocalOrder, anchorLocalMount, anchorLocalDetector, anchorLocalRelease, anchorLocalSuccess} {
				if err := check(c); err != nil {
					return fmt.Errorf("healthy control: %w", err)
				}
			}
			return nil
		})
	})
	sc.Step(`^no operation falls back to the original pathname or allocates a replacement after uncertainty$`, func() error { return w.anchorLocalEach(anchorLocalNoRetry) })
	sc.Step(`^source uncertainty retains the original anchor intent and source mismatch authorizes no foreign-resource mutation$`, func() error {
		return w.anchorLocalEach(func(w *world) error {
			if strings.Contains(w.session.anchorFault, "allocation response") {
				return anchorLocalRetained(w)
			}
			if strings.Contains(w.session.anchorFault, "replaced after") {
				return anchorLocalNo(w, "anchor remove")
			}
			return nil
		})
	})
	sc.Step(`^each faulted run attempts the post-start mounted-identity check and immediately kills and removes only its revalidated owned worker$`, func() error {
		return w.anchorLocalEach(anchorLocalImmediate)
	})
	sc.Step(`^each detector refusal reports "([^"]*)" with classification (\d+) and a nonzero result while preserving any observed nonzero workload exit$`, func(rule string, code int) error {
		return w.anchorLocalEach(func(w *world) error {
			r := w.session.create
			if !strings.Contains(r.Diagnostic, rule+" ") || r.Code == 0 || (r.Workload != nil && *r.Workload != 0 && r.Code != *r.Workload) || (r.Workload == nil && r.Code != code) {
				return fmt.Errorf("missing detector refusal: %+v", r)
			}
			return nil
		})
	})
	sc.Step(`^no detector refusal claims the worker never started or falls back to a pathname or another create or start$`, func() error {
		return w.anchorLocalEach(func(w *world) error {
			if !w.session.create.Started {
				return fmt.Errorf("detector refusal lost the successful start")
			}
			return anchorLocalNoRetry(w)
		})
	})
	sc.Step(`^each faulted run accounts for its durably owned anchor as "([^"]*)"$`, func(disposition string) error {
		return w.anchorLocalEach(func(w *world) error {
			if err := anchorLocalOrder(w); err != nil {
				return err
			}
			if disposition == "released after full-ID absence" {
				return anchorLocalRelease(w)
			}
			if err := anchorLocalNo(w, "anchor remove"); err != nil {
				return err
			}
			return anchorLocalRetained(w)
		})
	})
	sc.Step(`^each faulted run reports "([^"]*)" with exit (\d+) and no start or invented workload result$`, func(rule string, code int) error {
		return w.anchorLocalEach(func(w *world) error {
			if err := anchorLocalResult(w, rule, code); err != nil {
				return err
			}
			if w.session.create.Workload != nil {
				return fmt.Errorf("startup refusal invented a workload exit")
			}
			return anchorLocalNo(w, "start")
		})
	})
	sc.Step(`^no worker create is retried and a name-only absence never permits anchor release$`, func() error { return w.anchorLocalEach(anchorLocalNoRetry) })
	sc.Step(`^each faulted run retains protected anchor ownership and reports its actual worker and anchor observations as incomplete cleanup$`, func() error { return w.anchorLocalEach(anchorLocalRetained) })
	sc.Step(`^each faulted run returns (\d+) with separate "([^"]*)" guidance and its registered session ID$`, func(code int, rule string) error {
		return w.anchorLocalEach(func(w *world) error {
			if err := anchorLocalResult(w, rule, code); err != nil {
				return err
			}
			r := w.session.create
			if !offlineHex(r.SessionID) || !strings.Contains(r.Diagnostic, "itos container stop "+r.SessionID) || r.Workload == nil || *r.Workload != 0 {
				return fmt.Errorf("missing actual exit and registered recovery guidance")
			}
			return nil
		})
	})
	sc.Step(`^no anchor release precedes confirmed worker absence or mutates a foreign source$`, func() error { return w.anchorLocalEach(anchorLocalSafeRelease) })
	sc.Step(`^no session completion is reported before anchor absence and durable completion publication$`, func() error { return w.anchorLocalEach(anchorLocalRetained) })
}

func (w *world) anchorLocalFixture(fault string) error {
	if err := w.offlineLaunchFixture(); err != nil {
		return err
	}
	return w.sessionEach(func(w *world) error {
		for _, c := range []*world{w, w.session.offlineControl} {
			c.session.anchorFixture, c.session.launch.registered = true, true
		}
		w.session.anchorFault = fault
		if fault == "owned unstarted worker fails boundary inspection" {
			w.session.boundary = "network mode bridge"
		}
		return nil
	})
}

func (w *world) anchorLocalEach(check func(*world) error) error {
	var failures error
	err := w.sessionEach(func(w *world) error {
		if w.session.fixtureError != nil {
			failures = errors.Join(failures, fmt.Errorf("%s fixture: %w %s", w.session.backend, w.session.fixtureError, w.session.transcript))
			return nil
		}
		if err := check(w); err != nil {
			failures = errors.Join(failures, fmt.Errorf("%s: %w", w.session.backend, err))
		}
		return nil
	})
	return errors.Join(err, failures)
}

func anchorEventIndex(w *world, op string) int {
	for i, e := range w.session.create.Trace {
		if e.Operation == op {
			return i
		}
	}
	return -1
}
func anchorLocalNo(w *world, ops ...string) error {
	for _, op := range ops {
		if anchorEventIndex(w, op) >= 0 {
			return fmt.Errorf("forbidden operation %s", op)
		}
	}
	return nil
}
func anchorLocalResult(w *world, rule string, code int) error {
	r := w.session.create
	if r.Code != code || !strings.Contains(r.Diagnostic, rule+" ") {
		return fmt.Errorf("want %s exit %d, got %d %q", rule, code, r.Code, r.Diagnostic)
	}
	return nil
}
func anchorRecord(w *world) map[string]any {
	var r map[string]any
	_ = json.Unmarshal([]byte(w.session.final.Bytes), &r)
	return object(r["anchor"])
}
