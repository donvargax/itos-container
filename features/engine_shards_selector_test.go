package features

import (
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// The selector half of TestEngineShards: fixture features with memory-only steps,
// run through runFeatureScenarios as the schedule regressions do.

const (
	shardAlpha = `@engine
Feature: alpha
  @ID-SHARD-01
  Scenario: alpha one
    Then record "alpha-1"

  @ID-SHARD-02
  Scenario Outline: alpha rows
    Then record "alpha-<row>"
    Examples:
      | row |
      | a   |
      | b   |
`
	shardBeta = `@engine
Feature: beta
  @ID-SHARD-03
  Scenario: beta one
    Then record "beta-1"

  @wip @ID-SHARD-04
  Scenario: beta future
    Then record "beta-future"
`
	shardPlain = `Feature: plain
  @ID-SHARD-05
  Scenario: plain one
    Then record "plain-1"
`
	shardFuture = `@engine @wip
Feature: future
  @ID-SHARD-06
  Scenario: only future
    Then record "future-1"
`
	shardElsewhere = `@engine
Feature: elsewhere
  @ID-OTHER-01
  Scenario: elsewhere
    Then observe elsewhere
`
)

func writeShardFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runShard(selection scenarioSelection, dir string) ([]string, int) {
	var steps []string
	code := runFeatureScenarios(selection, godog.Options{Paths: []string{dir}, Format: "progress", Output: io.Discard}, func(sc *godog.ScenarioContext, _ func()) {
		sc.Step(`^record "([^"]*)"$`, func(value string) { steps = append(steps, value) })
	})
	slices.Sort(steps)
	return steps, code
}

func testShardSelector(t *testing.T) {
	files := map[string]string{
		"alpha.feature": shardAlpha, "beta.feature": shardBeta, "plain.feature": shardPlain, "future.feature": shardFuture,
	}
	dir := writeShardFixture(t, files)
	red := func(ids string) map[string]string {
		return map[string]string{"GITHUB_EVENT_NAME": "workflow_dispatch", redVariable: ids}
	}
	with := func(features string, extra map[string]string) map[string]string {
		values := map[string]string{shardFeaturesVariable: features}
		maps.Copy(values, extra)
		return values
	}
	t.Run("only the named files' rows run", func(t *testing.T) {
		for _, tc := range []struct {
			features string
			steps    []string
			ids      []string
		}{
			{"alpha.feature", []string{"alpha-1", "alpha-a", "alpha-b"}, []string{"@ID-SHARD-01", "@ID-SHARD-02"}},
			{"beta.feature", []string{"beta-1"}, []string{"@ID-SHARD-03"}},
			{"  beta.feature\n\talpha.feature ", []string{"alpha-1", "alpha-a", "alpha-b", "beta-1"}, []string{"@ID-SHARD-01", "@ID-SHARD-02", "@ID-SHARD-03"}},
			{"", []string{"alpha-1", "alpha-a", "alpha-b", "beta-1"}, []string{"@ID-SHARD-01", "@ID-SHARD-02", "@ID-SHARD-03"}},
			{" \n\t", []string{"alpha-1", "alpha-a", "alpha-b", "beta-1"}, []string{"@ID-SHARD-01", "@ID-SHARD-02", "@ID-SHARD-03"}},
		} {
			selection, err := selectScenarios("", dir, shardEnvironment(with(tc.features, nil)))
			if err != nil {
				t.Errorf("%q: %v", tc.features, err)
				continue
			}
			steps, code := runShard(selection, dir)
			if code != 0 || !slices.Equal(steps, tc.steps) || selection.count != len(tc.steps) || !slices.Equal(selection.ids, tc.ids) {
				t.Errorf("%q ran %v (code %d), selected %d row(s) %v; want %v, %v", tc.features, steps, code, selection.count, selection.ids, tc.steps, tc.ids)
			}
		}
	})
	t.Run("refusals precede any run", func(t *testing.T) {
		for _, tc := range []struct {
			name, features, engine, want string
		}{
			{"outside the engine tier", "alpha.feature", "", shardFeaturesVariable},
			{"engine switch off", "alpha.feature", "0", shardFeaturesVariable},
			{"relative path", "sub/alpha.feature", "1", "sub/alpha.feature"},
			{"parent path", "../alpha.feature", "1", "../alpha.feature"},
			{"dot path", "./alpha.feature", "1", "./alpha.feature"},
			{"absolute path", "/tmp/alpha.feature", "1", "/tmp/alpha.feature"},
			{"backslash path", `sub\alpha.feature`, "1", `sub\alpha.feature`},
			{"line suffix", "alpha.feature:2", "1", "alpha.feature:2"},
			{"no extension", "alpha", "1", "alpha"},
			{"wrong extension", "alpha.txt", "1", "alpha.txt"},
			{"only an extension", ".feature", "1", ".feature"},
			{"capital extension", "alpha.FEATURE", "1", "alpha.FEATURE"},
			{"given twice", "alpha.feature beta.feature alpha.feature", "1", "alpha.feature"},
			{"no engine case", "alpha.feature plain.feature", "1", "plain.feature"},
			{"no such file", "alpha.feature absent.feature", "1", "absent.feature"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				extra := map[string]string{shardFeaturesVariable: tc.features, "ITOS_CONTAINER_ENGINE_TESTS": tc.engine}
				selection, err := selectScenarios("", dir, shardEnvironment(extra))
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("accepted %q, or did not name %q: %v", tc.features, tc.want, err)
				}
				if selection.count != 0 || len(selection.ids) != 0 {
					t.Errorf("a refused selection still selected rows: %+v", selection)
				}
			})
		}
	})
	t.Run("a file with only future engine cases is accepted and selects nothing", func(t *testing.T) {
		selection, err := selectScenarios("", dir, shardEnvironment(with("future.feature", nil)))
		if err != nil {
			t.Fatal(err)
		}
		if selection.count != 0 || !strings.Contains(selection.skip, "no live @engine scenarios") {
			t.Errorf("want the empty engine skip, got count %d skip %q", selection.count, selection.skip)
		}
	})
	t.Run("red IDs", func(t *testing.T) {
		for _, tc := range []struct {
			name, features, ids string
			steps               []string
			skips               []string
		}{
			{name: "own row", features: "alpha.feature", ids: "@ID-SHARD-01", steps: []string{"alpha-1"}},
			{name: "own and another file's rows", features: "alpha.feature", ids: "@ID-SHARD-01 @ID-SHARD-03", steps: []string{"alpha-1"}},
			{name: "another file's row", features: "alpha.feature", ids: "@ID-SHARD-03", skips: []string{"@ID-SHARD-03"}},
			{name: "another file's future row", features: "alpha.feature", ids: "@ID-SHARD-04", skips: []string{"@ID-SHARD-04"}},
			{name: "two other files' rows", features: "beta.feature", ids: "@ID-SHARD-01 @ID-SHARD-02", skips: []string{"@ID-SHARD-01", "@ID-SHARD-02"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				selection, err := selectScenarios("", dir, shardEnvironment(with(tc.features, red(tc.ids))))
				if err != nil {
					t.Fatal(err)
				}
				steps, code := runShard(selection, dir)
				if code != 0 || !slices.Equal(steps, tc.steps) {
					t.Errorf("ran %v (code %d), want %v", steps, code, tc.steps)
				}
				if (len(tc.skips) == 0) != (selection.skip == "") {
					t.Fatalf("skip %q, want a skip naming %v", selection.skip, tc.skips)
				}
				for _, id := range tc.skips {
					if !strings.Contains(selection.skip, id) {
						t.Errorf("the skip does not name %s, left to another shard: %q", id, selection.skip)
					}
				}
			})
		}
		t.Run("an unknown ID still fails", func(t *testing.T) {
			if _, err := selectScenarios("", dir, shardEnvironment(with("alpha.feature", red("@ID-SHARD-99")))); err == nil || !strings.Contains(err.Error(), "@ID-SHARD-99") {
				t.Fatalf("unknown red ID accepted, or not named: %v", err)
			}
		})
		t.Run("an ID of another tier is left", func(t *testing.T) {
			selection, err := selectScenarios("", dir, shardEnvironment(with("alpha.feature", red("@ID-SHARD-05"))))
			if err != nil || selection.skip == "" || selection.count != 0 {
				t.Fatalf("want a skip, got count %d skip %q: %v", selection.count, selection.skip, err)
			}
		})
	})
	t.Run("a -scenarios expression intersects", func(t *testing.T) {
		for _, tc := range []struct {
			pattern string
			steps   []string
			skip    bool
		}{
			{"^@ID-SHARD-0[13]$", []string{"alpha-1"}, false},
			{"^@ID-SHARD-02$", []string{"alpha-a", "alpha-b"}, false},
			{"^@ID-SHARD-03$", nil, true},
		} {
			selection, err := selectScenarios(tc.pattern, dir, shardEnvironment(with("alpha.feature", nil)))
			if err != nil {
				t.Errorf("%s: %v", tc.pattern, err)
				continue
			}
			steps, code := runShard(selection, dir)
			if code != 0 || !slices.Equal(steps, tc.steps) || (selection.skip != "") != tc.skip {
				t.Errorf("%s ran %v (code %d, skip %q); want %v, skip %t", tc.pattern, steps, code, selection.skip, tc.steps, tc.skip)
			}
		}
		if _, err := selectScenarios("^@ID-SHARD-99$", dir, shardEnvironment(with("alpha.feature", nil))); err == nil {
			t.Error("an expression matching no row of any file was accepted")
		}
	})
	t.Run("the nested schedule follows the shard's files", func(t *testing.T) {
		dir := writeShardFixture(t, map[string]string{"lifecycle.feature": engineScheduleFixture, "elsewhere.feature": shardElsewhere})
		selection, err := selectScenarios("", dir, shardEnvironment(with("lifecycle.feature", nil)))
		if err != nil {
			t.Fatal(err)
		}
		var events []string
		quiet := false
		code := runFeatureScenarios(selection, godog.Options{Paths: []string{dir}, Format: "progress", Output: io.Discard}, func(sc *godog.ScenarioContext, duringQuiet func()) {
			sc.Step(`^observe escalation (\d+)$`, func(int) error {
				if !quiet {
					return fmt.Errorf("an escalation ran outside the quiet interval")
				}
				events = append(events, "escalation")
				return nil
			})
			sc.Step(`^observe quiet interval$`, func() {
				quiet = true
				if duringQuiet != nil {
					duringQuiet()
				}
				quiet = false
				events = append(events, "quiet")
			})
			sc.Step(`^observe other case$`, func() { events = append(events, "other") })
			sc.Step(`^observe elsewhere$`, func() { events = append(events, "elsewhere") })
		})
		want := []string{"escalation", "escalation", "escalation", "escalation", "quiet", "other"}
		if code != 0 || !slices.Equal(events, want) || selection.count != 6 {
			t.Errorf("code %d, count %d, events %v; want only the lifecycle file's rows %v", code, selection.count, events, want)
		}
	})
}
