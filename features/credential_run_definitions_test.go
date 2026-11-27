package features

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/cucumber/godog"
)

var credentialRunIDs = []string{"@ID-CREDRUN-08", "@ID-CREDRUN-09", "@ID-CREDRUN-10"}

func testCredentialRunRedInventory(t *testing.T, include []shardEntry) {
	t.Helper()
	for _, entry := range include {
		selection, err := selectScenarios("", ".", shardEnvironment(map[string]string{
			shardFeaturesVariable: entry.Features,
			"GITHUB_EVENT_NAME":   "workflow_dispatch",
			redVariable:           strings.Join(credentialRunIDs, " "),
		}))
		if err != nil {
			t.Fatal(err)
		}
		if entry.Shard == "egress" {
			if selection.count != 6 || selection.skip != "" || !slices.Equal(selection.ids, credentialRunIDs) {
				t.Errorf("egress must select six credential run rows and three IDs: count=%d ids=%v skip=%q", selection.count, selection.ids, selection.skip)
			}
		} else if selection.count != 0 || !strings.Contains(selection.skip, "other engine shard") {
			t.Errorf("shard %s must leave credential run rows to egress: count=%d skip=%q", entry.Shard, selection.count, selection.skip)
		}
	}
}

func TestCredentialRunNativeStepsDefined(t *testing.T) {
	const featurePath = "credentials-run.feature"
	var output strings.Builder
	options := &godog.Options{
		Paths: []string{featurePath}, Format: "progress", Output: &output, ShowStepDefinitions: true,
	}
	if _, err := os.Stat(featurePath); os.IsNotExist(err) {
		// The base-tree test projection contains the Go tests and the reviewed ledger,
		// but lacks the new feature file. The shard inventory still requires it on landing.
		options.FS = fstest.MapFS{featurePath: {Data: credentialRunLedgerFeature(t)}}
	} else if err != nil {
		t.Fatal(err)
	}
	suite := godog.TestSuite{
		Options:             options,
		ScenarioInitializer: func(sc *godog.ScenarioContext) { initializeScenario(sc, "unused-root", "unused-binary") },
	}
	if code := suite.Run(); code != 2 {
		t.Fatalf("definition listing exited %d", code)
	}
	ansi := regexp.MustCompile("\x1b\\[[0-9;]*m")
	var definitions []*regexp.Regexp
	for _, line := range strings.Split(ansi.ReplaceAllString(output.String(), ""), "\n") {
		expression, _, ok := strings.Cut(line, " # ")
		if ok {
			definitions = append(definitions, regexp.MustCompile(strings.TrimSpace(expression)))
		}
	}
	features, err := suite.RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	rows, steps := 0, 0
	for _, feature := range features {
		for _, scenario := range feature.Pickles {
			selected := false
			for _, tag := range scenario.Tags {
				selected = selected || slices.Contains(credentialRunIDs, tag.Name)
			}
			if !selected {
				continue
			}
			rows++
			for _, step := range scenario.Steps {
				matches := 0
				for _, definition := range definitions {
					if definition.MatchString(step.Text) {
						matches++
					}
				}
				if matches != 1 {
					t.Errorf("step %q has %d definitions", step.Text, matches)
				}
				steps++
			}
		}
	}
	if rows != 6 || steps != 69 {
		t.Fatalf("checked %d rows and %d steps, expected 6 rows and 69 steps", rows, steps)
	}
}

func credentialRunLedgerFeature(t *testing.T) []byte {
	t.Helper()
	ledger, err := os.ReadFile("../tasks/phase-1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	_, task, ok := strings.Cut(string(ledger), "\n- id: T-95\n")
	if !ok {
		t.Fatal("T-95 is missing from the task ledger")
	}
	task, _, _ = strings.Cut(task, "\n- id: ")
	const start = "\n      @phase-1\n"
	_, body, ok := strings.Cut(task, start)
	if !ok {
		t.Fatal("T-95's reviewed feature is missing from the task ledger")
	}
	var feature strings.Builder
	feature.WriteString("@phase-1\n")
	for line := range strings.SplitSeq(body, "\n") {
		content, indented := strings.CutPrefix(line, "      ")
		if !indented && line != "" {
			break
		}
		feature.WriteString(content)
		feature.WriteByte('\n')
	}
	return []byte(strings.TrimRight(feature.String(), "\n") + "\n")
}
