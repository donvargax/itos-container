package features

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/cucumber/godog"
)

func (w *world) sessionSteps(sc *godog.ScenarioContext) {
	w.sessionLocalSteps(sc)
	sc.Step(`^a CI-only session fixture using the independently inspected T-20 cached image$`, func() error { return w.sessionFixture(true, "0022") })
	sc.Step(`^the CI session fixture injects "([^"]*)" after native Docker accepts create$`, func(fault string) error {
		if fault != "lost create response" && fault != "owned-record file fsync" {
			return fmt.Errorf("unknown session fault")
		}
		w.session.fault = fault
		return nil
	})
	sc.Step(`^the session contract creates an unstarted worker and cleans it using native Docker$`, func() error { return w.sessionRun(false) })
	sc.Step(`^creation fails and a fresh session controller cleans using only the surviving protected intent$`, func() error { return w.sessionRun(true) })
	sc.Step(`^the CI observer sees one created worker whose full ID matches the durable ownership record$`, w.sessionOwned)
	sc.Step(`^the CI request recorder observed the exact durable intent before forwarding create$`, w.sessionIntentBeforeCreate)
	sc.Step(`^native Docker reports that worker never started before product removal$`, w.sessionNeverStarted)
	sc.Step(`^native Docker independently confirms that exact ID absent after product cleanup$`, w.sessionAbsent)
	sc.Step(`^the protected record durably records that ID absent without a workload result$`, w.sessionDurableAbsent)
	sc.Step(`^the CI audit preserves unrelated engine resources, checkout edits, approvals and snapshots$`, w.sessionUnrelated)
	sc.Step(`^no session operation starts work, acquires images or leaks sensitive fixture values$`, func() error { return w.sessionEach((*world).sessionNoForbidden) })
	sc.Step(`^the CI creation result is "([^"]*)" after exactly one independently observed unstarted create$`, func(outcome string) error {
		if err := w.sessionObservedCreate(); err != nil {
			return err
		}
		if w.session.create.Outcome != outcome {
			return fmt.Errorf("expected creation outcome %s, got %s", outcome, w.session.create.Outcome)
		}
		return w.sessionIntentBeforeCreate()
	})
	sc.Step(`^no successful ownership handoff or worker start was returned or attempted$`, func() error {
		return w.sessionEach(func(w *world) error {
			if w.session.create.Handoff || w.session.create.ID != "" || w.session.create.Workload != nil {
				return fmt.Errorf("failed creation supplied a usable handoff or workload result")
			}
			return w.sessionNoForbidden()
		})
	})
	sc.Step(`^the fresh controller revalidates the intent, daemon, random name and concrete ID before product removal$`, w.sessionRecoveryInspection)
	sc.Step(`^no replacement create, worker start or foreign removal was attempted$`, func() error {
		return w.sessionEach(func(w *world) error {
			if w.session.mode != "recovery" {
				if err := w.sessionObservedCreate(); err != nil {
					return err
				}
			}
			return w.sessionNoForbidden()
		})
	})
}

func (w *world) sessionUnrelated() error {
	s := w.session
	if s.before == nil || s.after == nil || !reflect.DeepEqual(s.before, s.after) {
		return fmt.Errorf("session changed protected checkout or control state")
	}
	if w.doctor.real != nil && (s.resourcesBefore == nil || s.resourcesAfter == nil || !reflect.DeepEqual(s.resourcesBefore, s.resourcesAfter)) {
		return fmt.Errorf("session changed unrelated engine resource inventory")
	}
	return nil
}

func (w *world) sessionNoForbidden() error {
	s := w.session
	if s.fixtureError != nil {
		return s.fixtureError
	}
	if strings.Contains(s.transcript, doctorCanary) || strings.Contains(s.owned.Bytes+s.final.Bytes, doctorCanary) || strings.Contains(string(approvalJSON(s.create))+string(approvalJSON(s.cleanup)), doctorCanary) {
		return fmt.Errorf("session diagnostics or ownership leaked synthetic fixture text")
	}
	for _, request := range s.api.requests {
		if doctorAllowedRequest(doctorRequest{Method: request.Method, Path: request.Path, Query: request.Query}, w.doctor.image, true) {
			continue
		}
		if request.Method == "POST" && request.Path == "/containers/create" {
			if err := w.sessionRequestProblem(request.Body); err != nil {
				return err
			}
			continue
		}
		id := strings.TrimPrefix(request.Path, "/containers/")
		if request.Method == "GET" && strings.HasSuffix(id, "/json") && request.Query == "" {
			id = strings.TrimSuffix(id, "/json")
			if id == "itos-container-"+s.token || (containsSessionID(s.api.created, id) || (w.doctor.real == nil && id == s.seedID)) {
				continue
			}
		}
		if request.Method == "DELETE" && (containsSessionID(s.api.created, id) || (w.doctor.real == nil && id == s.seedID)) && request.Status == 204 {
			continue
		}
		return fmt.Errorf("session attempted an unexpected engine request")
	}
	return nil
}
