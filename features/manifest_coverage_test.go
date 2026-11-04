package features

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// Each metadata ID must export its own product-code counters, including refusal
// exits. A passing scenario or coverage of the feature runner is not enough.
func TestManifestContractCoverage(t *testing.T) {
	if *scenarios != "" {
		t.Skip("whole-suite harness regression; filtered runs must execute only their selected scenarios")
	}
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(removeCallerPath)
	// These scenarios use only the test assembly and sentinels, cached beside
	// this binary path just as they are during TestFeatures.
	bin := filepath.Join(t.TempDir(), "itos-container")
	for _, id := range []string{"ID-MANIFEST-29", "ID-MANIFEST-20", "ID-MANIFEST-21", "ID-MANIFEST-30"} {
		t.Run(id, func(t *testing.T) {
			coverdir := t.TempDir()
			t.Setenv("GOCOVERDIR", coverdir)
			selection, err := selectScenarios("^@"+id+"$", ".", featureEnvironment{})
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if selection.count == 0 || runScenarios(selection, godog.Options{
				Format: "progress", Output: &output, Paths: []string{"."}, TestingT: t,
			}, func(sc *godog.ScenarioContext) { initializeScenario(sc, root, bin) }) != 0 {
				t.Fatalf("metadata scenarios failed: %s", output.String())
			}
			profile := filepath.Join(t.TempDir(), "coverage.out")
			cmd := exec.Command("go", "tool", "covdata", "textfmt", "-i="+coverdir, "-o="+profile)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("read per-ID coverage: %v\n%s", err, out)
			}
			data, err := os.ReadFile(profile)
			if err != nil {
				t.Fatal(err)
			}
			for _, source := range []string{
				"internal/engine/manifest/domain/metadata.go",
				"internal/engine/manifest/jsonc/parser.go",
				"internal/engine/manifest/reference/reference.go",
				"internal/slices/doctor/command/manifest.go",
			} {
				covered := false
				for line := range strings.SplitSeq(string(data), "\n") {
					fields := strings.Fields(line)
					if len(fields) == 3 && strings.Contains(fields[0], "/"+source+":") && fields[2] != "0" {
						covered = true
					}
				}
				if !covered {
					t.Errorf("%s produced no per-ID executed coverage for %s", id, source)
				}
			}
		})
	}
}
