package features

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/cucumber/godog"
)

func TestHarnessFailureFailsAtItsStep(t *testing.T) {
	const fixture = `Feature: Step failure attribution
  Scenario: Missing barrier
    When the fixture cannot reach the barrier
    Then the barrier was reached

  Scenario: Product refusal
    When the process exits with a refusal
    Then the refusal is reported
`
	barrierErr := errors.New("fixture never reached the barrier")
	var output bytes.Buffer
	suite := godog.TestSuite{
		Options: &godog.Options{
			Format:          "cucumber",
			Output:          &output,
			FeatureContents: []godog.Feature{{Name: "step-failure.feature", Contents: []byte(fixture)}},
		},
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			var exit int
			var diagnostic string
			sc.When(`^the fixture cannot reach the barrier$`, func() error { return barrierErr })
			sc.Then(`^the barrier was reached$`, func() error { return errors.New("barrier assertion must be skipped") })
			sc.When(`^the process exits with a refusal$`, func() {
				exit, diagnostic = 75, "run.fixture-refused"
			})
			sc.Then(`^the refusal is reported$`, func() error {
				if exit != 75 || diagnostic != "run.fixture-refused" {
					return fmt.Errorf("want refusal; exit=%d, diagnostic=%q", exit, diagnostic)
				}
				return nil
			})
		},
	}
	if suite.Run() != 1 {
		t.Fatalf("want a failed suite for the missing barrier, got report:\n%s", &output)
	}

	var report []struct {
		Elements []struct {
			Name  string
			Steps []struct {
				Name   string
				Result struct {
					Status string
					Error  string `json:"error_message"`
				}
			}
		}
	}
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("decode cucumber report: %v\n%s", err, &output)
	}
	want := map[string][2]string{
		"the fixture cannot reach the barrier": {"failed", barrierErr.Error()},
		"the barrier was reached":              {"skipped", ""},
		"the process exits with a refusal":     {"passed", ""},
		"the refusal is reported":              {"passed", ""},
	}
	for _, feature := range report {
		for _, scenario := range feature.Elements {
			for _, step := range scenario.Steps {
				expected, ok := want[step.Name]
				if got := [2]string{step.Result.Status, step.Result.Error}; !ok || got != expected {
					t.Errorf("%s / %s: got %q, want %q", scenario.Name, step.Name, got, expected)
				}
				delete(want, step.Name)
			}
		}
	}
	if len(want) != 0 {
		t.Errorf("missing steps in cucumber report: %v", want)
	}
}
