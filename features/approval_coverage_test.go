package features

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// Contract-only IDs must export the product decisions they execute, including
// refusal paths. Coverage of the feature runner cannot supply their reach.
func TestApprovalContractCoverage(t *testing.T) {
	if filteredFeatureRun(*scenarios, readFeatureEnvironment(os.Getenv, runtime.GOOS, runtime.GOARCH)) {
		t.Skip("whole-suite harness regression; filtered runs must execute only their selected scenarios")
	}
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(removeCallerPath)
	bin := filepath.Join(t.TempDir(), "itos-container")
	for _, id := range []string{"ID-APPROVAL-06", "ID-APPROVAL-31", "ID-APPROVAL-22", "ID-APPROVAL-23", "ID-APPROVAL-24"} {
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
				t.Fatalf("approval contract scenarios failed: %s", output.String())
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
			for line := range strings.SplitSeq(string(data), "\n") {
				fields := strings.Fields(line)
				if len(fields) == 3 && strings.Contains(fields[0], "/internal/engine/approval/domain/approval.go:") && fields[2] != "0" {
					return
				}
			}
			t.Errorf("%s produced no per-ID executed coverage for the approval domain", id)
		})
	}
}
