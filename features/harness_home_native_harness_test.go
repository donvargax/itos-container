package features

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

func TestHomeNativeFixtureGuard(t *testing.T) {
	if os.Getenv("ITOS_CONTAINER_ENGINE_TESTS") == "1" {
		t.Skip("guard refusal is checked in the ordinary non-engine tier")
	}
	w := &world{dir: t.TempDir()}
	if err := w.homeNativeFixture(); err == nil || w.doctor != nil || w.home != nil {
		t.Fatal("native home setup escaped its CI-only guard")
	}
	entries, err := os.ReadDir(w.dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("refused native home setup allocated fixture state", err)
	}
}

func TestHomeNativeProbeCompiles(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	// Compilation only. Native probes are never executed on this host.
	if err := recoveryBuildHelper(root, filepath.Join(t.TempDir(), "probe"), homeNativeProbeSource); err != nil {
		t.Fatal(err)
	}
}

func TestHomeNativePersistenceOracle(t *testing.T) {
	fixture := func() *world {
		report := homeNativeReport{UID: 1001, GID: 1002, RootUID: 1001, RootGID: 1002, Home: "/home/itos", Tmp: "/tmp", Mode: "drwx------", Writable: true}
		second := report
		second.HomeBefore = true
		return &world{approval: &approvalWorld{uid: "1001", gid: "1002"}, home: &homeWorld{native: &homeNative{runs: []homeNativeRun{{report: report}, {report: second}}}}}
	}
	if err := fixture().homeNativePersistence(); err != nil {
		t.Fatal(err)
	}
	for _, defect := range []string{"refused", "tmpfs-home", "wrong-owner", "unsafe-mode", "not-writable", "fresh-second-home", "initial-copy", "probe-error", "missing-run"} {
		t.Run(defect, func(t *testing.T) {
			w := fixture()
			run := &w.home.native.runs[0]
			switch defect {
			case "refused":
				run.exit = 1
			case "tmpfs-home":
				run.report.Home = "/tmp"
			case "wrong-owner":
				run.report.RootUID = 0
			case "unsafe-mode":
				run.report.Mode = "drwxrwxrwx"
			case "not-writable":
				run.report.Writable = false
			case "fresh-second-home":
				w.home.native.runs[1].report.HomeBefore = false
			case "initial-copy":
				run.report.Initial = []string{"image-home-file"}
			case "probe-error":
				run.report.Errors = []string{"synthetic probe failure"}
			case "missing-run":
				w.home.native.runs = w.home.native.runs[:1]
			}
			if w.homeNativePersistence() == nil {
				t.Fatal("native home oracle accepted", defect)
			}
		})
	}
}

func TestHomeNativeStepDefinitions(t *testing.T) {
	var output strings.Builder
	suite := godog.TestSuite{Options: &godog.Options{Paths: []string{"harness-home.feature"}, Format: "progress", Output: &output, ShowStepDefinitions: true}, ScenarioInitializer: func(sc *godog.ScenarioContext) { initializeScenario(sc, "unused-root", "unused-binary") }}
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
	count := 0
	for _, feature := range features {
		for _, scenario := range feature.Pickles {
			selected := false
			for _, tag := range scenario.Tags {
				selected = selected || tag.Name == "@ID-HOME-01" || tag.Name == "@ID-HOME-13"
			}
			if !selected {
				continue
			}
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
				count++
			}
		}
	}
	if count != 18 {
		t.Fatalf("checked %d home steps, expected 12 policy and 6 native steps", count)
	}
}
