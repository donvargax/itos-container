package features

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

// Exercise the collector protocol through real scenario hooks and product
// processes. These bounded reruns never recurse into the whole feature suite.
func TestScenarioCoverageContract(t *testing.T) {
	env := readFeatureEnvironment(os.Getenv, runtime.GOOS, runtime.GOARCH)
	if filteredFeatureRun(*scenarios, env) || env.engine == "1" {
		t.Skip("whole-suite harness regression; preserve filtered and engine selections")
	}
	t.Setenv("GOCOVERDIR", "")
	t.Setenv("ITOS_CC_TEST_COVERDIR", "")
	t.Cleanup(removeCallerPath)
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"plain", "aggregate", "per-ID", "both"} {
		t.Run(mode, func(t *testing.T) {
			aggregate, perID := t.TempDir(), t.TempDir()
			if mode == "aggregate" || mode == "both" {
				t.Setenv("GOCOVERDIR", aggregate)
			}
			if mode == "per-ID" || mode == "both" {
				cwd, err := os.Getwd()
				if err != nil {
					t.Fatal(err)
				}
				relative, err := filepath.Rel(cwd, perID)
				if err != nil {
					t.Fatal(err)
				}
				t.Setenv("ITOS_CC_TEST_COVERDIR", relative)
			}
			bin, err := build(root, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			ids := []string{"ID-COUNT-01", "ID-CLI-01"}
			runCoverageScenarios(t, root, bin, ids, env, func(w *world, id string) error {
				want := ""
				if mode == "aggregate" {
					want = aggregate
				} else if mode != "plain" {
					want = filepath.Join(perID, id)
				}
				got := ""
				for _, value := range w.env() {
					if strings.HasPrefix(value, "GOCOVERDIR=") {
						got = strings.TrimPrefix(value, "GOCOVERDIR=")
					}
				}
				if got != want {
					return fmt.Errorf("scenario coverage destination: got %q, want %q", got, want)
				}
				if want != "" {
					if info, err := os.Stat(want); err != nil || !info.IsDir() {
						return fmt.Errorf("coverage destination not created before launch: %s (%v)", want, err)
					}
				}
				return nil
			})
			if mode == "per-ID" || mode == "both" {
				assertCoverageEntries(t, perID, ids)
				for _, id := range ids {
					profile := readProductCoverage(t, filepath.Join(perID, id))
					if got := executedCoverage(profile, "/internal/slices/count/"); got != (id == "ID-COUNT-01") {
						t.Errorf("%s mixed or missing count coverage", id)
					}
				}
			} else {
				assertCoverageEntries(t, perID, nil)
			}
			if mode == "aggregate" {
				if !executedCoverage(readProductCoverage(t, aggregate), "/internal/slices/count/") {
					t.Error("aggregate coverage lacks executed count code")
				}
			} else {
				assertCoverageEntries(t, aggregate, nil)
			}
		})
	}

	t.Run("outlines and contract probes", func(t *testing.T) {
		cover := t.TempDir()
		t.Setenv("ITOS_CC_TEST_COVERDIR", cover)
		bin, err := build(root, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		cases := map[string]string{
			"ID-MANIFEST-29": "/internal/engine/manifest/domain/metadata.go:",
			"ID-APPROVAL-06": "/internal/engine/approval/domain/approval.go:",
		}
		if env.os == "linux" {
			// SESSION-01 goes through sessionLocalFixture's memory/disk worlds.
			cases["ID-SESSION-01"] = "/internal/engine/session/domain/session.go:"
			cases["ID-LIFECYCLE-14"] = "/internal/engine/session/domain/"
			cases["ID-RUN-13"] = "/internal/engine/approval/disk/"
			if env.arch == "amd64" {
				cases["ID-APPROVAL-32"] = "/internal/engine/approval/disk/private.go:"
			}
		}
		var ids []string
		for id := range cases {
			ids = append(ids, id)
		}
		runCoverageScenarios(t, root, bin, ids, env, nil)
		assertCoverageEntries(t, cover, ids)
		for id, source := range cases {
			if !executedCoverage(readProductCoverage(t, filepath.Join(cover, id)), source) {
				t.Errorf("%s lacks executed product blocks for %s", id, source)
			}
			if id == "ID-APPROVAL-32" || id == "ID-RUN-13" || id == "ID-LIFECYCLE-14" || id == "ID-SESSION-01" {
				files, err := filepath.Glob(filepath.Join(cover, id, "covcounters.*"))
				if err != nil || len(files) < 2 {
					t.Errorf("%s did not accumulate outline/secondary-process counters: %v (%v)", id, files, err)
				}
			}
		}
	})

	t.Run("plain covered plain assembly cache", func(t *testing.T) {
		w := &world{root: root, bin: filepath.Join(t.TempDir(), "product")}
		plain, err := w.sessionProbeBinary()
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("ITOS_CC_TEST_COVERDIR", t.TempDir())
		covered, err := w.sessionProbeBinary()
		if err != nil {
			t.Fatal(err)
		}
		if plain == covered {
			t.Fatal("covered request reused plain test assembly")
		}
		t.Setenv("ITOS_CC_TEST_COVERDIR", "")
		again, err := w.sessionProbeBinary()
		if err != nil || again != plain {
			t.Fatalf("plain request did not reuse plain assembly: %q (%v)", again, err)
		}
	})

	t.Run("invalid destinations fail before product launch", func(t *testing.T) {
		for _, tags := range []string{"", "@ID-CLI-01 @ID-COUNT-01", "@ID-CLI-01/../../escape", "@ID-cli-01", "@ID-CLI-01 @ID-bad", "@ID-CLI-01\\escape"} {
			t.Run(tags, func(t *testing.T) { rejectCoverageFixture(t, root, tags, t.TempDir(), "ID") })
		}
		file := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(file, nil, 0600); err != nil {
			t.Fatal(err)
		}
		rejectCoverageFixture(t, root, "@ID-CLI-01", file, "coverage")
	})
}

func runCoverageScenarios(t *testing.T, root, bin string, ids []string, env featureEnvironment, before func(*world, string) error) {
	t.Helper()
	selection, err := selectScenarios("^@(?:"+strings.Join(ids, "|")+")$", ".", env)
	if err != nil || selection.count == 0 {
		t.Fatalf("coverage scenario selection: %+v (%v)", selection, err)
	}
	var output bytes.Buffer
	var mu sync.Mutex
	var paths []string
	var routingErrors []error
	code := runScenarios(selection, godog.Options{Paths: []string{"."}, Format: "progress", Output: &output, Concurrency: 4}, func(sc *godog.ScenarioContext) {
		w := initializeScenario(sc, root, bin)
		sc.Before(func(ctx context.Context, scenario *godog.Scenario) (context.Context, error) {
			mu.Lock()
			paths = append(paths, w.dir)
			mu.Unlock()
			for _, tag := range scenario.Tags {
				if before != nil && strings.HasPrefix(tag.Name, "@ID-") {
					if err := before(w, strings.TrimPrefix(tag.Name, "@")); err != nil {
						mu.Lock()
						routingErrors = append(routingErrors, err)
						mu.Unlock()
					}
				}
			}
			return ctx, nil
		})
		sc.StepContext().After(func(ctx context.Context, _ *godog.Step, _ godog.StepResultStatus, err error) (context.Context, error) {
			if strings.Contains(w.stderr, "GOCOVERDIR not set") {
				return ctx, fmt.Errorf("product emitted missing coverage warning: %s", w.stderr)
			}
			return ctx, err
		})
	})
	if code != 0 || len(paths) != selection.count {
		t.Fatalf("coverage scenarios failed (%d/%d): %s", len(paths), selection.count, output.String())
	}
	assertFeatureScratchRemoved(t, paths)
	for _, err := range routingErrors {
		t.Error(err)
	}
}

func rejectCoverageFixture(t *testing.T, root, tags, destination, diagnostic string) {
	t.Helper()
	t.Setenv("ITOS_CC_TEST_COVERDIR", destination)
	dir := writeFeatureFixture(t, "Feature: coverage refusal\n  "+tags+"\n  Scenario: invalid coverage request\n    Given coverage launch sentinel\n")
	launched := false
	var output bytes.Buffer
	suite := godog.TestSuite{Options: &godog.Options{Paths: []string{dir}, Format: "pretty", Output: &output}, ScenarioInitializer: func(sc *godog.ScenarioContext) {
		initializeScenario(sc, root, "unused-binary")
		sc.Step(`^coverage launch sentinel$`, func() { launched = true })
	}}
	if code := suite.Run(); code == 0 || launched || !strings.Contains(output.String(), diagnostic) {
		t.Fatalf("invalid coverage must fail usefully before launch: code=%d launched=%v\n%s", code, launched, output.String())
	}
}

func assertCoverageEntries(t *testing.T, dir string, want []string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	want = slices.Clone(want)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("coverage entries: got %v, want %v", got, want)
	}
}

func readProductCoverage(t *testing.T, dir string) string {
	t.Helper()
	for _, pattern := range []string{"covmeta.*", "covcounters.*"} {
		files, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil || len(files) == 0 {
			t.Fatalf("missing %s in %s (%v)", pattern, dir, err)
		}
		for _, file := range files {
			data, err := os.ReadFile(file)
			if err != nil || len(data) == 0 {
				t.Fatalf("unreadable or empty coverage file %s (%v)", file, err)
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	profile := filepath.Join(t.TempDir(), "coverage.out")
	cmd := exec.CommandContext(ctx, "go", "tool", "covdata", "textfmt", "-i="+dir, "-o="+profile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("read product coverage: %v\n%s", err, out)
	}
	data, err := os.ReadFile(profile)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "mode: atomic\n") || !executedCoverage(text, "/internal/") {
		t.Fatalf("%s lacks atomic executed product coverage", dir)
	}
	return text
}

func executedCoverage(profile, source string) bool {
	for line := range strings.SplitSeq(profile, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && strings.Contains(fields[0], source) && !strings.Contains(fields[0], "/porttest/") && !strings.Contains(fields[0], "_test.go:") && fields[2] != "0" {
			return true
		}
	}
	return false
}
