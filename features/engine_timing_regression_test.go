package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cucumber/godog"
)

// Decode only the timing lines: godog's ordinary diagnostic output may contain
// step text, while timing records must contain identity, elapsed time and outcome.
func timingRecords[T any](t *testing.T, output, prefix string) []T {
	t.Helper()
	var records []T
	for _, line := range strings.Split(output, "\n") {
		if _, rest, ok := strings.Cut(line, prefix); ok {
			var record T
			decoder := json.NewDecoder(strings.NewReader(rest))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&record); err != nil {
				t.Fatalf("invalid timing record: %v", err)
			}
			records = append(records, record)
		}
	}
	return records
}

func timingSelection(t *testing.T, source string, engine bool) (string, scenarioSelection) {
	t.Helper()
	dir := writeFeatureFixture(t, source)
	env := featureEnvironment{}
	if engine {
		env = featureEnvironment{engine: "1", ci: "true", actions: "true", os: "linux", arch: "amd64"}
	}
	selection, err := selectScenarios("", dir, env)
	if err != nil {
		t.Fatal(err)
	}
	return dir, selection
}

func TestEngineScenarioTiming(t *testing.T) {
	const source = `@engine
Feature: timing
  @ID-MEMORY-01
  Scenario Outline: identical names and values
    Then spend time
    Examples:
      | value |
      | same  |
      | same  |
      | same  |
      | same  |
      | same  |
`
	for _, failure := range []string{"setup", "step", "cleanup", "pending", "skip", "undefined"} {
		t.Run(failure, func(t *testing.T) {
			dir, selection := timingSelection(t, source, true)
			synctest.Test(t, func(t *testing.T) {
				var output bytes.Buffer
				row := 0
				code := runFeatureScenarios(selection, godog.Options{Paths: []string{dir}, Format: "progress", Output: &output}, func(sc *godog.ScenarioContext, _ func()) {
					sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
						row++
						if row == 3 && len(timingRecords[scenarioTimingRecord](t, output.String(), "engine scenario timing: ")) != 2 {
							t.Error("completed records were not streamed before the next row's setup")
						}
						time.Sleep(2 * time.Second)
						if row == 2 && failure == "setup" {
							return ctx, errors.New("private fixture setup detail")
						}
						return ctx, nil
					})
					sc.Step(`^spend time$`, func() error {
						time.Sleep(3 * time.Second)
						if row == 2 {
							switch failure {
							case "step":
								return errors.New("private workload detail")
							case "pending":
								return godog.ErrPending
							case "skip":
								return godog.ErrSkip
							case "undefined":
								return godog.ErrUndefined
							}
						}
						return nil
					})
					sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
						time.Sleep(5 * time.Second)
						if row == 2 && failure == "cleanup" {
							return ctx, errors.New("private cleanup detail")
						}
						return ctx, nil
					})
				})
				rows := timingRecords[scenarioTimingRecord](t, output.String(), "engine scenario timing: ")
				if code == 0 || len(rows) != 5 {
					t.Fatalf("failure or timing lost: exit %d, completed records %d", code, len(rows))
				}
				var total time.Duration
				for i, record := range rows {
					want := 10 * time.Second
					outcome := "passed"
					if i == 1 {
						outcome = "failed"
						if failure == "setup" {
							want -= 3 * time.Second
						}
					}
					total += want
					if record.Feature != filepath.Join(dir, "memory.feature") || record.ID != "@ID-MEMORY-01" || record.Line != int64(8+i) || record.DurationNS != int64(want) || record.Outcome != outcome {
						t.Errorf("row %d lost identity, setup/cleanup time or outcome: %+v", i, record)
					}
				}
				summaries := timingRecords[suiteTimingRecord](t, output.String(), "engine suite timing: ")
				want := suiteTimingRecord{Selected: 5, Completed: 5, DurationNS: int64(total), Outcome: "failed"}
				if len(summaries) != 1 || summaries[0] != want {
					t.Errorf("suite summary = %+v, want %+v", summaries, want)
				}
			})
		})
	}
	t.Run("overlapping rows are not summed", func(t *testing.T) {
		dir, selection := timingSelection(t, engineScheduleFixture, true)
		synctest.Test(t, func(t *testing.T) {
			var output bytes.Buffer
			code := runFeatureScenarios(selection, godog.Options{Paths: []string{dir}, Format: "progress", Output: &output}, func(sc *godog.ScenarioContext, duringQuiet func()) {
				sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
					time.Sleep(2 * time.Second)
					return ctx, nil
				})
				sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
					time.Sleep(5 * time.Second)
					return ctx, nil
				})
				sc.Step(`^observe escalation (\d+)$`, func(_ int) { time.Sleep(3 * time.Second) })
				sc.Step(`^observe other case$`, func() { time.Sleep(3 * time.Second) })
				sc.Step(`^observe quiet interval$`, func() {
					until := time.Now().Add(125 * time.Second)
					if duringQuiet != nil {
						duringQuiet()
					}
					time.Sleep(time.Until(until))
				})
			})
			rows := timingRecords[scenarioTimingRecord](t, output.String(), "engine scenario timing: ")
			if code != 0 || len(rows) != 6 {
				t.Fatalf("nested exit %d, completed records %d", code, len(rows))
			}
			var sum int64
			for _, row := range rows {
				sum += row.DurationNS
				want := 10 * time.Second
				if row.ID == "@ID-LIFECYCLE-13" {
					want = 132 * time.Second
				}
				if row.DurationNS != int64(want) || row.Outcome != "passed" {
					t.Errorf("nested timing lost: %+v", row)
				}
			}
			summaries := timingRecords[suiteTimingRecord](t, output.String(), "engine suite timing: ")
			want := suiteTimingRecord{Selected: 6, Completed: 6, DurationNS: int64(142 * time.Second), Outcome: "passed"}
			if len(summaries) != 1 || summaries[0] != want || sum <= want.DurationNS {
				t.Errorf("nested elapsed time double counted: %+v, row sum %s", summaries, time.Duration(sum))
			}
		})
	})
	t.Run("ordinary tier has no timing records", func(t *testing.T) {
		dir, selection := timingSelection(t, strings.TrimPrefix(source, "@engine\n"), false)
		var output bytes.Buffer
		code := runFeatureScenarios(selection, godog.Options{Paths: []string{dir}, Format: "progress", Output: &output}, func(sc *godog.ScenarioContext, _ func()) {
			sc.Step(`^spend time$`, func() {})
		})
		if code != 0 || strings.Contains(output.String(), "engine scenario timing:") || strings.Contains(output.String(), "engine suite timing:") {
			t.Fatal("timing escaped the engine tier")
		}
	})
	t.Run("missing selected row fails the suite summary", func(t *testing.T) {
		dir, selection := timingSelection(t, source, true)
		selection.count++
		var output bytes.Buffer
		code := runFeatureScenarios(selection, godog.Options{Paths: []string{dir}, Format: "progress", Output: &output}, func(sc *godog.ScenarioContext, _ func()) {
			sc.Step(`^spend time$`, func() {})
		})
		summaries := timingRecords[suiteTimingRecord](t, output.String(), "engine suite timing: ")
		if code == 0 || len(summaries) != 1 || summaries[0].Completed != 5 || summaries[0].Selected != 6 || summaries[0].Outcome != "failed" {
			t.Fatalf("missing row was concealed: code=%d, summaries=%+v", code, summaries)
		}
	})
	t.Run("suite includes the product build interval", func(t *testing.T) {
		dir, selection := timingSelection(t, source, true)
		synctest.Test(t, func(t *testing.T) {
			selection.started = time.Now()
			time.Sleep(11 * time.Second)
			var output bytes.Buffer
			code := runFeatureScenarios(selection, godog.Options{Paths: []string{dir}, Format: "progress", Output: &output}, func(sc *godog.ScenarioContext, _ func()) {
				sc.Step(`^spend time$`, func() {})
			})
			summaries := timingRecords[suiteTimingRecord](t, output.String(), "engine suite timing: ")
			if code != 0 || len(summaries) != 1 || summaries[0].DurationNS != int64(11*time.Second) {
				t.Fatalf("product build interval lost: code=%d, summaries=%+v", code, summaries)
			}
		})
	})
}
