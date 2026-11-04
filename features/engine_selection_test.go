package features

import (
	"context"
	"io"
	"os"
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
