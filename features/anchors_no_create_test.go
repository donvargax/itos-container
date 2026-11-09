package features

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cucumber/godog"
)

func (w *world) anchorRecoveryNoCreateSteps(sc *godog.ScenarioContext) {
	sc.Step(`^an anchor recovery fixture with an allocated volume and a no-create "([^"]*)" intent$`, func(stage string) error {
		if err := w.anchorRecoverySeed("preparing", "absent", "present", "pending", "unknown"); err != nil {
			return err
		}
		a := w.anchorLocalRecovery
		a.mode, a.fault = "no-create", stage
		a.intent["stage"], a.intent["container_id"] = stage, ""
		if err := os.WriteFile(w.sessionRecordPath(), approvalJSON(a.intent), 0600); err != nil {
			return err
		}
		return w.recoveryCapture()
	})
	sc.Step(`^both stops and subsequent status report stopped worker absent anchor absent cleanup complete and unknown workload exit$`, func() error {
		return errors.Join(w.anchorRecoveryStopResult(0, "unknown"), w.anchorRecoveryStopResult(1, "unknown"), w.anchorRecoveryStopResult(2, "unknown"), w.anchorRecoveryMemoryResult())
	})
	sc.Step(`^the no-create intent is durably cancelled with one owned anchor release and no worker lookup or mutation$`, w.anchorRecoveryNoCreateDurable)
}

func (w *world) anchorRecoveryNoCreateDurable() error {
	a := w.anchorLocalRecovery
	deletes, inspections := 0, 0
	for _, q := range w.sessionRequests() {
		if strings.HasPrefix(q.Path, "/containers/") && (q.Method != "GET" || q.Path != "/containers/"+strings.Repeat("c", 63)+"2/json") {
			return fmt.Errorf("no-create session contacted a worker: %s %s", q.Method, q.Path)
		}
		if q.Path == "/volumes/"+a.name {
			if q.Method == "DELETE" {
				deletes++
			}
			if q.Method == "GET" {
				inspections++
			}
		}
	}
	if deletes != 1 || inspections == 0 || w.session.api.volumes[a.name] != nil {
		return fmt.Errorf("missing independent anchor cleanup: deletes=%d inspections=%d", deletes, inspections)
	}
	return w.anchorRecoveryRepeated()
}
