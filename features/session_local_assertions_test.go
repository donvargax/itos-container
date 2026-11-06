package features

import (
	"fmt"
	"reflect"
	"strings"
)

func (w *world) sessionOutcome(want string) error {
	s := w.session
	if s.fixtureError != nil {
		return s.fixtureError
	}
	got := s.create
	if s.mode == "recovery" {
		got = s.cleanup
	}
	if got.Outcome != want || got.Handoff || got.ID != "" || got.Workload != nil {
		return fmt.Errorf("expected %s without handoff, got %+v", want, got)
	}
	return nil
}
func (w *world) sessionNoMutations() error {
	for _, r := range w.session.api.requests {
		if r.Method == "POST" || r.Method == "DELETE" {
			return fmt.Errorf("unexpected engine mutation %s %s", r.Method, r.Path)
		}
	}
	return nil
}
func (w *world) sessionIntentRetained() error {
	s := w.session
	if err := w.sessionExactRecord(s.owned, "intent", ""); err != nil {
		return err
	}
	if s.owned != s.final {
		return fmt.Errorf("original intent changed")
	}
	for _, event := range append(append([]sessionEvent{}, s.create.Events...), s.cleanup.Events...) {
		if event.Stage == "owned" || event.Stage == "absent" {
			return fmt.Errorf("unexpected progress update")
		}
	}
	return nil
}
func (w *world) sessionInspected() error {
	s := w.session
	id := s.seedID
	if id == "" && len(s.api.created) == 1 {
		id = s.api.created[0]
	}
	for _, r := range s.api.requests {
		if r.Method == "GET" && r.Path == "/containers/"+id+"/json" {
			return nil
		}
	}
	return fmt.Errorf("recorded or acknowledged full ID was never inspected")
}
func (w *world) sessionOnlyInspections() error {
	s := w.session
	start := s.cleanupStart
	if s.mode != "recovery" {
		for i, r := range s.api.requests {
			if r.Method == "POST" {
				start = i + 1
			}
		}
	}
	for _, r := range s.api.requests[start:] {
		if r.Method != "GET" && r.Method != "HEAD" {
			return fmt.Errorf("post-create request was a mutation")
		}
	}
	return nil
}
func (w *world) sessionCleanupComplete() error {
	s := w.session
	if s.fixtureError != nil {
		return s.fixtureError
	}
	if s.cleanup.Outcome != "cleanup-complete" || s.cleanup.Handoff || s.cleanup.Workload != nil {
		return fmt.Errorf("expected cleanup-complete, got %+v", s.cleanup)
	}
	id := s.seedID
	if id == "" && len(s.api.created) == 1 {
		id = s.api.created[0]
	}
	if err := w.sessionExactRecord(s.final, "absent", id); err != nil {
		return err
	}
	found := false
	for _, r := range s.api.requests[s.cleanupStart:] {
		if r.Method == "GET" && r.Path == "/containers/"+id+"/json" && r.Status == 404 {
			found = true
		}
	}
	if !found || !s.api.absent[id] {
		return fmt.Errorf("no independently confirmed full-ID absence")
	}
	if s.firstRecord == s.final && s.firstRecord.Bytes != "" {
		if s.firstResult.Outcome == "completion-write-failed" {
			file, parent := false, false
			for _, e := range s.cleanup.Events {
				if e.Stage != "absent" || e.Failed {
					continue
				}
				record, err := sessionDecodeRecord(e.Bytes)
				if err != nil || !reflect.DeepEqual(record, w.sessionExpectedRecord("absent", id)) {
					return fmt.Errorf("retry synced different ownership bytes")
				}
				file = file || e.Operation == "file fsync"
				parent = parent || e.Operation == "parent fsync"
			}
			if !file || !parent {
				return fmt.Errorf("visible tombstone was not made durable again")
			}
		}
		return nil
	}
	return w.sessionDurability(s.cleanup.Events, "absent", id)
}
func (w *world) sessionRemovalVerified() error {
	s := w.session
	id := s.seedID
	if id == "" && len(s.api.created) == 1 {
		id = s.api.created[0]
	}
	inspected := false
	removed := false
	for _, r := range s.api.requests[s.cleanupStart:] {
		if r.Method == "GET" && r.Path == "/containers/"+id+"/json" && r.Status == 200 {
			inspected = true
		}
		if r.Method == "DELETE" {
			if !inspected || r.Path != "/containers/"+id {
				return fmt.Errorf("removed without fresh exact-ID inspection")
			}
			removed = true
		}
	}
	if !removed {
		return fmt.Errorf("no verified full-ID removal")
	}
	return nil
}
func (w *world) sessionSafeReference() error {
	s := w.session
	r := s.create
	if s.mode == "recovery" {
		r = s.cleanup
	}
	if r.Reference != s.reference || r.Handoff || r.ID != "" {
		return fmt.Errorf("missing surviving relative intent reference")
	}
	return nil
}
func (w *world) sessionProgress(stage string) error {
	record, err := sessionDecodeRecord(w.session.final.Bytes)
	if err != nil || record["stage"] != stage || w.session.cleanup.Workload != nil {
		return fmt.Errorf("expected complete %s progress record", stage)
	}
	return nil
}
func (w *world) sessionFirstUncertain() error {
	s := w.session
	if err := w.sessionOutcome("create-uncertain"); err != nil {
		return err
	}
	if s.firstResult.Outcome != "create-uncertain" || s.firstResult.Reference != s.reference {
		return fmt.Errorf("first cleanup lost uncertainty or reference")
	}
	return w.sessionExactRecord(s.firstRecord, "intent", "")
}
func (w *world) sessionFirstNoRemoval() error {
	s := w.session
	if len(s.firstRequests) == 0 {
		return fmt.Errorf("first cleanup attempted no fresh engine inspection")
	}
	for _, r := range s.firstRequests {
		if r.Method == "DELETE" || r.Method == "POST" {
			return fmt.Errorf("uncertain first cleanup mutated engine")
		}
	}
	for _, e := range s.firstResult.Events {
		if e.Stage == "absent" {
			return fmt.Errorf("uncertain first cleanup recorded absence")
		}
	}
	return nil
}
func (w *world) sessionCompound() error {
	s := w.session
	if err := w.sessionObservedCreate(); err != nil {
		return err
	}
	if err := w.sessionOutcome("ownership-write-failed"); err != nil {
		return err
	}
	if s.create.Cleanup != "cleanup-incomplete" || s.firstResult.Outcome != "cleanup-incomplete" {
		return fmt.Errorf("write error lost separate incomplete cleanup")
	}
	if err := w.sessionExactRecord(s.firstRecord, "intent", ""); err != nil {
		return err
	}
	if len(s.api.created) != 1 || s.firstResources[s.api.created[0]] == nil {
		return fmt.Errorf("created worker not retained during compound failure")
	}
	return nil
}
func (w *world) sessionCompletionFailed() error {
	s := w.session
	if s.firstResult.Outcome != "completion-write-failed" || s.firstResult.Reference != s.reference {
		return fmt.Errorf("expected completion-write-failed, got %+v", s.firstResult)
	}
	observedAbsent := false
	for _, r := range s.firstRequests {
		if r.Method == "GET" && r.Path == "/containers/"+s.seedID+"/json" && r.Status == 404 {
			observedAbsent = true
		}
	}
	if !observedAbsent {
		return fmt.Errorf("completion write failed without prior concrete-ID absence")
	}
	if s.firstResources[s.seedID] != nil {
		return fmt.Errorf("completion failure did not follow observed absence")
	}
	r, err := sessionDecodeRecord(s.firstRecord.Bytes)
	if err != nil || (r["stage"] != "owned" && r["stage"] != "absent") || r["container_id"] != s.seedID {
		return fmt.Errorf("failed persistence lost complete recovery record")
	}
	return nil
}
func (w *world) sessionRepeated() error {
	s := w.session
	if s.firstResult.Outcome != "cleanup-complete" {
		return fmt.Errorf("first cleanup did not complete")
	}
	if err := w.sessionCleanupComplete(); err != nil {
		return err
	}
	if s.firstRecord != s.final {
		return fmt.Errorf("repeat rewrote durable tombstone")
	}
	return w.sessionNoMutations()
}
func (w *world) sessionTwo() error {
	s := w.session
	if s.second == nil {
		return fmt.Errorf("second operation missing")
	}
	for _, cycle := range []*sessionWorld{s, s.second} {
		copyWorld := *w
		copyWorld.session = cycle
		if err := copyWorld.sessionOwned(); err != nil {
			return err
		}
		if err := copyWorld.sessionDurableAbsent(); err != nil {
			return err
		}
		if err := copyWorld.sessionUnrelated(); err != nil {
			return err
		}
	}

	r1, e1 := sessionDecodeRecord(s.final.Bytes)
	r2, e2 := sessionDecodeRecord(s.second.final.Bytes)
	if e1 != nil || e2 != nil || r1["stage"] != "absent" || r2["stage"] != "absent" || r1["container_id"] == r2["container_id"] || r1["name"] == r2["name"] || r1["snapshot"] != r2["snapshot"] || r1["fingerprint"] != r2["fingerprint"] {
		return fmt.Errorf("independent operations did not retain distinct IDs and common frozen binding")
	}
	if s.create.Outcome != "owned-unstarted" || s.second.create.Outcome != "owned-unstarted" || s.cleanup.Outcome != "cleanup-complete" || s.second.cleanup.Outcome != "cleanup-complete" {
		return fmt.Errorf("two cycles did not complete")
	}
	return nil
}
func (w *world) sessionNoNameLookup() error {
	s := w.session
	for _, r := range s.api.requests {
		if strings.HasPrefix(r.Path, "/containers/") && r.Method == "GET" && r.Path != "/containers/"+s.seedID+"/json" {
			return fmt.Errorf("owned ID inspection fell back to name")
		}
	}
	return w.sessionInspected()
}
func (w *world) sessionSnapshotBindings() error {
	s := w.session
	for _, r := range []sessionRecord{s.owned, s.final} {
		obj, err := sessionDecodeRecord(r.Bytes)
		if err != nil {
			return err
		}
		expected := w.sessionExpectedRecord(obj["stage"].(string), obj["container_id"].(string))
		if !reflect.DeepEqual(obj, expected) {
			return fmt.Errorf("record lost original snapshot binding")
		}
	}
	return nil
}
func (w *world) sessionVisibleIntent(visible string) error {
	s := w.session
	if s.owned.Bytes != "" {
		if visible == "no final record" {
			return fmt.Errorf("failed publication exposed a final record")
		}
		if err := w.sessionExactRecord(s.owned, "intent", ""); err != nil {
			return err
		}
	}
	return w.sessionNoMutations()
}
