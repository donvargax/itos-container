package features

import "fmt"

func (w *world) sessionReconciledBeforeRemoval() error {
	s := w.session
	for _, request := range s.firstRequests {
		if request.Method != "DELETE" {
			continue
		}
		if request.Path != "/containers/"+s.seedID {
			return fmt.Errorf("removed a different resource")
		}
		if err := w.sessionExactRecord(request.Record, "owned", s.seedID); err != nil {
			return fmt.Errorf("removal preceded reconciled ID persistence: %w", err)
		}
		return w.sessionDurability(request.Events, "owned", s.seedID)
	}
	return fmt.Errorf("no reconciled removal observed")
}

func (w *world) sessionReconciledWriteFailed() error {
	s := w.session
	if s.firstResult.Outcome != "ownership-write-failed" || s.firstResult.Reference != s.reference || s.firstResult.Handoff || s.firstResult.ID != "" {
		return fmt.Errorf("expected ownership-write-failed without handoff, got %s", s.firstResult.Outcome)
	}
	if err := w.sessionFirstNoRemoval(); err != nil {
		return err
	}
	id := s.seedID
	if id == "" && len(s.api.created) == 1 {
		id = s.api.created[0]
	}
	if err := w.sessionIdentityProblem(s.firstResources[id], id); err != nil {
		return fmt.Errorf("failed ownership save did not preserve worker: %w", err)
	}
	record, err := sessionDecodeRecord(s.firstRecord.Bytes)
	if err != nil {
		return err
	}
	stage, storedID := "intent", ""
	if s.condition == "parent fsync" {
		stage, storedID = "owned", id
	}
	if record["stage"] != stage || record["container_id"] != storedID {
		return fmt.Errorf("failed ownership save lost its complete recovery record")
	}
	return nil
}
