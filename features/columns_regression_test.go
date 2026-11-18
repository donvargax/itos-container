package features

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// An inherited COLUMNS makes Kong wrap help to the caller's terminal width, which
// splits the phrases help scenarios assert. The scenarios' environment must not
// carry the caller's width, while a scenario that names its own still gets it.
// It runs on filtered runs too: it selects its own scenario.
func TestHarnessEnvClearsColumns(t *testing.T) {
	isolateProductCoverage(t)
	t.Cleanup(removeCallerPath)
	t.Setenv("COLUMNS", "30")

	for _, kv := range (&world{}).env() {
		if name, _, _ := strings.Cut(kv, "="); strings.EqualFold(name, "COLUMNS") {
			t.Errorf("the scenarios' environment inherits %q", kv)
		}
	}
	explicit := 0
	for _, kv := range (&world{}).withEnv("COLUMNS=200") {
		if name, value, _ := strings.Cut(kv, "="); strings.EqualFold(name, "COLUMNS") {
			explicit++
			if value != "200" {
				t.Errorf("an explicit width is overridden: %q", kv)
			}
		}
	}
	if explicit != 1 {
		t.Errorf("an explicit COLUMNS=200 appears %d times, want 1", explicit)
	}

	// Black box: a help scenario passes under the narrow inherited width.
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	bin, err := build(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	selection, err := selectScenarios("^@ID-APPROVAL-29$", ".", featureEnvironment{})
	if err != nil {
		t.Fatal(err)
	}
	if selection.count != 1 {
		t.Fatalf("selected %d scenarios for ID-APPROVAL-29, want 1", selection.count)
	}
	var output bytes.Buffer
	if runScenarios(selection, godog.Options{
		Format: "progress", Output: &output, Paths: []string{"."},
	}, func(sc *godog.ScenarioContext) { initializeScenario(sc, root, bin) }) != 0 {
		t.Errorf("ID-APPROVAL-29 fails under an inherited COLUMNS=30: %s", output.String())
	}
}
