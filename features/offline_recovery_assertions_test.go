package features

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func (w *world) offlineRegisteredRetry() error {
	s, r := w.session, w.session.create
	if !offlineHex(r.SessionID) || r.SessionID == s.token {
		return fmt.Errorf("cleanup did not retain an independent registered session ID")
	}
	retries := regexp.MustCompile(`itos container stop (\S+)`).FindAllStringSubmatch(r.Diagnostic, -1)
	if len(retries) == 0 || !strings.Contains(r.Diagnostic, "same effective project") {
		return fmt.Errorf("cleanup diagnostic lacks an executable same-project retry")
	}
	for _, retry := range retries {
		if retry[1] != r.SessionID {
			return fmt.Errorf("cleanup retry does not target the registered session")
		}
	}
	data := r.PublicRecords[r.SessionID]
	if s.backend == "disk" {
		path := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", w.approval.project, r.SessionID, "record.json")
		var err error
		data, err = os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("retry has no protected registration: %w", err)
		}
	} else if len(r.PublicRecords) != 1 {
		return fmt.Errorf("retry must bind exactly one public registration")
	}
	var record map[string]any
	if json.Unmarshal(data, &record) != nil || len(record) != 7 || record["schema"] != float64(1) || record["id"] != r.SessionID || record["project"] != w.approval.project || record["intent"] != s.reference {
		return fmt.Errorf("retry registration lost the original project or intent")
	}
	if record["state"] == "stopped" || (record["workload_exit"] != nil && record["workload_exit"] != float64(s.launch.child)) {
		return fmt.Errorf("incomplete cleanup fabricated completion or changed the workload exit")
	}
	return nil
}
