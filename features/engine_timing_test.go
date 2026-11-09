package features

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

type scenarioTimingRecord struct {
	Feature    string `json:"feature"`
	ID         string `json:"id"`
	Line       int64  `json:"line"`
	DurationNS int64  `json:"duration_ns"`
	Outcome    string `json:"outcome"`
}

type suiteTimingRecord struct {
	Selected   int    `json:"selected"`
	Completed  int    `json:"completed"`
	DurationNS int64  `json:"duration_ns"`
	Outcome    string `json:"outcome"`
}

// One collector spans the outer and nested godog suites. Each row gets its own
// monotonic origin; elapsed suite time is measured independently, never summed.
type engineTiming struct {
	output    io.Writer
	started   time.Time
	selected  int
	completed int
	failed    bool
}

func timingOutcome(failed bool) string {
	if failed {
		return "failed"
	}
	return "passed"
}

func (timing *engineTiming) write(kind string, record any) {
	if timing.output == nil {
		return
	}
	data, err := json.Marshal(record)
	if err == nil {
		// The newline also separates records from godog's progress formatter.
		_, err = fmt.Fprintf(timing.output, "\nengine %s timing: %s\n", kind, data)
	}
	timing.failed = timing.failed || err != nil
}

func (timing *engineTiming) complete(scenario *godog.Scenario, started time.Time, failed bool) {
	record := scenarioTimingRecord{Feature: scenario.Uri, DurationNS: int64(time.Since(started)), Outcome: timingOutcome(failed)}
	for _, tag := range scenario.Tags {
		if strings.HasPrefix(tag.Name, "@ID-") {
			record.ID = tag.Name
			break
		}
	}
	if scenario.Location != nil {
		record.Line = scenario.Location.Line
	}
	timing.completed++
	timing.failed = timing.failed || failed
	timing.write("scenario", record)
}

func (timing *engineTiming) finish(code int) int {
	timing.failed = timing.failed || code != 0 || timing.completed != timing.selected
	timing.write("suite", suiteTimingRecord{
		Selected: timing.selected, Completed: timing.completed,
		DurationNS: int64(time.Since(timing.started)), Outcome: timingOutcome(timing.failed),
	})
	if code == 0 && timing.failed {
		return 1
	}
	return code
}
