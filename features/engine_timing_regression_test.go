package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
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
					if filepath.Clean(record.Feature) != filepath.Join(dir, "memory.feature") || record.ID != "@ID-MEMORY-01" || record.Line != int64(8+i) || record.DurationNS != int64(want) || record.Outcome != outcome {
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
	t.Run("LIFECYCLE-09 rows carry their signal offsets", func(t *testing.T) {
		// The real world's Before hook would otherwise make a coverage directory per ID.
		t.Setenv("GOCOVERDIR", "")
		t.Setenv("ITOS_CC_TEST_COVERDIR", "")
		const fixture = `@engine
Feature: signal offsets
  @ID-LIFECYCLE-09
  Scenario Outline: escalation
    Given the escalation journal "<journal>"
    Then the row <verdict>
    Examples:
      | journal             | verdict |
      | immediate           | passes  |
      | delayed             | passes  |
      | cut after the check | fails   |
      | no signal request   | passes  |
      | never signalled     | passes  |
      | no journal          | passes  |

  @ID-LIFECYCLE-08
  Scenario: another row
    Given the escalation journal "immediate"
    Then the row passes
`
		// The journals of TestLifecycleSignalWindow's cases, each with a fault so that
		// the world's closeLifecycleNative runs, as it does for a collected row.
		built := func(signal string, percent int, name string, withoutSignals bool) *lifecycleNative {
			delay := time.Duration(percent) * engineGraceBudget / 100
			cases := lifecycleImmediateWindowCases()
			if percent != 0 {
				cases = lifecycleDelayedWindowCases(delay)
			}
			i := slices.IndexFunc(cases, func(c lifecycleWindowCase) bool { return c.name == name })
			if i < 0 {
				t.Fatalf("no window case %q", name)
			}
			l := lifecycleWindowWorld(t, signal, delay, cases[i]).lifecycle
			l.fault = newLifecycleFaults("ignore", "", delay)
			if withoutSignals {
				l.journalAtReturn = slices.DeleteFunc(slices.Clone(l.journalAtReturn), lifecycleSignalRequest)
			}
			return l
		}
		journals := map[string]*lifecycleNative{
			"immediate":           built("SIGINT", 0, "immediate at the grace", false),
			"delayed":             built("SIGTERM", 60, "delayed slow teardown", false),
			"cut after the check": built("SIGTERM", 60, "delayed acknowledgement cut a call limit after the daemon check", false),
			"no signal request":   built("SIGINT", 0, "immediate at the grace", true),
			"never signalled":     built("SIGINT", 0, "immediate at the grace", false),
			"no journal":          built("SIGINT", 0, "immediate at the grace", false),
		}
		// Keep requests without a signal time, and a signal time without requests,
		// so each reason to omit signals is exercised independently.
		journals["never signalled"].signalled = time.Time{}
		journals["no journal"].journalAtReturn = nil
		dir, selection := timingSelection(t, fixture, true)
		var output bytes.Buffer
		code := runFeatureScenarios(selection, godog.Options{Paths: []string{dir}, Format: "progress", Output: &output}, func(sc *godog.ScenarioContext, _ func()) {
			w := initializeScenario(sc, t.TempDir(), "unused")
			sc.Step(`^the escalation journal "([^"]*)"$`, func(name string) error {
				journal, ok := journals[name]
				if !ok {
					return fmt.Errorf("no journal %q", name)
				}
				copied := *journal
				w.lifecycle = &copied
				return nil
			})
			sc.Step(`^the row passes$`, func() {})
			sc.Step(`^the row fails$`, func() error { return errors.New("the row fails") })
		})
		type signalledRecord struct {
			scenarioTimingRecord
			Signals json.RawMessage `json:"signals"`
		}
		rows := timingRecords[signalledRecord](t, output.String(), "engine scenario timing: ")
		ms := func(n int) int64 { return int64(n) * int64(time.Millisecond) }
		signal := func(signal string, at int, replied string, cancelled bool, previous int, next string) string {
			return fmt.Sprintf(`{"signal":%q,"at_ns":%d%s,"status":204,"cancelled":%t,"previous_ns":%d%s}`, signal, ms(at), replied, cancelled, ms(previous), next)
		}
		replied := func(at int) string { return fmt.Sprintf(`,"replied_ns":%d`, ms(at)) }
		next := func(at int) string { return fmt.Sprintf(`,"next_ns":%d`, ms(at)) }
		type expectation struct {
			id, outcome string
			signals     *string // nil: no signals field
		}
		wants := []expectation{
			{"@ID-LIFECYCLE-09", "passed", ptr("[" + signal("SIGINT", 50, replied(50), false, 50, next(1000)) + "," + signal("SIGKILL", 1000, replied(1000), false, 1000, "") + "]")},
			{"@ID-LIFECYCLE-09", "passed", ptr("[" + signal("SIGTERM", 500, replied(1100), false, 500, next(1300)) + "," + signal("SIGKILL", 1300, replied(1300), false, 1300, "") + "]")},
			{"@ID-LIFECYCLE-09", "failed", ptr("[" + signal("SIGTERM", 650, "", true, 600, next(1620)) + "," + signal("SIGKILL", 1620, replied(1620), false, 1620, "") + "]")},
			{"@ID-LIFECYCLE-09", "passed", ptr("[]")},
			{"@ID-LIFECYCLE-09", "passed", nil},
			{"@ID-LIFECYCLE-09", "passed", nil},
			{"@ID-LIFECYCLE-08", "passed", nil},
		}
		if code == 0 || len(rows) != len(wants) {
			t.Fatalf("exit %d, %d records, want a failing run of %d: %s", code, len(rows), len(wants), output.String())
		}
		for i, want := range wants {
			got := rows[i]
			if got.ID != want.id || got.Outcome != want.outcome {
				t.Errorf("row %d is %s %s, want %s %s", i, got.ID, got.Outcome, want.id, want.outcome)
			}
			switch {
			case want.signals == nil && got.Signals != nil:
				t.Errorf("row %d (%s) carries signals %s, want none", i, got.ID, got.Signals)
			case want.signals != nil && string(got.Signals) != *want.signals:
				seen := "no signals field"
				if got.Signals != nil {
					seen = string(got.Signals)
				}
				t.Errorf("row %d (%s) has %s\nwant %s", i, got.ID, seen, *want.signals)
			}
		}
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
