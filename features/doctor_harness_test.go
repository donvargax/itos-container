package features

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

func TestDoctorPlatformSelection(t *testing.T) {
	dir := writeFeatureFixture(t, `Feature: platform selection
  @ID-MEMORY-31 @local-engine-fixture
  Scenario: local API
    Then record "local"
  @ID-MEMORY-32 @manual-platform @unsupported-host
  Scenario: unsupported host
    Then record "unsupported"
  @ID-MEMORY-33 @engine
  Scenario: actual engine
    Then record "engine"
`)
	for _, tc := range []struct {
		os, arch, pattern string
		want              []string
		excluded          int
	}{
		{"linux", "amd64", "", []string{"local"}, 2},
		{"linux", "amd64", "^@ID-MEMORY-32$", nil, 1},
		{"darwin", "arm64", "", []string{"unsupported"}, 2},
		{"windows", "amd64", "", []string{"unsupported"}, 2},
		{"windows", "amd64", "^@ID-MEMORY-31$", nil, 1},
		{"linux", "arm64", "", nil, 3},
		{"linux", "amd64", "^@ID-MEMORY-31$|^@ID-MEMORY-32$", []string{"local"}, 1},
	} {
		t.Run(tc.os+tc.arch+tc.pattern, func(t *testing.T) {
			selection, err := selectScenarios(tc.pattern, dir, featureEnvironment{os: tc.os, arch: tc.arch})
			if err != nil {
				t.Fatal(err)
			}
			seen := executeMemory(t, dir, selection, nil)
			if !slices.Equal(seen.steps, tc.want) || seen.before != len(tc.want) || seen.after != len(tc.want) || len(selection.excluded) != tc.excluded {
				t.Fatalf("platform leaked setup/steps: %+v exclusions %v", seen, selection.excluded)
			}
		})
	}
}

func TestDoctorFixtureJournal(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Unix fixture is selected only on Linux")
	}
	w := &world{dir: t.TempDir()}
	if err := w.doctorCheckout(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.closeDoctor)
	if err := w.doctorLocalAPI(false); err != nil {
		t.Fatal(err)
	}
	transport := doctorTransport(w.doctor.engine.socket)
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: time.Second}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		request, err := http.NewRequestWithContext(context.Background(), method, "http://fixture/v1.56/images/"+doctorImage+"/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		want := 200
		if method == http.MethodPost {
			want = 403
		}
		if response.StatusCode != want {
			t.Fatalf("%s status %d, want %d", method, response.StatusCode, want)
		}
	}
	if requests := w.doctor.engine.requests(); len(requests) != 2 || requests[1].Method != http.MethodPost {
		t.Fatalf("forbidden attempt was not journaled: %+v", requests)
	}
	if err := w.doctorAllowedJournal(true); err == nil {
		t.Fatal("assertion accepted a refused mutation attempt")
	}
}

func TestDoctorTreeDetectsChanges(t *testing.T) {
	w := &world{dir: t.TempDir()}
	if err := w.doctorCheckout(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.closeDoctor)
	var err error
	w.doctor.snapshot, err = doctorTree(w.dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.doctorUnchanged(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(w.doctor.host, "new-record")
	if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := w.doctorUnchanged(); err == nil {
		t.Fatal("new host file escaped snapshot assertion")
	}
}

func TestDoctorAssertionsRejectMissingInspection(t *testing.T) {
	w := &world{stdout: `{"schema":1,"ok":true,"problems":[],"manifest":{"valid":true}}`}
	if err := w.doctorReady(); err == nil {
		t.Fatal("file-only success counted as readiness")
	}
	w.stdout = `{"schema":1,"ok":false,"problems":[{"rule":"doctor.image.missing","exit":3,"message":"missing","fix":"prepare","image":"fixture"}]}`
	w.exit = 3
	if err := w.doctorProblem("doctor.image.missing", 3); err == nil {
		t.Fatal("refusal lacking inspection.ready=false accepted")
	}
	w.stdout = `{"schema":1,"ok":true,"problems":[],"inspection":{"ready":true,"approved":true}}`
	w.exit = 0
	if err := w.doctorReady(); err == nil {
		t.Fatal("inspection approval claim accepted")
	}
	w.stdout = `{"schema":1,"ok":true,"problems":[],"inspection":{"ready":true}} {}`
	if err := w.doctorReady(); err == nil {
		t.Fatal("two JSON objects accepted")
	}
}

// Godog can list registered definitions without executing any hooks or steps.
// Match its actual expressions to every compiled outline row, including CI-only
// cases. This checks completeness, never supplies behavioural red evidence.
func TestDoctorStepDefinitions(t *testing.T) {
	var output strings.Builder
	suite := godog.TestSuite{Options: &godog.Options{Paths: []string{"doctor.feature"}, Format: "progress", Output: &output, ShowStepDefinitions: true}, ScenarioInitializer: func(sc *godog.ScenarioContext) { initializeScenario(sc, "unused-root", "unused-binary") }}
	if code := suite.Run(); code != 2 {
		t.Fatalf("definition listing exited %d", code)
	}
	ansi := regexp.MustCompile("\x1b\\[[0-9;]*m")
	var definitions []*regexp.Regexp
	for _, line := range strings.Split(ansi.ReplaceAllString(output.String(), ""), "\n") {
		expression, _, ok := strings.Cut(line, " # ")
		if !ok {
			continue
		}
		definition, err := regexp.Compile(strings.TrimSpace(expression))
		if err != nil {
			t.Fatal(err)
		}
		definitions = append(definitions, definition)
	}
	features, err := suite.RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, feature := range features {
		for _, scenario := range feature.Pickles {
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
	if count == 0 {
		t.Fatal("no doctor steps checked")
	}
}
