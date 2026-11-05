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

// Exercise per-ID product counters for success, digest refusal and normalized
// identity. These use Unix HTTP fixtures and never require a Docker daemon.
func TestDoctorListedCoverage(t *testing.T) {
	env := readFeatureEnvironment(os.Getenv, runtime.GOOS, runtime.GOARCH)
	if filteredFeatureRun(*scenarios, env) || env.os != "linux" || env.arch != "amd64" || env.engine == "1" {
		t.Skip("whole-suite Linux fixture regression; preserve filtered and engine selections")
	}
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(removeCallerPath)
	t.Setenv("GOCOVERDIR", t.TempDir())
	bin, err := build(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"ID-DOCTOR-01", "ID-DOCTOR-16", "ID-DOCTOR-30"} {
		t.Run(id, func(t *testing.T) {
			coverdir := t.TempDir()
			t.Setenv("GOCOVERDIR", coverdir)
			selection, err := selectScenarios("^@"+id+"$", ".", env)
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if selection.count == 0 || runScenarios(selection, godog.Options{
				Format: "progress", Output: &output, Paths: []string{"."}, TestingT: t,
			}, func(sc *godog.ScenarioContext) { initializeScenario(sc, root, bin) }) != 0 {
				t.Fatalf("doctor scenarios failed: %s", output.String())
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
			for _, source := range []string{"readiness/domain/image.go", "readiness/reference/digests.go", "readiness/docker/engine.go"} {
				covered := false
				for line := range strings.SplitSeq(string(data), "\n") {
					fields := strings.Fields(line)
					if len(fields) == 3 && strings.Contains(fields[0], "/internal/engine/"+source+":") && fields[2] != "0" {
						covered = true
					}
				}
				if !covered {
					t.Errorf("%s produced no per-ID product coverage for %s", id, source)
				}
			}
		})
	}
}
