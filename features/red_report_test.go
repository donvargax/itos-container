package features

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"go.yaml.in/yaml/v3"
)

func TestRedScenarioReport(t *testing.T) {
	for _, tc := range []struct {
		name, ci, actions, event, red string
		wantReport                    bool
	}{
		{"red dispatch", "true", "true", "workflow_dispatch", "@ID-REPORT-01", true},
		{"empty dispatch", "true", "true", "workflow_dispatch", "", false},
		{"whitespace dispatch", "true", "true", "workflow_dispatch", " \t", false},
		{"pull request", "true", "true", "pull_request", "@ID-REPORT-01", false},
		{"push", "true", "true", "push", "@ID-REPORT-01", false},
		{"no CI", "", "true", "workflow_dispatch", "@ID-REPORT-01", false},
		{"no Actions", "true", "", "workflow_dispatch", "@ID-REPORT-01", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "results.json")
			feature := `Feature: Report results
  @ID-REPORT-01
  Scenario: A failing assertion
    Then the assertion fails
`
			if err := os.WriteFile(filepath.Join(dir, "report.feature"), []byte(feature), 0600); err != nil {
				t.Fatal(err)
			}
			values := map[string]string{
				"CI": tc.ci, "GITHUB_ACTIONS": tc.actions, "GITHUB_EVENT_NAME": tc.event,
				redVariable: tc.red, redReportVariable: path,
			}
			env := readFeatureEnvironment(func(key string) string { return values[key] }, "linux", "amd64")
			selection, err := selectScenarios("", dir, env)
			if err != nil {
				t.Fatal(err)
			}
			var pretty bytes.Buffer
			options := godog.Options{Format: env.reportFormat(), Output: &pretty, Paths: []string{dir}, NoColors: true}
			if runFeatureScenarios(selection, options, func(sc *godog.ScenarioContext, _ func()) {
				sc.Step("^the assertion fails$", func() error { return fmt.Errorf("fixture assertion failed") })
			}) == 0 {
				t.Fatal("the failing scenario passed")
			}
			if !strings.Contains(pretty.String(), "Then the assertion fails") {
				t.Errorf("pretty output lost the step: %q", pretty.String())
			}
			data, err := os.ReadFile(path)
			if !tc.wantReport {
				if !os.IsNotExist(err) {
					t.Fatalf("expected no report, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !json.Valid(data) || !bytes.Contains(data, []byte(`"status": "failed"`)) ||
				!bytes.Contains(data, []byte(`"error_message": "fixture assertion failed"`)) {
				t.Fatalf("expected Cucumber JSON with the failed step and error, got %s", data)
			}
		})
	}
}

const redReportPath = "${{ runner.temp }}/red-scenarios.json"
const redUploadStep = "Upload red scenario report"
const redUploadGuard = "${{ always() && github.event_name == 'workflow_dispatch' && inputs.red_scenarios != '' }}"
const redUploadAction = "actions/upload-artifact@cf430e030ddbb5b0abf93d22962f4752f3646cd9"

func engineRedWorkflowIssues(data []byte) []string {
	var w engineWorkflow
	if err := yaml.Unmarshal(data, &w); err != nil {
		return []string{err.Error()}
	}
	var issues []string
	require := func(ok bool, issue string) {
		if !ok {
			issues = append(issues, issue)
		}
	}
	require(validRedDispatch(w.On["workflow_dispatch"]), "exact optional red dispatch inputs")
	allowed := map[string]string{}
	for job, name := range map[string]string{"engine-shard": "Engine scenarios", "platform-macos": "Tests", "platform-windows": "Tests"} {
		j := w.Jobs[job]
		require(!j.ContinueOnError, "job failure propagation for "+job)
		found, uploads := 0, 0
		run, artifact := "go test ./... -count=1 -timeout=20m", "red-scenarios-"+job
		if job == "engine-shard" {
			run = engineInvocation
			artifact += "-${{ matrix.shard }}"
		}
		for i, step := range j.Steps {
			path := "jobs." + job + ".steps." + strconv.Itoa(i)
			switch step.Name {
			case name:
				found++
				allowed[path+".env."+redVariable] = redDispatchExpression
				allowed[path+".env."+redReportVariable] = redReportPath
				require(step.Env[redVariable] == redDispatchExpression, "step-local red selection for "+job)
				require(step.Env[redReportVariable] == redReportPath, "step-local red report path for "+job)
				require(step.Run == run, "unchanged test command for "+job)
				require(!step.ContinueOnError && step.If == "", "step failure propagation for "+job)
			case redUploadStep:
				uploads++
				allowed[path+".if"] = redUploadGuard
				allowed[path+".uses"] = redUploadAction
				require(found == 1 && step.If == redUploadGuard, "red report upload guard and order for "+job)
				require(step.Uses == redUploadAction, "red report upload pin for "+job)
				require(maps.Equal(step.With, map[string]string{
					"name": artifact, "path": redReportPath, "if-no-files-found": "ignore",
				}), "red report upload inputs for "+job)
			}
		}
		require(found == 1, "exactly one selected test step for "+job)
		require(uploads == 1, "exactly one red report upload for "+job)
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return append(issues, err.Error())
	}
	walkFixtureScalars(&node, "", func(path, value string) {
		if strings.HasPrefix(path, "on.workflow_dispatch.inputs.red_scenarios.") {
			return
		}
		if strings.Contains(path, redVariable) || strings.Contains(value, redVariable) || strings.Contains(value, "red_scenarios") {
			require(allowed[path] == value && value != "", "leaked red selection at "+path)
		}
		if strings.Contains(path, redReportVariable) || strings.Contains(value, redReportVariable) || strings.HasPrefix(value, "actions/upload-artifact@") {
			require(allowed[path] == value && value != "", "leaked red report wiring at "+path)
		}
	})
	return issues
}
