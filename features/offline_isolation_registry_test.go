package features

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

func (s *isolationWorld) registrationRoot() string {
	return filepath.Join(s.w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", s.w.recoveryIndexKey())
}

func (s *isolationWorld) registrationIntent(r *isolationRun) string {
	token := strings.TrimPrefix(fmt.Sprint(r.expected["name"]), "itos-container-")
	return "intents/" + s.w.approval.project + "/" + token + ".json"
}

// Observe each run independently, before another invocation can change its
// registration. Match the original intent, never the current fixture token.
func (s *isolationWorld) observeRegistration(r *isolationRun) error {
	entries, err := os.ReadDir(s.registrationRoot())
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !offlineHex(entry.Name()) {
			return errors.New("native run has an invalid public registration entry")
		}
		record, err := sessionReadRecord(filepath.Join(s.registrationRoot(), entry.Name(), "record.json"))
		if err != nil {
			return err
		}
		data, err := sessionDecodeRecord(record.Bytes)
		if err != nil {
			return err
		}
		if data["intent"] == s.registrationIntent(r) {
			if r.sessionID != "" {
				return errors.New("multiple registrations claim the same native run")
			}
			r.sessionID, r.registration = entry.Name(), record
		}
	}
	if r.sessionID == "" {
		return errors.New("native run has no registration bound to its original intent")
	}
	return nil
}

func (s *isolationWorld) registryPaths() (map[string]bool, error) {
	entries, err := os.ReadDir(s.registrationRoot())
	if err != nil {
		return nil, err
	}
	if len(s.runs) == 0 || len(entries) != len(s.runs) {
		return nil, errors.New("native runs did not retain exactly their public registrations")
	}
	allowed, seen := map[string]bool{}, map[string]bool{}
	for _, r := range s.runs {
		token := strings.TrimPrefix(fmt.Sprint(r.expected["name"]), "itos-container-")
		if !offlineHex(r.sessionID) || r.sessionID == token || seen[r.sessionID] {
			return nil, errors.New("native runs lost independent public session identities")
		}
		seen[r.sessionID] = true
		paths, err := s.w.offlineRegistryObjects(r.sessionID)
		if err != nil {
			return nil, err
		}
		record, err := sessionReadRecord(filepath.Join(s.registrationRoot(), r.sessionID, "record.json"))
		if err != nil || record != r.registration {
			return nil, errors.New("later invocation changed an observed public registration")
		}
		want := map[string]any{"schema": 1, "id": r.sessionID, "project": s.w.approval.project,
			"intent": s.registrationIntent(r), "state": "stopped", "workload_exit": r.config.Status, "last_problem": nil}
		if !r.absent {
			want["state"], want["last_problem"] = "failed", "cleanup-incomplete"
		}
		if !completedDetector(record.Bytes, want) {
			return nil, errors.New("public registration lost its original binding, workload exit or cleanup result")
		}
		maps.Copy(allowed, paths)
	}
	return allowed, nil
}

func (s *isolationWorld) registryPreserved() error {
	registry, err := s.registryPaths()
	if err != nil {
		return err
	}
	after := maps.Clone(s.after)
	for path := range registry {
		// Accept only validated additions; existing records and every other
		// path remain in the full host/checkout preservation comparison.
		if _, existed := s.before[path]; !existed {
			delete(after, path)
		}
	}
	if s.before == nil || s.after == nil || !reflect.DeepEqual(s.before, after) {
		return errors.New("unrelated checkout files, ownership or protected approval changed")
	}
	return nil
}
