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

func testRedEnvironmentBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, ci, actions, event string
		allowed                  bool
	}{
		{"local", "", "", "", false},
		{"missing CI", "", "true", "workflow_dispatch", false},
		{"missing Actions", "true", "", "workflow_dispatch", false},
		{"inexact CI", "1", "true", "workflow_dispatch", false},
		{"inexact Actions", "true", "True", "workflow_dispatch", false},
		{"PR", "true", "true", "pull_request", false},
		{"push", "true", "true", "push", false},
		{"other event", "true", "true", "schedule", false},
		{"missing event", "true", "true", "", false},
		{"dispatch", "true", "true", "workflow_dispatch", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, value := range []string{"", " \n\t", "@ID-MEMORY-03", "$(hostile)"} {
				values := map[string]string{"CI": tc.ci, "GITHUB_ACTIONS": tc.actions, "GITHUB_EVENT_NAME": tc.event, redVariable: value}
				var reads []string
				env := readFeatureEnvironment(func(key string) string { reads = append(reads, key); return values[key] }, "darwin", "amd64")
				if slices.Contains(reads, redVariable) != tc.allowed {
					t.Errorf("red lookup allowed=%v, reads=%v", tc.allowed, reads)
				}
				if tc.allowed {
					if env.red != value {
						t.Errorf("boundary changed red input %q to %q", value, env.red)
					}
					for _, key := range []string{"CI", "GITHUB_ACTIONS", "GITHUB_EVENT_NAME"} {
						if slices.Index(reads, key) >= slices.Index(reads, redVariable) {
							t.Errorf("red lookup preceded %s: %v", key, reads)
						}
					}
				}
				want := tc.allowed && strings.TrimSpace(value) != ""
				if filteredFeatureRun("", env) != want {
					t.Errorf("coverage rerun exclusion for %q = %v, want %v", value, filteredFeatureRun("", env), want)
				}
				if !filteredFeatureRun("^@ID-MEMORY-01$", env) {
					t.Error("ordinary filtered run lost coverage exclusion")
				}
				if tc.allowed {
					continue
				}
				// Even a directly supplied hostile value cannot activate policy outside dispatch.
				env.red = value
				if filteredFeatureRun("", env) {
					t.Error("ignored input skipped ordinary coverage")
				}
				dir := writeFeatureFixture(t, tierFixture)
				selected, err := selectScenarios("", dir, env)
				if err != nil {
					t.Fatal(err)
				}
				if seen := executeMemory(t, dir, selected, nil); !slices.Equal(seen.ids, []string{"@ID-MEMORY-01"}) {
					t.Errorf("ignored input ran %v", seen.ids)
				}
			}
		})
	}
}

func testRedInheritance(t *testing.T) {
	fixtures := []string{
		"@wip @ID-MEMORY-10\nFeature: feature inheritance\n  Background:\n    Given a memory background\n  Scenario: inherited\n    Then record \"feature\"\n",
		"Feature: rule inheritance\n  @wip @ID-MEMORY-11\n  Rule: future\n    Background:\n      Given a memory background\n    Scenario: inherited\n      Then record \"rule\"\n",
		`Feature: outline inheritance
  Background:
    Given a memory background
  @wip @ID-MEMORY-12
  Scenario Outline: rows
    Then record "<row>"
    Examples: portable
      | row |
      | outline-a |
      | outline-b |
    @linux
    Examples: Linux only
      | row |
      | linux |
    @engine
    Examples: engine only
      | row |
      | engine |
`,
		`Feature: examples inheritance
  Background:
    Given a memory background
  Scenario Outline: rows
    Then record "<row>"
    @wip @ID-MEMORY-13
    Examples: selected
      | row |
      | example-a |
      | example-b |
    @ID-MEMORY-14
    Examples: unrelated live
      | row |
      | unrequested |
`,
	}
	for i, text := range fixtures {
		id := []string{"@ID-MEMORY-10", "@ID-MEMORY-11", "@ID-MEMORY-12", "@ID-MEMORY-13"}[i]
		want := [][]string{{"feature"}, {"rule"}, {"outline-a", "outline-b"}, {"example-a", "example-b"}}[i]
		dir := writeFeatureFixture(t, text)
		selected, err := selectScenarios("", dir, redEnvironment("", "darwin", id))
		if err != nil {
			t.Fatal(err)
		}
		seen := executeMemory(t, dir, selected, nil)
		if !slices.Equal(seen.steps, want) || seen.initialized != len(want) || seen.before != len(want) || seen.after != len(want) || seen.background != len(want) {
			t.Errorf("inherited %s: %+v, want %v", id, seen, want)
		}
		if len(seen.ids) != len(want) {
			t.Errorf("inherited IDs %v", seen.ids)
		}
		for _, got := range seen.ids {
			if got != id {
				t.Errorf("unrequested inherited ID %s", got)
			}
		}
	}
}

func testRedCompletion(t *testing.T) {
	for _, engine := range []string{"", "1"} {
		for _, mode := range []string{"failed", "pending", "skipped", "undefined", "empty", "before skip", "after skip", "step hook skip", "lost"} {
			t.Run(engine+mode, func(t *testing.T) {
				text := "@wip @ID-MEMORY-20\nFeature: red completion\n  Scenario: selected\n    Then a selected step\n"
				if engine == "1" {
					text = "@engine " + text
				}
				if mode == "empty" {
					text = strings.ReplaceAll(text, "    Then a selected step\n", "")
				}
				dir := writeFeatureFixture(t, text)
				selected, err := selectScenarios("", dir, redEnvironment(engine, "linux", "@ID-MEMORY-20"))
				if err != nil {
					t.Fatal(err)
				}
				if mode == "lost" {
					selected.filter = "@absent"
				}
				code := runScenarios(selected, godog.Options{Paths: []string{dir}, Format: "progress", Output: io.Discard}, func(sc *godog.ScenarioContext) {
					if mode != "undefined" {
						sc.Step(`^a selected step$`, func() error {
							switch mode {
							case "failed":
								return errors.New("assertion failed")
							case "pending":
								return godog.ErrPending
							case "skipped":
								return godog.ErrSkip
							}
							return nil
						})
					}
					switch mode {
					case "before skip":
						sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) { return ctx, godog.ErrSkip })
					case "after skip":
						sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
							return ctx, godog.ErrSkip
						})
					case "step hook skip":
						sc.StepContext().After(func(ctx context.Context, _ *godog.Step, _ godog.StepResultStatus, _ error) (context.Context, error) {
							return ctx, godog.ErrSkip
						})
					}
				})
				if code == 0 {
					t.Errorf("%s selected execution passed", mode)
				}
			})
		}
	}
}
