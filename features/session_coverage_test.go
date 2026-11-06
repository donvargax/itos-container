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

// Listed-ID proof needs both the subprocess observations and executed product
// coverage, including fresh-controller cleanup and restrictive child umasks.
func TestSessionContractCoverage(t *testing.T) {
	env := readFeatureEnvironment(os.Getenv, runtime.GOOS, runtime.GOARCH)
	if filteredFeatureRun(*scenarios, env) || env.os != "linux" || env.engine == "1" {
		t.Skip("whole-suite Linux fixture regression; preserve filtered and engine selections")
	}
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(removeCallerPath)
	// Both IDs share the compiled assembly, but must export their own executed
	// counters into the current ID's directory when the subprocess runs.
	bin := filepath.Join(t.TempDir(), "itos-container")
	// An earlier plain run must not supply an uninstrumented executable to a
	// later coverage request using the same suite path.
	t.Setenv("GOCOVERDIR", "")
	plain := &world{root: root, bin: bin, dir: t.TempDir()}
	if _, err := plain.sessionProbeBinary(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"ID-SESSION-01", "ID-SESSION-08"} {
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
				t.Fatalf("session contract scenarios failed: %s", output.String())
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
				if len(fields) == 3 && strings.Contains(fields[0], "/internal/engine/session/domain/session.go:") && fields[2] != "0" {
					return
				}
			}
			t.Errorf("%s produced no per-ID executed coverage for the session domain", id)
		})
	}
}
