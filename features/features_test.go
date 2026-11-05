// The features: every *.feature file in this folder, run by godog through go
// test against the itos-container binary built once a run from this tree,
// each scenario a subtest of TestFeatures. Harvested from itos's
// (github.com/donvargax/itos, features/features_test.go) and the owner's
// itos-template's, without either one's generator behaviour.
//
//	go test ./features -count=1                        every live non-engine scenario
//	go test ./features -count=1 -scenarios=<regexp>    the live scenarios with a
//	                                                   tag the expression matches
//	GOCOVERDIR=<dir> go test ./features -count=1       either, itos-container
//	                                                   built with -cover, what
//	                                                   the scenarios ran in <dir>
//
// godog's own tag filter takes exact tags joined by commas; itos's run
// templates join the IDs they select with |, as a regular expression
// (itos.yaml's tests.scenario.run), so -scenarios takes that expression and
// is intersected with the live set, selected engine tier and platform using godog.
// Ordinary runs exclude @wip. Manual CI can select exact IDs for red evidence;
// @engine is still CI-only and @linux requires a Linux test process (see README.md).
//
// The scenarios run itos-container as another process, where Go records
// coverage only for a binary built with -cover and run with GOCOVERDIR naming
// an existing folder (go.dev/doc/build-cover); go test's own -cover sees none
// of it. So with GOCOVERDIR set, the binary is built with -cover
// -covermode=atomic -coverpkg=./..., every package of the module counted, and
// what the scenarios run lands in that folder, where itos-cc merges it with go
// test's. The build keys on GOCOVERDIR alone: a -cover binary run without it
// writes "warning: GOCOVERDIR not set, no coverage data emitted" to its
// stderr, which breaks every scenario that reads stderr, and only
// GOCOVERDIR's being set guarantees every command a scenario starts has it.
// w.env() passes it on unchanged, every environment a step runs a command in
// is w.env() changed elsewhere (noGit, knowingNoOne, withGitConfig). Never
// ITOS_CC_TEST_COVERDIR, which itos-cc sets alone for a listed test's first
// coverage run and w.env() drops with every ITOS_ variable. With GOCOVERDIR
// unset the build is the plain one.
package features

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

var scenarios = flag.String("scenarios", "", "run only the live scenarios with a tag this regular expression matches")

// stampedVersion is the version the binary under test is stamped with, as a
// release's build stamps the release's, so a scenario can tell the stamp
// from the version a build without one says.
const stampedVersion = "1.2.3-features"

// stampedCommit is the commit the binary under test is stamped with, as a
// release's build stamps the commit it was built from, so a scenario can
// name the commit the binary says it was built from.
const stampedCommit = "0123456789abcdef0123456789abcdef01234567"

// versionSymbol and commitSymbol are the variables the build stamps
// (-ldflags -X).
const (
	versionSymbol = "github.com/donvargax/itos-container/internal/version.stamp"
	commitSymbol  = "github.com/donvargax/itos-container/internal/version.commit"
)

func TestFeatures(t *testing.T) {
	selection, err := selectScenarios(*scenarios, ".", readFeatureEnvironment(os.Getenv, runtime.GOOS, runtime.GOARCH))
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.excluded) > 0 {
		t.Logf("excluded %d scenario(s): %s", len(selection.excluded), strings.Join(selection.excluded, "; "))
	}
	if selection.skip != "" {
		t.Skip(selection.skip)
	}
	t.Cleanup(removeCallerPath)
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	bin, err := build(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if runScenarios(selection, godog.Options{
		Format:   "pretty",
		Output:   os.Stdout,
		Paths:    []string{"."},
		Strict:   true,
		TestingT: t,
	}, func(sc *godog.ScenarioContext) { initializeScenario(sc, root, bin) }) != 0 {
		t.Fatal("a scenario failed")
	}
}

// Environment and platform values enter the policy as data. Harness entry points
// supply the real lookup; regression fixtures never enable engine access.
type featureEnvironment struct{ engine, ci, actions, event, red, os, arch string }

func readFeatureEnvironment(lookup func(string) string, hostOS, arch string) featureEnvironment {
	env := featureEnvironment{
		engine: lookup("ITOS_CONTAINER_ENGINE_TESTS"),
		ci:     lookup("CI"), actions: lookup("GITHUB_ACTIONS"),
		event: lookup("GITHUB_EVENT_NAME"), os: hostOS, arch: arch,
	}
	if env.manualDispatch() {
		env.red = lookup("ITOS_CONTAINER_RED_SCENARIOS")
	}
	return env
}

func (env featureEnvironment) manualDispatch() bool {
	return env.ci == "true" && env.actions == "true" && env.event == "workflow_dispatch"
}

func (env featureEnvironment) redRequested() bool {
	return env.manualDispatch() && strings.TrimSpace(env.red) != ""
}

func (env featureEnvironment) redIDs() ([]string, error) {
	if !env.redRequested() {
		return nil, nil
	}
	ids := strings.Fields(env.red)
	valid := regexp.MustCompile(`^@ID-[A-Z]+-[0-9]+$`)
	for _, id := range ids {
		if !valid.MatchString(id) {
			return nil, fmt.Errorf("invalid red scenario tag %q: expected @ID-AREA-number", id)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}

type scenarioSelection struct {
	filter   string
	ids      []string
	engine   bool
	red      bool
	count    int
	skip     string
	excluded []string
}

func (env featureEnvironment) engineEnabled() (bool, error) {
	switch env.engine {
	case "", "0":
		return false, nil
	case "1":
		if env.ci != "true" || env.actions != "true" || env.os != "linux" || env.arch != "amd64" {
			return false, fmt.Errorf("ITOS_CONTAINER_ENGINE_TESTS=1 requires CI=true, GITHUB_ACTIONS=true and a Linux amd64 test process")
		}
		return true, nil
	default:
		return false, fmt.Errorf("ITOS_CONTAINER_ENGINE_TESTS must be empty, 0 or 1")
	}
}

// These tags select native binaries before any scenario hook or fixture runs.
// The repository's macOS/Windows jobs are manual; Linux proof never runs them.
func (env featureEnvironment) excludedPlatforms() []string {
	var tags []string
	if env.os != "linux" || env.arch != "amd64" {
		tags = append(tags, "@local-engine-fixture")
	}
	if env.os != "darwin" && env.os != "windows" {
		tags = append(tags, "@manual-platform", "@unsupported-host")
	}
	return tags
}

// RetrieveFeatures compiles Gherkin pickles, including inherited Feature, Rule,
// Outline and Examples tags. Selection runs before any binary build or hooks.
func selectScenarios(selection, dir string, env featureEnvironment) (scenarioSelection, error) {
	var result scenarioSelection
	engine, err := env.engineEnabled()
	if err != nil {
		return result, err
	}
	result.engine = engine
	redIDs, err := env.redIDs()
	if err != nil {
		return result, err
	}
	result.red = len(redIDs) != 0
	if result.red {
		if selection != "" {
			return result, fmt.Errorf("red scenarios cannot be combined with -scenarios")
		}
		// Only validated complete IDs become exact anchored alternatives.
		selection = "^(?:" + strings.Join(redIDs, "|") + ")$"
	}
	pattern, err := regexp.Compile(selection)
	if err != nil {
		return result, err
	}
	suite := godog.TestSuite{Options: &godog.Options{Paths: []string{dir}}}
	features, err := suite.RetrieveFeatures()
	if err != nil {
		return result, err
	}
	matchedTags := map[string]bool{}
	expected := map[string]bool{}
	matched, liveEngine := 0, 0
	for _, feature := range features {
		for _, scenario := range feature.Pickles {
			wip, needsEngine, needsLinux, matches := false, false, false, selection == ""
			wrongPlatform := false
			var ids []string
			for _, tag := range scenario.Tags {
				wrongPlatform = wrongPlatform || slices.Contains(env.excludedPlatforms(), tag.Name)
				wip = wip || tag.Name == "@wip"
				needsEngine = needsEngine || tag.Name == "@engine"
				needsLinux = needsLinux || tag.Name == "@linux"
				if strings.HasPrefix(tag.Name, "@ID-") {
					ids = append(ids, tag.Name)
				}
				if selection != "" && pattern.MatchString(tag.Name) {
					matches = true
					matchedTags[tag.Name] = true
				}
			}
			if needsEngine && !wip {
				liveEngine++
			}
			if !matches {
				continue
			}
			matched++
			if (wip && !result.red) || needsEngine != engine || wrongPlatform || (needsLinux && env.os != "linux") {
				reason := "other tier"
				if wrongPlatform {
					reason = "other platform"
				}
				if wip && !result.red {
					reason = "@wip"
				} else if needsEngine {
					reason = "@engine disabled"
				} else if needsLinux && env.os != "linux" {
					reason = "@linux requires Linux"
				}
				result.excluded = append(result.excluded, fmt.Sprintf("%s %q (%s)", strings.Join(ids, ","), scenario.Name, reason))
				continue
			}
			expected[scenario.Uri+":"+scenario.Id] = true
			result.ids = append(result.ids, ids...)
			result.count++
		}
	}
	for _, id := range redIDs {
		if !matchedTags[id] {
			return result, fmt.Errorf("unknown red scenario tag %q in feature inventory", id)
		}
	}
	if selection != "" && matched == 0 {
		return result, &noMatch{selection}
	}
	slices.Sort(result.ids)
	result.ids = slices.Compact(result.ids)
	if result.count == 0 {
		result.skip = "no live scenarios in selected tier and platform (count 0)"
		if result.red {
			result.skip = fmt.Sprintf("no eligible red scenarios (count 0); excluded %d scenario(s): %s", len(result.excluded), strings.Join(result.excluded, "; "))
		} else if engine && liveEngine == 0 {
			result.skip = "no live @engine scenarios (count 0)"
		}
		return result, nil
	}

	tier := "~@engine"
	if engine {
		tier = "@engine"
	}
	if !result.red {
		tier = "~@wip&&" + tier
	}
	if env.os != "linux" {
		tier += "&&~@linux"
	}
	for _, tag := range env.excludedPlatforms() {
		tier += "&&~" + tag
	}
	result.filter = tier
	if selection != "" {
		var branches []string
		for tag := range matchedTags {
			// godog uses comma for OR and && for AND. Every OR branch must
			// carry all restrictions, including expressions matching @wip or @linux.
			branches = append(branches, tag+"&&"+tier)
		}
		slices.Sort(branches)
		result.filter = strings.Join(branches, ",")
	}
	// Check godog's actual filtered pickles before hooks. This also refuses a
	// tag containing filter operators if it would broaden or lose the selection.
	suite.Options.Tags = result.filter
	filtered, err := suite.RetrieveFeatures()
	if err != nil {
		return result, err
	}
	for _, feature := range filtered {
		for _, scenario := range feature.Pickles {
			key := scenario.Uri + ":" + scenario.Id
			if !expected[key] {
				return result, fmt.Errorf("godog tag filter selected an excluded scenario %q", scenario.Name)
			}
			delete(expected, key)
		}
	}
	if len(expected) != 0 {
		return result, fmt.Errorf("godog tag filter lost %d selected scenario(s)", len(expected))
	}
	return result, nil
}

func runScenarios(selection scenarioSelection, options godog.Options, initialize func(*godog.ScenarioContext)) int {
	if selection.skip != "" {
		return 0 // TestFeatures reports the explicit testing.T skip before calling us.
	}
	options.Tags = selection.filter
	options.Strict = true
	options.Concurrency = 1
	completed, unsuccessful := 0, false
	suite := godog.TestSuite{Name: "itos-container", Options: &options, ScenarioInitializer: func(sc *godog.ScenarioContext) {
		initialize(sc)
		if selection.engine || selection.red {
			sc.StepContext().After(func(ctx context.Context, _ *godog.Step, status godog.StepResultStatus, err error) (context.Context, error) {
				unsuccessful = unsuccessful || status != godog.StepPassed || err != nil
				return ctx, nil
			})
			sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
				completed++
				unsuccessful = unsuccessful || err != nil
				return ctx, nil
			})
		}
	}}
	code := suite.Run()
	if (selection.engine || selection.red) && (unsuccessful || completed != selection.count) {
		if options.Output != nil {
			_, _ = fmt.Fprintf(options.Output, "selected execution incomplete or unsuccessful: completed %d of %d scenario(s)\n", completed, selection.count)
		}
		return 1
	}
	return code
}

// build builds cmd/itos-container from the module at root into dir,
// stamped with stampedVersion and stampedCommit, once for every scenario of
// the run: itos-container, or itos-container.exe on windows, where a
// program is found by its extension. No VCS stamping, so the build runs no
// git of the caller's. With GOCOVERDIR set it is a -cover build (see the
// package's comment).
func build(root, dir string) (string, error) {
	name := "itos-container"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(dir, name)
	args := []string{"build", "-trimpath", "-buildvcs=false",
		"-ldflags", "-X " + versionSymbol + "=" + stampedVersion + " -X " + commitSymbol + "=" + stampedCommit}
	if os.Getenv("GOCOVERDIR") != "" {
		args = append(args, "-cover", "-covermode=atomic", "-coverpkg=./...")
	}
	cmd := exec.Command("go", append(args, "-o", bin, "./cmd/itos-container")...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("building itos-container: %v\n%s", err, out)
	}
	return bin, nil
}

type noMatch struct{ selection string }

func (e *noMatch) Error() string {
	return "no scenario has a tag that " + e.selection + " matches"
}
