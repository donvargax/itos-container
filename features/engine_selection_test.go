package features

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

const tierFixture = `Feature: tier selection
  Background:
    Given a memory background

  @ID-MEMORY-01
  Scenario: ordinary
    Then record "ordinary"

  @engine @ID-MEMORY-02
  Scenario: engine sentinel
    Then record "engine"

  @wip @ID-MEMORY-03
  Scenario: ordinary wip
    Then record "wip"

  @engine @wip @ID-MEMORY-04
  Scenario: engine wip
    Then record "engine-wip"
`

func TestEngineSelection(t *testing.T) {
	allowed := featureEnvironment{engine: "1", ci: "true", actions: "true", os: "linux", arch: "amd64"}
	for _, tc := range []struct {
		name, pattern string
		env           featureEnvironment
		want          []string
	}{
		{name: "default excludes engine sentinel", want: []string{"ordinary"}},
		{name: "zero excludes engine sentinel", env: featureEnvironment{engine: "0"}, want: []string{"ordinary"}},
		{name: "enabled excludes ordinary", env: allowed, want: []string{"engine"}},
		{name: "ordinary mixed OR", pattern: "^@ID-MEMORY-01$|^@engine$|^@wip$", want: []string{"ordinary"}},
		{name: "engine mixed OR", pattern: "^@ID-MEMORY-01$|^@engine$|^@wip$", env: allowed, want: []string{"engine"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeFeatureFixture(t, tierFixture)
			selected, err := selectScenarios(tc.pattern, dir, tc.env)
			if err != nil {
				t.Fatal(err)
			}
			seen := executeMemory(t, dir, selected, nil)
			expectedID := "@ID-MEMORY-01"
			if tc.env.engine == "1" {
				expectedID = "@ID-MEMORY-02"
			}
			if !slices.Equal(seen.ids, []string{expectedID}) {
				t.Errorf("executed IDs %v; want %s", seen.ids, expectedID)
			}
			if selected.count != len(tc.want) {
				t.Errorf("selected count %d; want %d", selected.count, len(tc.want))
			}
			if len(selected.excluded) != 3 {
				t.Errorf("excluded %v; want three cases", selected.excluded)
			}
			if !slices.Equal(seen.steps, tc.want) {
				t.Errorf("executed %v; want %v", seen.steps, tc.want)
			}
			if seen.initialized != len(tc.want) || seen.before != len(tc.want) || seen.after != len(tc.want) || seen.background != len(tc.want) {
				t.Errorf("excluded scenario initialized hooks/background: %+v", seen)
			}
		})
	}
	t.Run("workflow-style opt in is refused outside allowed context", func(t *testing.T) {
		for _, env := range []featureEnvironment{
			{engine: "yes"}, {engine: " 1"}, {engine: "false"},
			{engine: "1", ci: "true", actions: "true", os: "darwin", arch: "amd64"},
			{engine: "1", ci: "true", actions: "true", os: "linux", arch: "arm64"},
			{engine: "1", ci: "false", actions: "true", os: "linux", arch: "amd64"},
			{engine: "1", ci: "true", actions: "", os: "linux", arch: "amd64"},
		} {
			if _, err := selectScenarios("", writeFeatureFixture(t, tierFixture), env); err == nil {
				t.Errorf("accepted unsupported context %+v", env)
			}
		}
	})
	for _, pattern := range []string{"^@engine$", "^@wip$", "^@ID-MEMORY-04$"} {
		t.Run("explicit skip "+pattern, func(t *testing.T) {
			selected, err := selectScenarios(pattern, writeFeatureFixture(t, tierFixture), featureEnvironment{})
			if err != nil {
				t.Fatal(err)
			}
			seen := executeMemory(t, writeFeatureFixture(t, tierFixture), selected, nil)
			if seen.initialized != 0 {
				t.Errorf("skip initialized %d scenarios", seen.initialized)
			}
			if selected.skip == "" || len(selected.excluded) == 0 {
				t.Errorf("want explicit skip with exclusions, got %+v", selected)
			}
		})
	}
	t.Run("invalid and unmatched selection", func(t *testing.T) {
		dir := writeFeatureFixture(t, tierFixture)
		for _, pattern := range []string{"[", "^@absent$"} {
			if _, err := selectScenarios(pattern, dir, featureEnvironment{}); err == nil {
				t.Errorf("accepted %q", pattern)
			}
		}
	})
	t.Run("empty engine bootstrap", func(t *testing.T) {
		dir := writeFeatureFixture(t, strings.ReplaceAll(tierFixture, "@engine @ID-MEMORY-02", "@engine @wip @ID-MEMORY-02"))
		selected, err := selectScenarios("", dir, allowed)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(selected.skip, "no live @engine scenarios") || selected.count != 0 {
			t.Errorf("want explicit empty engine bootstrap, got %+v", selected)
		}
	})
	t.Run("inheritance and outline rows", testInheritedEngineTags)
	t.Run("parse and read errors precede hooks", testEngineFeatureErrors)
	t.Run("runtime failures cannot turn green", func(t *testing.T) {
		for name, failure := range map[string]error{
			"step failure": errors.New("memory assertion failed"),
			"pending":      godog.ErrPending,
			"skipped":      godog.ErrSkip,
		} {
			t.Run(name, func(t *testing.T) {
				dir := writeFeatureFixture(t, tierFixture)
				selected, err := selectScenarios("", dir, allowed)
				if err != nil {
					t.Fatal(err)
				}
				seen := executeMemory(t, dir, selected, failure)
				if seen.code == 0 {
					t.Errorf("engine %s passed", name)
				}
				if !slices.Equal(seen.steps, []string{"engine"}) {
					t.Errorf("lost live sentinel: %+v", seen)
				}
			})
		}
		for _, text := range []string{
			"@engine\nFeature: undefined\n  Scenario: undefined\n    Then an undefined memory step\n",
			"@engine\nFeature: no steps\n  Scenario: empty\n",
		} {
			dir := writeFeatureFixture(t, text)
			selected, err := selectScenarios("", dir, allowed)
			if err != nil {
				t.Fatal(err)
			}
			result := runScenarios(selected, godog.Options{Paths: []string{dir}, Format: "progress", Output: io.Discard}, func(*godog.ScenarioContext) {})
			if result == 0 {
				t.Errorf("incomplete engine scenario passed: %s", text)
			}
		}
	})
	t.Run("engine hooks cannot skip runtime evidence", func(t *testing.T) {
		for _, where := range []string{"before", "after", "after step"} {
			t.Run(where, func(t *testing.T) {
				dir := writeFeatureFixture(t, "@engine\nFeature: hooks\n  Scenario: runtime\n    Then memory passes\n")
				selected, err := selectScenarios("", dir, allowed)
				if err != nil {
					t.Fatal(err)
				}
				code := runScenarios(selected, godog.Options{Paths: []string{dir}, Format: "progress", Output: io.Discard}, func(sc *godog.ScenarioContext) {
					sc.Step(`^memory passes$`, func() {})
					switch where {
					case "before":
						sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) { return ctx, godog.ErrSkip })
					case "after":
						sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
							return ctx, godog.ErrSkip
						})
					case "after step":
						sc.StepContext().After(func(ctx context.Context, _ *godog.Step, _ godog.StepResultStatus, _ error) (context.Context, error) {
							return ctx, godog.ErrSkip
						})
					}
				})
				if code == 0 {
					t.Errorf("%s hook skipped engine evidence without failing", where)
				}
			})
		}
	})

	t.Run("nonempty engine set cannot disappear", func(t *testing.T) {
		dir := writeFeatureFixture(t, tierFixture)
		selected, err := selectScenarios("", dir, allowed)
		if err != nil {
			t.Fatal(err)
		}
		selected.filter = "@missing-sentinel"
		result := runScenarios(selected, godog.Options{Paths: []string{dir}, Format: "progress", Output: io.Discard}, func(*godog.ScenarioContext) {})
		if result == 0 {
			t.Fatal("lost engine sentinel passed as an empty bootstrap")
		}
		selected, err = selectScenarios("^@ID-MEMORY-01$", dir, allowed)
		if err != nil {
			t.Fatal(err)
		}
		if selected.skip == "" || strings.Contains(selected.skip, "no live @engine") || len(selected.excluded) != 1 {
			t.Errorf("filtered nonempty engine tier misreported as bootstrap: %+v", selected)
		}
	})
	t.Run("no engine tags bootstrap", func(t *testing.T) {
		dir := writeFeatureFixture(t, "Feature: ordinary\n  Scenario: live\n    Then record \"ordinary\"\n")
		selected, err := selectScenarios("", dir, allowed)
		if err != nil {
			t.Fatal(err)
		}
		if selected.skip != "no live @engine scenarios (count 0)" || selected.count != 0 {
			t.Errorf("bad bootstrap: %+v", selected)
		}
		if seen := executeMemory(t, dir, selected, nil); seen.initialized != 0 {
			t.Fatal("bootstrap ran hooks")
		}
	})
	t.Run("real harness skips before build", testHarnessEarlyExit)

}

func writeFeatureFixture(t *testing.T, text string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "memory.feature"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

type memoryExecution struct {
	steps, ids                                   []string
	initialized, before, after, background, code int
}

func executeMemory(t *testing.T, dir string, selected scenarioSelection, stepError error) memoryExecution {
	t.Helper()
	var seen memoryExecution
	seen.code = runScenarios(selected, godog.Options{Paths: []string{dir}, Format: "progress", Output: io.Discard, Strict: true}, func(sc *godog.ScenarioContext) {
		seen.initialized++
		sc.Before(func(ctx context.Context, scenario *godog.Scenario) (context.Context, error) {
			seen.before++
			for _, tag := range scenario.Tags {
				if strings.HasPrefix(tag.Name, "@ID-") {
					seen.ids = append(seen.ids, tag.Name)
				}
			}
			return ctx, nil
		})
		sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
			seen.after++
			return ctx, nil
		})
		sc.Step(`^a memory background$`, func() { seen.background++ })
		sc.Step(`^record "([^"]*)"$`, func(value string) error {
			seen.steps = append(seen.steps, value)
			return stepError
		})
	})
	if stepError == nil && seen.code != 0 {
		t.Fatalf("memory suite failed: %+v", seen)
	}
	return seen
}

// Different files and arbitrary names ensure this is tag inheritance, not a
// filename convention. Examples tags apply only to their own compiled rows.
func testInheritedEngineTags(t *testing.T) {
	fixtures := map[string]string{
		"feature.feature": `@engine @ID-MEMORY-10
Feature: inherited feature
  Background:
    Given a memory background
  Scenario: inherited
    Then record "feature"
`,
		"rule.feature": `Feature: inherited rule
  @engine @ID-MEMORY-11
  Rule: engine rule
    Background:
      Given a memory background
    Scenario: inherited
      Then record "rule"
`,
		"outline.feature": `Feature: inherited outline
  Background:
    Given a memory background
  @engine @ID-MEMORY-12
  Scenario Outline: engine rows
    Then record "<row>"
    Examples: live
      | row |
      | outline-a |
      | outline-b |
    @wip
    Examples: future
      | row |
      | outline-wip |
`,
		"examples.feature": `Feature: inherited examples
  Background:
    Given a memory background
  @ID-MEMORY-13
  Scenario Outline: mixed rows
    Then record "<row>"
    Examples: ordinary
      | row |
      | ordinary-row |
    @engine
    Examples: runtime
      | row |
      | example-a |
      | example-b |
    @engine @wip
    Examples: future
      | row |
      | example-wip |
`,
		"wip.feature": `@wip @ID-MEMORY-14
Feature: future inherited
  @engine
  Scenario: engine below wip
    Then record "feature-wip"
`,
		"wip-rule.feature": `Feature: future rule
  @wip @ID-MEMORY-15
  Rule: future
    @engine
    Scenario: engine below wip rule
      Then record "rule-wip"
`,
	}
	dir := t.TempDir()
	for name, text := range fixtures {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		engine, pattern string
		want, ids       []string
		excluded        int
	}{
		{"", "", []string{"ordinary-row"}, []string{"@ID-MEMORY-13"}, 10},
		{"1", "", []string{"example-a", "example-b", "feature", "outline-a", "outline-b", "rule"}, []string{"@ID-MEMORY-13", "@ID-MEMORY-13", "@ID-MEMORY-10", "@ID-MEMORY-12", "@ID-MEMORY-12", "@ID-MEMORY-11"}, 5},
		{"1", "^@ID-MEMORY-12$|^@wip$", []string{"outline-a", "outline-b"}, []string{"@ID-MEMORY-12", "@ID-MEMORY-12"}, 4},
		{"1", "^@ID-MEMORY-10$|^@ID-MEMORY-11$", []string{"feature", "rule"}, []string{"@ID-MEMORY-10", "@ID-MEMORY-11"}, 0},
	} {
		t.Run(tc.engine+tc.pattern, func(t *testing.T) {
			selected, err := selectScenarios(tc.pattern, dir, featureEnvironment{engine: tc.engine, ci: "true", actions: "true", os: "linux", arch: "amd64"})
			if err != nil {
				t.Fatal(err)
			}
			seen := executeMemory(t, dir, selected, nil)
			if !slices.Equal(seen.steps, tc.want) || !slices.Equal(seen.ids, tc.ids) {
				t.Errorf("executed %+v; want rows %v and IDs %v", seen, tc.want, tc.ids)
			}
			if seen.initialized != len(tc.want) || seen.before != len(tc.want) || seen.after != len(tc.want) || seen.background != len(tc.want) {
				t.Errorf("excluded hooks or backgrounds ran: %+v", seen)
			}
			if len(selected.excluded) != tc.excluded {
				t.Errorf("exclusions %v; want %d", selected.excluded, tc.excluded)
			}
		})
	}
}

func testEngineFeatureErrors(t *testing.T) {
	for _, text := range []string{
		"Feature: broken\n  Scenario: invalid\n    Given text\n      \"\"\"\n      unterminated\n",
		"@unused\nFeature: no scenarios\n",
	} {
		dir := writeFeatureFixture(t, text)
		if _, err := selectScenarios("^@unused$", dir, featureEnvironment{}); err == nil {
			t.Errorf("accepted malformed or no-scenario tag: %q", text)
		}
	}
	dir := writeFeatureFixture(t, "Feature: broken\n  Scenario: invalid\n    Given text\n      \"\"\"\n      unterminated\n")
	if _, err := selectScenarios("", dir, featureEnvironment{}); err == nil {
		t.Fatal("whole selection ignored malformed Gherkin")
	}
	if _, err := selectScenarios("", filepath.Join(t.TempDir(), "missing"), featureEnvironment{}); err == nil {
		t.Fatal("unreadable feature path accepted")
	}
	// The parent is a file, so this feature path cannot be read on any OS.
	unreadable := filepath.Join(dir, "memory.feature", "unreadable.feature")
	if _, err := selectScenarios("", unreadable, featureEnvironment{}); err == nil {
		t.Fatal("unreadable feature accepted")
	}

}

func testHarnessEarlyExit(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, mode, pattern, want string
		failure                   bool
	}{
		{"engine ID in ordinary tier", "", "^@ID-MEMORY-02$", "--- SKIP: TestFeatures", false},
		{"wip ID", "0", "^@ID-MEMORY-04$", "--- SKIP: TestFeatures", false},
		{"invalid switch", "yes", "", "ITOS_CONTAINER_ENGINE_TESTS must be", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(executable, "-test.run=^TestFeatures$", "-test.v", "-scenarios="+tc.pattern)
			cmd.Dir = writeFeatureFixture(t, tierFixture) // no go.mod, so a build attempt fails
			cmd.Env = append(cleanHarnessEnvironment(), "ITOS_CONTAINER_ENGINE_TESTS="+tc.mode,
				"DOCKER_HOST=unix:///unavailable/T-16.sock", "DOCKER_CONTEXT=T-16-unused", "GOCOVERDIR="+t.TempDir())
			output, err := cmd.CombinedOutput()
			if (err != nil) != tc.failure {
				t.Fatalf("exit %v, output %s", err, output)
			}
			if !strings.Contains(string(output), tc.want) {
				t.Fatalf("missing %q: %s", tc.want, output)
			}
			if !tc.failure && (!strings.Contains(string(output), "excluded 1 scenario(s)") || !strings.Contains(string(output), tc.pattern[1:len(tc.pattern)-1])) {
				t.Errorf("skip omitted excluded ID/count: %s", output)
			}
		})
	}
}

func cleanHarnessEnvironment() []string {
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if key != "ITOS_CONTAINER_ENGINE_TESTS" && key != "ITOS_CONTAINER_RED_SCENARIOS" && key != "ITOS_CONTAINER_ENGINE_FEATURES" && key != "DOCKER_HOST" && key != "DOCKER_CONTEXT" && key != "GOCOVERDIR" {
			env = append(env, entry)
		}
	}
	return env
}

const redVariable = "ITOS_CONTAINER_RED_SCENARIOS"

func redEnvironment(engine, hostOS, ids string) featureEnvironment {
	return featureEnvironment{engine: engine, ci: "true", actions: "true", event: "workflow_dispatch", red: ids, os: hostOS, arch: "amd64"}
}

func TestEngineRedSelection(t *testing.T) {
	for _, tc := range []struct {
		name, engine, hostOS, ids string
		want, steps               []string
	}{
		{"ordinary named live", "", "darwin", "@ID-MEMORY-01", []string{"@ID-MEMORY-01"}, []string{"ordinary"}},
		{"engine named live", "1", "linux", "@ID-MEMORY-02", []string{"@ID-MEMORY-02"}, []string{"engine"}},
		{"ordinary future", "", "darwin", "@ID-MEMORY-03", []string{"@ID-MEMORY-03"}, []string{"wip"}},
		{"engine future", "1", "linux", "@ID-MEMORY-04", []string{"@ID-MEMORY-04"}, []string{"engine-wip"}},
		{"mixed live future duplicates", "0", "windows", " @ID-MEMORY-03\t@ID-MEMORY-01\n@ID-MEMORY-03\u2003", []string{"@ID-MEMORY-01", "@ID-MEMORY-03"}, []string{"ordinary", "wip"}},
		{"engine live future", "1", "linux", "@ID-MEMORY-02 @ID-MEMORY-04", []string{"@ID-MEMORY-02", "@ID-MEMORY-04"}, []string{"engine", "engine-wip"}},
		{"shared list ordinary", "", "darwin", "@ID-MEMORY-03 @ID-MEMORY-04", []string{"@ID-MEMORY-03"}, []string{"wip"}},
		{"shared list engine", "1", "linux", "@ID-MEMORY-03 @ID-MEMORY-04", []string{"@ID-MEMORY-04"}, []string{"engine-wip"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := tierFixture + "\n  @ID-MEMORY-05\n  Scenario: unrelated live\n    Then record \"unrequested\"\n" +
				"\n  @engine @ID-MEMORY-06\n  Scenario: unrelated engine\n    Then record \"unrequested-engine\"\n"
			dir := writeFeatureFixture(t, fixture)
			selected, err := selectScenarios("", dir, redEnvironment(tc.engine, tc.hostOS, tc.ids))
			if err != nil {
				t.Fatal(err)
			}
			seen := executeMemory(t, dir, selected, nil)
			if !slices.Equal(seen.ids, tc.want) || !slices.Equal(seen.steps, tc.steps) {
				t.Errorf("executed IDs %v and steps %v; want %v and %v", seen.ids, seen.steps, tc.want, tc.steps)
			}
			if selected.count != len(tc.want) || seen.initialized != len(tc.want) || seen.before != len(tc.want) || seen.after != len(tc.want) || seen.background != len(tc.want) {
				t.Errorf("unrequested or missing initializer/hook/background: selection %+v, execution %+v", selected, seen)
			}
			data, err := os.ReadFile(filepath.Join(dir, "memory.feature"))
			if err != nil || string(data) != fixture {
				t.Fatalf("selection rewrote feature: %v", err)
			}
		})
	}
	t.Run("exact IDs do not match prefixes", func(t *testing.T) {
		dir := writeFeatureFixture(t, tierFixture+"\n  @wip @ID-MEMORY-030\n  Scenario: overlapping ID\n    Then record \"unrequested\"\n")
		selected, err := selectScenarios("", dir, redEnvironment("", "darwin", "@ID-MEMORY-03"))
		if err != nil {
			t.Fatal(err)
		}
		if seen := executeMemory(t, dir, selected, nil); !slices.Equal(seen.ids, []string{"@ID-MEMORY-03"}) {
			t.Errorf("executed IDs %v", seen.ids)
		}
	})
	t.Run("inherited tags and all outline rows", testRedInheritance)
	t.Run("reject invalid lists before initialization", func(t *testing.T) {
		dir := writeFeatureFixture(t, tierFixture)
		for _, value := range []string{"ID-MEMORY-03", "@wip", "@ID-memory-03", "@ID-MEMORY-03,@ID-MEMORY-04", "^@ID-MEMORY-03$", "@ID-MEMORY-03|@ID-MEMORY-04", "@ID-MEMORY-03&&@wip", "$(id)", "@ID-MEMORY-03;true", "@ID-MEMORY-03 @ID-MEMORY-99", "@ID-MEMORY-99"} {
			t.Run(value, func(t *testing.T) {
				_, err := selectScenarios("", dir, redEnvironment("", "darwin", value))
				if err == nil || !strings.Contains(err.Error(), "red") {
					t.Errorf("red list %q should fail with a selection diagnostic, got %v", value, err)
				}
			})
		}
		if _, err := selectScenarios("^@ID-MEMORY-01$", dir, redEnvironment("", "darwin", "@ID-MEMORY-03")); err == nil || !strings.Contains(err.Error(), "-scenarios") {
			t.Errorf("conflicting selectors accepted: %v", err)
		}
		for _, text := range []string{"Feature: broken\n  Scenario: bad\n    Given text\n      \"\"\"\n      open\n", "@ID-MEMORY-03\nFeature: no pickles\n"} {
			if _, err := selectScenarios("", writeFeatureFixture(t, text), redEnvironment("", "darwin", "@ID-MEMORY-03")); err == nil {
				t.Error("malformed or empty inventory accepted")
			}
		}
	})
	t.Run("empty input preserves live selection", func(t *testing.T) {
		for _, value := range []string{"", " \t\n\u2003"} {
			for _, engine := range []string{"", "1"} {
				dir := writeFeatureFixture(t, tierFixture)
				selected, err := selectScenarios("", dir, redEnvironment(engine, "linux", value))
				if err != nil {
					t.Fatal(err)
				}
				want := "@ID-MEMORY-01"
				if engine == "1" {
					want = "@ID-MEMORY-02"
				}
				if seen := executeMemory(t, dir, selected, nil); !slices.Equal(seen.ids, []string{want}) {
					t.Errorf("empty red input executed %v", seen.ids)
				}
			}
		}
	})
	t.Run("cross tier and platform skips", func(t *testing.T) {
		for _, tc := range []struct{ engine, hostOS, id, reason string }{
			{"", "darwin", "@ID-MEMORY-04", "@engine disabled"},
			{"1", "linux", "@ID-MEMORY-03", "other tier"},
			{"", "windows", "@ID-MEMORY-05", "@linux requires Linux"},
		} {
			dir := writeFeatureFixture(t, tierFixture+"\n  @wip @linux @ID-MEMORY-05\n  Scenario: Linux only\n    Then record \"linux\"\n")
			selected, err := selectScenarios("", dir, redEnvironment(tc.engine, tc.hostOS, tc.id))
			if err != nil {
				t.Fatal(err)
			}
			if selected.skip == "" || !strings.Contains(selected.skip, "count 0") || len(selected.excluded) != 1 || !strings.Contains(strings.Join(selected.excluded, ";"), tc.id) || !strings.Contains(strings.Join(selected.excluded, ";"), tc.reason) {
				t.Errorf("missing exclusion ID/reason/count: %+v", selected)
			}
			if seen := executeMemory(t, dir, selected, nil); seen.initialized != 0 {
				t.Errorf("excluded list ran scenarios: %+v", seen)
			}
		}
	})
	t.Run("invalid engine contexts remain refused", func(t *testing.T) {
		base := redEnvironment("1", "linux", "@ID-MEMORY-04")
		for _, mutate := range []func(*featureEnvironment){
			func(e *featureEnvironment) { e.ci = "" }, func(e *featureEnvironment) { e.actions = "" },
			func(e *featureEnvironment) { e.os = "darwin" }, func(e *featureEnvironment) { e.arch = "arm64" },
			func(e *featureEnvironment) { e.engine = "true" }, func(e *featureEnvironment) { e.engine = " 1" },
		} {
			env := base
			mutate(&env)
			if _, err := selectScenarios("", writeFeatureFixture(t, tierFixture), env); err == nil {
				t.Errorf("accepted engine context %+v", env)
			}
		}
	})
	t.Run("guarded environment boundary and coverage reruns", testRedEnvironmentBoundary)
	t.Run("incomplete execution cannot pass", testRedCompletion)
	t.Run("child and synthetic environment isolation", func(t *testing.T) {
		// This inert input never grants CI or engine context to this process.
		t.Setenv(redVariable, "@ID-MEMORY-03")
		t.Cleanup(removeCallerPath)
		w := world{}
		for name, env := range map[string][]string{"product": w.env(), "synthetic harness": cleanHarnessEnvironment()} {
			for _, entry := range env {
				if strings.HasPrefix(entry, redVariable+"=") {
					t.Errorf("%s inherited red selector", name)
				}
			}
		}
	})
}
