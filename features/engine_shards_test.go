package features

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"go.yaml.in/yaml/v3"
)

// The engine tier runs as three shards (T-45), each a native runner and daemon of
// whole feature files. Nothing here runs an engine, Docker or a product command:
// the workflow is parsed, the real feature files are selected as data, and fixture
// features run with memory-only steps, as the schedule regressions do.

// shardWorkflow models only what the shard checks read. A negative control marshals it
// back, so a field the checks read must be modelled here.
type shardWorkflow struct {
	Jobs map[string]shardJob `yaml:"jobs"`
}

type shardJob struct {
	Name            string            `yaml:"name"`
	If              string            `yaml:"if"`
	Runner          string            `yaml:"runs-on"`
	Timeout         int               `yaml:"timeout-minutes"`
	ContinueOnError bool              `yaml:"continue-on-error"`
	Needs           []string          `yaml:"needs"`
	Env             map[string]string `yaml:"env"`
	Container       any               `yaml:"container"`
	Services        map[string]any    `yaml:"services"`
	Strategy        struct {
		FailFast *bool `yaml:"fail-fast"`
		Matrix   struct {
			Include []shardEntry `yaml:"include"`
		} `yaml:"matrix"`
	} `yaml:"strategy"`
	Steps []shardStep `yaml:"steps"`
}

type shardEntry struct {
	Shard    string `yaml:"shard"`
	Features string `yaml:"features"`
}

type shardStep struct {
	Name            string            `yaml:"name"`
	If              string            `yaml:"if"`
	Timeout         int               `yaml:"timeout-minutes"`
	ContinueOnError bool              `yaml:"continue-on-error"`
	Env             map[string]string `yaml:"env"`
	Run             string            `yaml:"run"`
}

const (
	shardFeaturesVariable = "ITOS_CONTAINER_ENGINE_FEATURES"
	shardJobID            = "engine-shard"
	shardGuard            = ciGuard
	shardGate             = `test "$SHARD_RESULT" = success`
)

// engineFeatureFiles lists the feature files under dir with a case that has an
// effective @engine tag, live or @wip, inherited or its own.
func engineFeatureFiles(dir string) ([]string, error) {
	features, err := (&godog.TestSuite{Options: &godog.Options{Paths: []string{dir}}}).RetrieveFeatures()
	if err != nil {
		return nil, err
	}
	files := map[string]bool{}
	for _, feature := range features {
		for _, pickle := range feature.Pickles {
			for _, tag := range pickle.Tags {
				if tag.Name == "@engine" {
					files[filepath.Base(pickle.Uri)] = true
				}
			}
		}
	}
	return slices.Sorted(maps.Keys(files)), nil
}

// engineShardIssues holds the sharded engine job and its aggregate to T-45's
// contract, against the real feature files for the partition of the features.
func engineShardIssues(data []byte) []string {
	var workflow shardWorkflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return []string{err.Error()}
	}
	var issues []string
	require := func(ok bool, issue string) {
		if !ok {
			issues = append(issues, issue)
		}
	}
	shard, aggregate := workflow.Jobs[shardJobID], workflow.Jobs["engine"]
	_, hasShard := workflow.Jobs[shardJobID]
	require(hasShard, "an engine-shard job")
	require(shard.Name == "Engine shard (${{ matrix.shard }})", "shard job name")
	require(shard.If == eventGuard, "shard event guard")
	require(shard.Runner == "ubuntu-latest" && shard.Container == nil && len(shard.Services) == 0, "shard native runner")
	require(shard.Timeout == 10, "shard job budget")
	require(!shard.ContinueOnError && len(shard.Needs) == 0, "shard failure propagation and independence")
	require(shard.Strategy.FailFast != nil && !*shard.Strategy.FailFast, "fail-fast false")
	require(len(shard.Env) == 0, "step-local feature input and engine switch")

	include := shard.Strategy.Matrix.Include
	var names []string
	for _, entry := range include {
		names = append(names, entry.Shard)
	}
	slices.Sort(names)
	require(slices.Equal(names, []string{"lifecycle", "offline", "other"}), "exactly three named shards: lifecycle, offline and other")
	want, err := engineFeatureFiles(".")
	if err != nil {
		issues = append(issues, "feature inventory: "+err.Error())
	}
	placed := map[string]int{}
	for _, entry := range include {
		fields := strings.Fields(entry.Features)
		require(len(fields) > 0, "empty shard "+entry.Shard)
		for _, name := range fields {
			require(name == filepath.Base(name) && !strings.ContainsAny(name, `/\:`) && strings.HasSuffix(name, ".feature"), "feature path "+strconv.Quote(name)+" in shard "+entry.Shard)
			require(err != nil || slices.Contains(want, name), "unknown engine feature "+strconv.Quote(name))
			placed[name]++
		}
	}
	for _, name := range want {
		require(placed[name] != 0, "feature in no shard: "+name)
	}
	for name, count := range placed {
		require(count < 2, "feature in two shards: "+name)
	}

	enabled := ""
	scenarios := 0
	for i, step := range shard.Steps {
		if step.Name != "Engine scenarios" {
			continue
		}
		scenarios++
		prefix := "jobs." + shardJobID + ".steps." + strconv.Itoa(i) + ".env."
		enabled = prefix + "ITOS_CONTAINER_ENGINE_TESTS"
		require(step.Env[shardFeaturesVariable] == "${{ matrix.features }}", "step-local feature input")
		require(strings.Contains(step.Run, "elapsed > 180000 && result == 0") && !strings.Contains(step.Run, "elapsed > 240000"), "180-second step budget")
		require(step.Timeout == 5 && step.If == "" && !step.ContinueOnError, "shard step budget and failure propagation")
	}
	require(scenarios == 1, "exactly one Engine scenarios step")

	require(aggregate.Name == "Engine scenarios (Linux amd64)", "aggregate name")
	require(slices.Equal(aggregate.Needs, []string{shardJobID}), "aggregate needs engine-shard alone")
	require(aggregate.If == shardGuard, "aggregate guard: always() with the event guard")
	require(aggregate.Runner == "ubuntu-latest" && aggregate.Timeout == 1 && len(aggregate.Env) == 0, "aggregate runner and budget")
	require(!aggregate.ContinueOnError, "aggregate failure propagation")
	if len(aggregate.Steps) != 1 {
		require(false, "aggregate gate")
	} else {
		gate := aggregate.Steps[0]
		require(gate.Name == "Require every engine shard" && gate.Run == shardGate && gate.If == "" && !gate.ContinueOnError &&
			maps.Equal(gate.Env, map[string]string{"SHARD_RESULT": "${{ needs.engine-shard.result }}"}), "aggregate gate")
	}

	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return append(issues, err.Error())
	}
	switches, inputs := 0, 0
	inspectEngineSwitch(&node, "", func(path, value string) {
		require(path == enabled && value == "1", "leaked or invalid engine switch at "+path)
		switches++
	})
	require(switches == 1, "exactly one engine switch")
	walkFixtureScalars(&node, "", func(path, value string) {
		if strings.Contains(path, shardFeaturesVariable) || strings.Contains(value, shardFeaturesVariable) {
			require(strings.HasPrefix(path, "jobs."+shardJobID+".steps.") && strings.HasSuffix(path, ".env."+shardFeaturesVariable) && value == "${{ matrix.features }}", "step-local feature input at "+path)
			inputs++
		}
	})
	require(inputs == 1, "exactly one step-local feature input")
	return issues
}

func shardEnvironment(extra map[string]string) featureEnvironment {
	values := map[string]string{"ITOS_CONTAINER_ENGINE_TESTS": "1", "CI": "true", "GITHUB_ACTIONS": "true"}
	maps.Copy(values, extra)
	return readFeatureEnvironment(func(name string) string { return values[name] }, "linux", "amd64")
}

func readShardWorkflow(t *testing.T) ([]byte, shardWorkflow) {
	t.Helper()
	data, err := os.ReadFile("../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow shardWorkflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	return data, workflow
}

func TestEngineShards(t *testing.T) {
	data, workflow := readShardWorkflow(t)
	t.Run("workflow shards", func(t *testing.T) {
		for _, issue := range engineShardIssues(data) {
			t.Error(issue)
		}
	})
	t.Run("partition", func(t *testing.T) { testShardPartition(t, workflow.Jobs[shardJobID].Strategy.Matrix.Include) })
	t.Run("selector", testShardSelector)
	t.Run("broken workflows", func(t *testing.T) {
		if issues := engineShardIssues(data); len(issues) != 0 {
			t.Errorf("the unmodified workflow is not clean: %v", issues)
		}
		testBrokenShardWorkflows(t)
	})
}

// Each shard selects exactly its files' rows of the unsharded selection, and the
// three together select that selection once.
func testShardPartition(t *testing.T, include []shardEntry) {
	t.Helper()
	unsharded, err := selectScenarios("", ".", shardEnvironment(nil))
	if err != nil {
		t.Fatal(err)
	}
	if unsharded.count == 0 || unsharded.skip != "" {
		t.Fatalf("the unsharded engine tier selects nothing: count=%d skip=%q", unsharded.count, unsharded.skip)
	}
	rows, err := (&godog.TestSuite{Options: &godog.Options{Paths: []string{"."}, Tags: unsharded.filter}}).RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	count, ids := map[string]int{}, map[string][]string{}
	for _, feature := range rows {
		for _, pickle := range feature.Pickles {
			base := filepath.Base(pickle.Uri)
			count[base]++
			for _, tag := range pickle.Tags {
				if strings.HasPrefix(tag.Name, "@ID-") {
					ids[base] = append(ids[base], tag.Name)
				}
			}
		}
	}
	if len(include) == 0 {
		t.Fatal("the workflow lists no engine shards")
	}
	total, placed := 0, map[string]string{}
	var union []string
	byShard := map[string][]string{}
	for _, entry := range include {
		files := strings.Fields(entry.Features)
		selection, err := selectScenarios("", ".", shardEnvironment(map[string]string{shardFeaturesVariable: entry.Features}))
		if err != nil {
			t.Errorf("shard %s: %v", entry.Shard, err)
			continue
		}
		wantCount, wantIDs := 0, []string(nil)
		for _, file := range files {
			wantCount += count[file]
			wantIDs = append(wantIDs, ids[file]...)
			if other, again := placed[file]; again {
				t.Errorf("%s is in shards %s and %s", file, other, entry.Shard)
			}
			placed[file] = entry.Shard
		}
		slices.Sort(wantIDs)
		wantIDs = slices.Compact(wantIDs)
		if selection.count != wantCount || selection.skip != "" || !slices.Equal(selection.ids, wantIDs) {
			t.Errorf("shard %s selects %d row(s) %v (skip %q); its files hold %d row(s) %v of the unsharded selection",
				entry.Shard, selection.count, selection.ids, selection.skip, wantCount, wantIDs)
		}
		total += selection.count
		union = append(union, selection.ids...)
		byShard[entry.Shard] = selection.ids
	}
	if total != unsharded.count {
		t.Errorf("the shards select %d row(s) between them; the unsharded tier selects %d", total, unsharded.count)
	}
	slices.Sort(union)
	if !slices.Equal(union, unsharded.ids) {
		t.Errorf("the shards' IDs %v are not the unsharded tier's %v exactly once each", union, unsharded.ids)
	}
	engineFiles, err := engineFeatureFiles(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range engineFiles {
		if placed[file] == "" {
			t.Errorf("%s has an engine case and no shard", file)
		}
	}
	for _, id := range []string{"@ID-LIFECYCLE-09", "@ID-LIFECYCLE-13"} {
		if !slices.Contains(byShard["lifecycle"], id) {
			t.Errorf("%s is not in the lifecycle shard, which must hold the nested quiet schedule whole", id)
		}
	}
}

// A valid in-memory workflow lets every negative control run before the real
// workflow has its shards. The other shard holds every engine feature the first
// two do not, so the baseline stays valid as feature files are added.
func shardBaseline(t *testing.T) []byte {
	t.Helper()
	engine, err := engineFeatureFiles(".")
	if err != nil {
		t.Fatal(err)
	}
	other := slices.DeleteFunc(slices.Clone(engine), func(name string) bool { return name == "offline.feature" || name == "lifecycle.feature" })
	text := fmt.Sprintf(`jobs:
  engine-shard:
    name: Engine shard (${{ matrix.shard }})
    if: ${{ github.event_name != 'pull_request' || !github.event.pull_request.draft }}
    runs-on: ubuntu-latest
    timeout-minutes: 10
    strategy:
      fail-fast: false
      matrix:
        include:
          - {shard: offline, features: offline.feature}
          - {shard: lifecycle, features: lifecycle.feature}
          - {shard: other, features: "%s"}
    steps:
      - name: Engine scenarios
        timeout-minutes: 5
        env:
          ITOS_CONTAINER_ENGINE_TESTS: "1"
          ITOS_CONTAINER_ENGINE_FEATURES: ${{ matrix.features }}
        run: |
          if (( elapsed > 180000 && result == 0 )); then result=1; fi
  engine:
    name: Engine scenarios (Linux amd64)
    needs: [engine-shard]
    if: ${{ always() && (github.event_name != 'pull_request' || !github.event.pull_request.draft) }}
    runs-on: ubuntu-latest
    timeout-minutes: 1
    steps:
      - name: Require every engine shard
        env:
          SHARD_RESULT: ${{ needs.engine-shard.result }}
        run: test "$SHARD_RESULT" = success
`, strings.Join(other, " "))
	if issues := engineShardIssues([]byte(text)); len(issues) != 0 {
		t.Fatalf("invalid negative-control baseline: %v", issues)
	}
	return []byte(text)
}

func testBrokenShardWorkflows(t *testing.T) {
	baseline := shardBaseline(t)
	failFast := true
	editShard := func(change func(*shardJob)) func(*shardWorkflow) {
		return func(w *shardWorkflow) {
			job := w.Jobs[shardJobID]
			change(&job)
			w.Jobs[shardJobID] = job
		}
	}
	editAggregate := func(change func(*shardJob)) func(*shardWorkflow) {
		return func(w *shardWorkflow) {
			job := w.Jobs["engine"]
			change(&job)
			w.Jobs["engine"] = job
		}
	}
	replaceFeatures := func(shard, from, to string) func(*shardWorkflow) {
		return editShard(func(j *shardJob) {
			for i := range j.Strategy.Matrix.Include {
				if j.Strategy.Matrix.Include[i].Shard == shard {
					j.Strategy.Matrix.Include[i].Features = strings.Replace(j.Strategy.Matrix.Include[i].Features, from, to, 1)
				}
			}
		})
	}
	editGate := func(change func(*shardStep)) func(*shardWorkflow) {
		return editAggregate(func(j *shardJob) { change(&j.Steps[0]) })
	}
	for _, tc := range []struct {
		name, issue string
		breakIt     func(*shardWorkflow)
	}{
		{"a feature dropped from every shard", "feature in no shard", replaceFeatures("other", "hardlinks.feature", "")},
		{"a feature in two shards", "feature in two shards", replaceFeatures("lifecycle", "lifecycle.feature", "lifecycle.feature offline.feature")},
		{"a feature twice in one shard", "feature in two shards", replaceFeatures("offline", "offline.feature", "offline.feature offline.feature")},
		{"an unknown feature", "unknown engine feature", replaceFeatures("other", "doctor.feature", "doctor.feature absent.feature")},
		{"a path", "feature path", replaceFeatures("other", "doctor.feature", "features/doctor.feature")},
		{"an absolute path", "feature path", replaceFeatures("other", "doctor.feature", "/doctor.feature")},
		{"an empty shard", "empty shard", editShard(func(j *shardJob) { j.Strategy.Matrix.Include[2].Features = " " })},
		{"a fourth shard", "exactly three named shards", editShard(func(j *shardJob) {
			j.Strategy.Matrix.Include = append(j.Strategy.Matrix.Include, shardEntry{Shard: "extra", Features: "doctor.feature"})
		})},
		{"a renamed shard", "exactly three named shards", editShard(func(j *shardJob) { j.Strategy.Matrix.Include[0].Shard = "first" })},
		{"the aggregate without always()", "aggregate guard", editAggregate(func(j *shardJob) { j.If = eventGuard })},
		{"the aggregate without the event guard", "aggregate guard", editAggregate(func(j *shardJob) { j.If = "${{ always() }}" })},
		{"the aggregate needing more", "aggregate needs", editAggregate(func(j *shardJob) { j.Needs = []string{shardJobID, "platform"} })},
		{"the aggregate needing nothing", "aggregate needs", editAggregate(func(j *shardJob) { j.Needs = nil })},
		{"the aggregate needing another job", "aggregate needs", editAggregate(func(j *shardJob) { j.Needs = []string{"platform"} })},
		{"a gate accepting skipped", "aggregate gate", editGate(func(s *shardStep) { s.Run = `test "$SHARD_RESULT" = success || test "$SHARD_RESULT" = skipped` })},
		{"a gate refusing only failure", "aggregate gate", editGate(func(s *shardStep) { s.Run = `test "$SHARD_RESULT" != failure` })},
		{"a gate reading another result", "aggregate gate", editGate(func(s *shardStep) { s.Env = map[string]string{"SHARD_RESULT": "${{ needs.engine.result }}"} })},
		{"a skipped gate", "aggregate gate", editGate(func(s *shardStep) { s.If = "false" })},
		{"a gate that cannot fail", "aggregate gate", editGate(func(s *shardStep) { s.ContinueOnError = true })},
		{"continue-on-error on the shard job", "shard failure propagation", editShard(func(j *shardJob) { j.ContinueOnError = true })},
		{"continue-on-error on the aggregate", "aggregate failure propagation", editAggregate(func(j *shardJob) { j.ContinueOnError = true })},
		{"fail-fast true", "fail-fast false", editShard(func(j *shardJob) { j.Strategy.FailFast = &failFast })},
		{"fail-fast unset", "fail-fast false", editShard(func(j *shardJob) { j.Strategy.FailFast = nil })},
		{"the feature input moved to job env", "step-local feature input", editShard(func(j *shardJob) {
			j.Env = map[string]string{shardFeaturesVariable: "${{ matrix.features }}"}
			delete(j.Steps[0].Env, shardFeaturesVariable)
		})},
		{"the feature input missing", "step-local feature input", editShard(func(j *shardJob) { delete(j.Steps[0].Env, shardFeaturesVariable) })},
		{"the feature input from another matrix key", "step-local feature input", editShard(func(j *shardJob) { j.Steps[0].Env[shardFeaturesVariable] = "${{ matrix.shard }}" })},
		{"the feature input also on the aggregate", "step-local feature input", editAggregate(func(j *shardJob) {
			j.Steps[0].Env[shardFeaturesVariable] = "${{ matrix.features }}"
		})},
		{"a second engine switch", "engine switch", editAggregate(func(j *shardJob) { j.Steps[0].Env["ITOS_CONTAINER_ENGINE_TESTS"] = "1" })},
		{"the engine switch in job env", "engine switch", editShard(func(j *shardJob) { j.Env = map[string]string{"ITOS_CONTAINER_ENGINE_TESTS": "1"} })},
		{"the 240-second budget", "180-second step budget", editShard(func(j *shardJob) {
			j.Steps[0].Run = strings.ReplaceAll(j.Steps[0].Run, "180000", "240000")
		})},
		{"no budget", "180-second step budget", editShard(func(j *shardJob) { j.Steps[0].Run = "true" })},
		{"a shard guard lost", "shard event guard", editShard(func(j *shardJob) { j.If = "" })},
		{"a shard that needs another job", "shard failure propagation and independence", editShard(func(j *shardJob) { j.Needs = []string{"ci"} })},
		{"a shard on another runner", "shard native runner", editShard(func(j *shardJob) { j.Runner = "windows-latest" })},
		{"a longer shard job", "shard job budget", editShard(func(j *shardJob) { j.Timeout = 20 })},
	} {
		t.Run("reject "+tc.name, func(t *testing.T) {
			var broken shardWorkflow
			if err := yaml.Unmarshal(baseline, &broken); err != nil {
				t.Fatal(err)
			}
			tc.breakIt(&broken)
			fixture, err := yaml.Marshal(broken)
			if err != nil {
				t.Fatal(err)
			}
			if issues := strings.Join(engineShardIssues(fixture), "; "); !strings.Contains(issues, tc.issue) {
				t.Errorf("negative control missed %q: %s", tc.issue, issues)
			}
		})
	}
}
