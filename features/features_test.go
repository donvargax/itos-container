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
//	ITOS_CC_TEST_COVERDIR=<root> go test ./features    coverage in <root>/<ID>
//
// godog's own tag filter takes exact tags joined by commas; itos's run
// templates join the IDs they select with |, as a regular expression
// (itos.yaml's tests.scenario.run), so -scenarios takes that expression and
// is intersected with the live set, selected engine tier and platform using godog.
// Ordinary runs exclude @wip. Manual CI can select exact IDs for red evidence;
// @engine is still CI-only and @linux requires a Linux test process (see README.md).
//
// Product subprocesses need -cover -covermode=atomic -coverpkg=./... and an
// existing GOCOVERDIR. Either GOCOVERDIR or ITOS_CC_TEST_COVERDIR requests
// covered CLI and test-assembly builds. The latter routes each scenario to its
// absolute <root>/<ID> directory and takes precedence over aggregate coverage.
// Destinations belong to worlds (including their secondary fixture worlds), not
// process-global Before-hook mutations. Outline rows accumulate under one ID;
// cleanup releases fixture state but preserves counters for the external reader.
// With neither input, selected and whole runs retain plain builds and output.
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
	"time"

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
	selection.started = time.Now()
	t.Cleanup(removeCallerPath)
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	bin, err := buildProduct(root, t.TempDir(), selection.engine)
	if err != nil {
		t.Fatal(err)
	}
	if runFeatureScenarios(selection, godog.Options{
		Format:      "pretty",
		Output:      os.Stdout,
		Paths:       []string{"."},
		Strict:      true,
		TestingT:    t,
		Concurrency: 4,
	}, func(sc *godog.ScenarioContext, duringQuiet func()) {
		w := initializeScenario(sc, root, bin)
		w.duringQuiet = duringQuiet
	}) != 0 {
		t.Fatal("a scenario failed")
	}
	if dir := os.Getenv("ITOS_CC_TEST_COVERDIR"); selection.engine && dir != "" {
		// The CI engine invocation keeps its behavior assertions and also proves
		// that every selected ID exported readable, executed product counters.
		readScenarioCoverage(t, dir, selection.ids)
	}
}

// Environment and platform values enter the policy as data. Harness entry points
// supply the real lookup; regression fixtures never enable engine access.
type featureEnvironment struct{ engine, ci, actions, event, red, shard, os, arch string }

func readFeatureEnvironment(lookup func(string) string, hostOS, arch string) featureEnvironment {
	env := featureEnvironment{
		engine: lookup("ITOS_CONTAINER_ENGINE_TESTS"),
		ci:     lookup("CI"), actions: lookup("GITHUB_ACTIONS"),
		event: lookup("GITHUB_EVENT_NAME"), shard: lookup("ITOS_CONTAINER_ENGINE_FEATURES"), os: hostOS, arch: arch,
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
	paths    []string // the shard's feature files; empty reads whatever the run is given
	count    int
	skip     string
	excluded []string
	started  time.Time
	timing   *engineTiming
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

// featureName is a feature file's basename: no path, no godog line suffix.
var featureName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*\.feature$`)

// engineFeatures reads ITOS_CONTAINER_ENGINE_FEATURES, the whitespace-separated
// basenames of the feature files one engine shard runs. Blank is the whole tier.
func (env featureEnvironment) engineFeatures(engine bool) ([]string, error) {
	names := strings.Fields(env.shard)
	if len(names) == 0 {
		return nil, nil
	}
	if !engine {
		return nil, fmt.Errorf("ITOS_CONTAINER_ENGINE_FEATURES applies only to the engine tier (ITOS_CONTAINER_ENGINE_TESTS=1)")
	}
	for i, name := range names {
		if !featureName.MatchString(name) {
			return nil, fmt.Errorf("ITOS_CONTAINER_ENGINE_FEATURES: %q is not a feature file basename ending in .feature", name)
		}
		if slices.Contains(names[:i], name) {
			return nil, fmt.Errorf("ITOS_CONTAINER_ENGINE_FEATURES: %q is given twice", name)
		}
	}
	return names, nil
}

// These tags select native binaries before any scenario hook or fixture runs.
// The repository's macOS/Windows jobs are manual; Linux proof never runs them.
func (env featureEnvironment) excludedPlatforms() []string {
	var tags []string
	if env.os != "linux" && env.os != "darwin" {
		tags = append(tags, "@unix")
	}
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
//
// An engine shard (ITOS_CONTAINER_ENGINE_FEATURES) selects from its own files alone,
// and the filtered-pickle check below reads those same files: godog numbers pickles
// across the files of one read, so a check over other files than the selection's
// would never match. Red IDs and the expression are still validated against every
// file, and the rows a shard leaves to the others are named among its exclusions.
func selectScenarios(selection, dir string, env featureEnvironment) (scenarioSelection, error) {
	var result scenarioSelection
	engine, err := env.engineEnabled()
	if err != nil {
		return result, err
	}
	result.engine = engine
	shard, err := env.engineFeatures(engine)
	if err != nil {
		return result, err
	}
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
	inventory, err := suite.RetrieveFeatures()
	if err != nil {
		return result, err
	}
	features, paths := inventory, []string(nil)
	if len(shard) > 0 {
		engineFiles := map[string]bool{}
		for _, feature := range inventory {
			for _, scenario := range feature.Pickles {
				for _, tag := range scenario.Tags {
					engineFiles[filepath.Base(scenario.Uri)] = engineFiles[filepath.Base(scenario.Uri)] || tag.Name == "@engine"
				}
			}
		}
		for _, name := range shard {
			if !engineFiles[name] {
				return result, fmt.Errorf("ITOS_CONTAINER_ENGINE_FEATURES: no feature file %q has an @engine case", name)
			}
			paths = append(paths, filepath.Join(dir, name))
		}
		suite.Options.Paths = paths
		if features, err = suite.RetrieveFeatures(); err != nil {
			return result, err
		}
	}
	matchedTags := map[string]bool{}
	expected := map[string]bool{}
	matched, liveEngine := 0, 0
	inspect := func(scenario *godog.Scenario, mine bool) {
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
		if needsEngine && !wip && mine {
			liveEngine++
		}
		if !matches {
			return
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
			return
		}
		if !mine {
			result.excluded = append(result.excluded, fmt.Sprintf("%s %q (other engine shard)", strings.Join(ids, ","), scenario.Name))
			return
		}
		expected[scenario.Uri+":"+scenario.Id] = true
		result.ids = append(result.ids, ids...)
		result.count++
	}
	for _, feature := range features {
		for _, scenario := range feature.Pickles {
			inspect(scenario, true)
		}
	}
	for _, feature := range inventory {
		for _, scenario := range feature.Pickles {
			if len(shard) > 0 && !slices.Contains(shard, filepath.Base(scenario.Uri)) {
				inspect(scenario, false)
			}
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
	result.paths = paths
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
	// suite still holds the shard's own paths, if it has any.
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

// scope reads the files the selection read: a shard's own, whatever options named.
func (selection scenarioSelection) scope(options godog.Options) godog.Options {
	if len(selection.paths) > 0 {
		options.Paths = selection.paths
	}
	return options
}

func runScenarios(selection scenarioSelection, options godog.Options, initialize func(*godog.ScenarioContext)) int {
	if selection.skip != "" {
		return 0 // TestFeatures reports the explicit testing.T skip before calling us.
	}
	options = selection.scope(options)
	options.Tags = selection.filter
	options.Strict = true
	// Local worlds own their processes, state and endpoints. Overlap their
	// real deadline waits, without changing those deadlines or the selected set.
	// Native resource audits and red evidence retain their serial ordering.
	if selection.engine || selection.red || options.Concurrency < 1 {
		options.Concurrency = 1
	} else {
		options.Concurrency = min(options.Concurrency, 4)
	}
	completed, unsuccessful := 0, false
	suite := godog.TestSuite{Name: "itos-container", Options: &options, ScenarioInitializer: func(sc *godog.ScenarioContext) {
		var started time.Time
		rowFailed := false
		if selection.timing != nil {
			// Register before fixture setup, including a failing setup hook.
			sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
				started, rowFailed = time.Now(), false
				return ctx, nil
			})
		}
		initialize(sc)
		if selection.engine || selection.red {
			sc.StepContext().After(func(ctx context.Context, _ *godog.Step, status godog.StepResultStatus, err error) (context.Context, error) {
				unsuccessful = unsuccessful || status != godog.StepPassed || err != nil
				rowFailed = rowFailed || status != godog.StepPassed || err != nil
				return ctx, nil
			})
			sc.After(func(ctx context.Context, scenario *godog.Scenario, err error) (context.Context, error) {
				completed++
				unsuccessful = unsuccessful || err != nil
				if selection.timing != nil {
					// Godog runs After hooks in registration order. This is after
					// all fixture cleanup and receives any cleanup error as well.
					selection.timing.complete(scenario, started, rowFailed || err != nil, signalOffsetsOf(ctx))
				}
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
// git of the caller's. Either coverage input requests a -cover build (see the
// package's comment).
func build(root, dir string) (string, error) {
	return buildProduct(root, dir, false)
}

// buildProduct is build, except that the engine tier's product is built from a
// copy of the sources with the shortened budget fixture (engineBudgetTree).
func buildProduct(root, dir string, engine bool) (string, error) {
	name := "itos-container"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(dir, name)
	args := []string{"build", "-trimpath", "-buildvcs=false",
		"-ldflags", "-X " + versionSymbol + "=" + stampedVersion + " -X " + commitSymbol + "=" + stampedCommit}
	source := root
	if engine {
		var err error
		if source, err = engineBudgetTree(root, dir); err != nil {
			return "", err
		}
	}
	if productCoverageRequested() {
		args = append(args, "-cover", "-covermode=atomic", "-coverpkg=./...")
	}
	cmd := exec.Command("go", append(args, "-o", bin, "./cmd/itos-container")...)
	cmd.Dir = source
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("building itos-container: %v\n%s", err, out)
	}
	return bin, nil
}

type noMatch struct{ selection string }

func (e *noMatch) Error() string {
	return "no scenario has a tag that " + e.selection + " matches"
}
