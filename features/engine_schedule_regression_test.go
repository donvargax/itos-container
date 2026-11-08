package features

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

const engineScheduleFixture = `@engine
Feature: independent runtime observations
  @ID-LIFECYCLE-09
  Scenario Outline: escalation
    Then observe escalation <row>
    Examples:
      | row |
      | 1   |
      | 2   |
      | 3   |
      | 4   |

  @ID-LIFECYCLE-13
  Scenario: quiet interval
    When observe quiet interval

  @ID-MEMORY-01
  Scenario: other engine case
    Then observe other case

  @wip @ID-MEMORY-02
  Scenario: deferred engine case
    Then never execute
`

func TestEngineScheduleOverlapsOnlyWithinQuietLifetime(t *testing.T) {
	dir := writeFeatureFixture(t, engineScheduleFixture)
	selection, err := selectScenarios("", dir, featureEnvironment{engine: "1", ci: "true", actions: "true", os: "linux", arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	quiet, escalating := false, false
	code := runFeatureScenarios(selection, godog.Options{Paths: []string{dir}, Format: "progress", Output: io.Discard, TestingT: t}, func(sc *godog.ScenarioContext, duringQuiet func()) {
		// Each escalation snapshots the same stable quiet resource. All nested
		// cleanup finishes before that resource leaves its running state.
		sc.Step(`^observe escalation (\d+)$`, func(_ int) error {
			if !quiet || escalating {
				return errors.New("escalation ran outside the stable quiet lifetime or concurrently with another escalation")
			}
			escalating = true
			events = append(events, "escalation")
			return nil
		})
		sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
			if escalating {
				events = append(events, "cleanup")
				escalating = false
			}
			return ctx, nil
		})
		sc.Step(`^observe quiet interval$`, func() error {
			quiet = true
			events = append(events, "ready")
			if duringQuiet != nil {
				duringQuiet()
				duringQuiet() // Re-entry must not duplicate any selected row.
			}
			if escalating {
				return errors.New("quiet worker stopped before nested cleanup")
			}
			events = append(events, "cancel")
			quiet = false
			return nil
		})
		sc.Step(`^observe other case$`, func() { events = append(events, "other") })
	})
	want := []string{"ready", "escalation", "cleanup", "escalation", "cleanup", "escalation", "cleanup", "escalation", "cleanup", "cancel", "other"}
	if code != 0 || !slices.Equal(events, want) {
		t.Fatalf("schedule code=%d events=%v, want %v", code, events, want)
	}
}

func TestEngineScheduleRetainsFailuresAndFallback(t *testing.T) {
	for _, tc := range []struct {
		name         string
		failure      error
		reachQuiet   bool
		missingCount bool
	}{
		{name: "assertion", reachQuiet: true, failure: errors.New("assertion failed")},
		{name: "pending", reachQuiet: true, failure: godog.ErrPending},
		{name: "skipped", reachQuiet: true, failure: godog.ErrSkip},
		{name: "missing selected row", reachQuiet: true, missingCount: true},
		{name: "quiet never ready", failure: errors.New("quiet startup failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeFeatureFixture(t, engineScheduleFixture)
			selection, err := selectScenarios("", dir, featureEnvironment{engine: "1", ci: "true", actions: "true", os: "linux", arch: "amd64"})
			if err != nil {
				t.Fatal(err)
			}
			if tc.missingCount {
				selection.count++
			}
			rows, others := 0, 0
			code := runFeatureScenarios(selection, godog.Options{Paths: []string{dir}, Format: "progress", Output: io.Discard}, func(sc *godog.ScenarioContext, duringQuiet func()) {
				sc.Step(`^observe escalation (\d+)$`, func(row int) error {
					rows++
					if row == 2 && tc.reachQuiet {
						return tc.failure
					}
					return nil
				})
				sc.Step(`^observe quiet interval$`, func() error {
					if !tc.reachQuiet {
						return tc.failure
					}
					if duringQuiet != nil {
						duringQuiet()
					}
					return nil
				})
				sc.Step(`^observe other case$`, func() { others++ })
			})
			if code == 0 || rows != 4 || others != 1 {
				t.Fatalf("failure lost or selected cases skipped: code=%d escalation rows=%d others=%d", code, rows, others)
			}
		})
	}
}

func TestEngineSchedulePreservesFilteredAndRedSelections(t *testing.T) {
	for _, tc := range []struct {
		name, pattern, red  string
		wantRows, wantQuiet int
	}{
		{name: "only escalation", pattern: "^@ID-LIFECYCLE-09$", wantRows: 4},
		{name: "only quiet", pattern: "^@ID-LIFECYCLE-13$", wantQuiet: 1},
		{name: "manual red", red: "@ID-LIFECYCLE-09 @ID-LIFECYCLE-13", wantRows: 4, wantQuiet: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeFeatureFixture(t, engineScheduleFixture)
			env := featureEnvironment{engine: "1", ci: "true", actions: "true", os: "linux", arch: "amd64", event: "workflow_dispatch", red: tc.red}
			selection, err := selectScenarios(tc.pattern, dir, env)
			if err != nil {
				t.Fatal(err)
			}
			rows, quiet := 0, 0
			code := runFeatureScenarios(selection, godog.Options{Paths: []string{dir}, Format: "progress", Output: io.Discard}, func(sc *godog.ScenarioContext, duringQuiet func()) {
				if duringQuiet != nil {
					t.Fatal("isolated or red selection enabled interleaving")
				}
				sc.Step(`^observe escalation (\d+)$`, func(_ int) { rows++ })
				sc.Step(`^observe quiet interval$`, func() { quiet++ })
			})
			if code != 0 || rows != tc.wantRows || quiet != tc.wantQuiet {
				t.Fatalf("selection changed: code=%d rows=%d quiet=%d", code, rows, quiet)
			}
		})
	}
}

func TestEngineScheduleIntersectsEverySelectionBranch(t *testing.T) {
	dir := writeFeatureFixture(t, engineScheduleFixture)
	selection, err := selectScenarios("^@ID-LIFECYCLE-09$|^@ID-LIFECYCLE-13$|^@wip$", dir,
		featureEnvironment{engine: "1", ci: "true", actions: "true", os: "linux", arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	rows, quiet := 0, 0
	active := false
	code := runFeatureScenarios(selection, godog.Options{Paths: []string{dir}, Format: "progress", Output: io.Discard}, func(sc *godog.ScenarioContext, duringQuiet func()) {
		sc.Step(`^observe escalation (\d+)$`, func(_ int) error {
			rows++
			if !active {
				return errors.New("OR branch escaped the quiet interval")
			}
			return nil
		})
		sc.Step(`^observe quiet interval$`, func() {
			quiet++
			active = true
			if duringQuiet != nil {
				duringQuiet()
			}
			active = false
		})
	})
	if code != 0 || rows != 4 || quiet != 1 {
		t.Fatalf("OR selection broadened, lost or duplicated cases: code=%d rows=%d quiet=%d", code, rows, quiet)
	}
}

func TestEngineScheduleLeavesOrdinarySelectionsSerial(t *testing.T) {
	dir := writeFeatureFixture(t, strings.ReplaceAll(engineScheduleFixture, "@engine\n", ""))
	selection, err := selectScenarios("", dir, featureEnvironment{})
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	code := runFeatureScenarios(selection, godog.Options{Paths: []string{dir}, Format: "progress", Output: io.Discard}, func(sc *godog.ScenarioContext, duringQuiet func()) {
		if duringQuiet != nil {
			t.Fatal("ordinary selection enabled engine scheduling")
		}
		sc.Step(`^observe escalation (\d+)$`, func(_ int) { events = append(events, "escalation") })
		sc.Step(`^observe quiet interval$`, func() { events = append(events, "quiet") })
		sc.Step(`^observe other case$`, func() { events = append(events, "other") })
	})
	if code != 0 || !slices.Equal(events, []string{"escalation", "escalation", "escalation", "escalation", "quiet", "other"}) {
		t.Fatalf("ordinary schedule changed: code=%d events=%v", code, events)
	}
}
