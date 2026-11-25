package features

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/cucumber/godog"
	"go.yaml.in/yaml/v3"
)

// The engine tier runs as four shards (T-45, T-59), each a native runner and daemon of
// whole feature files. Nothing here runs an engine, Docker or a product command:
// the workflow is parsed, the real feature files are selected as data, and fixture
// features run with memory-only steps, as the schedule regressions do.

// shardEntry is one include of the shard job's matrix: the shard's name and its
// whitespace-separated feature file basenames.
type shardEntry struct {
	Shard    string `yaml:"shard"`
	Features string `yaml:"features"`
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

// shardOtherFiles is the other shard's fixed set of files (T-59): every engine feature
// this tree keeps outside offline, lifecycle and egress.
var shardOtherFiles = []string{
	"anchors.feature", "checkout-overlap.feature", "claude-home.feature", "doctor.feature",
	"exec.feature", "hardlinks.feature", "recovery.feature", "run_terminal.feature", "session.feature",
	"sockets.feature",
}

// egressShardFiles is the egress shard's complete pinned set: credentials-run.feature
// (T-77's placement, held whether or not the file is on this tree yet: see
// pendingEngineFeatures), egress.feature, plus egress_lifecycle.feature while the engine
// inventory has it, so slice-28's file joins this shard when it lands on this tree
// (T-59's placement). The list is sorted, as shardFileIssue compares it. The pin is this explicit
// allowlist, never the inventory's leftover: a new engine file is not allowed to hide
// in egress (slice-37's run_terminal.feature belongs in other, never here) and fails
// visibly until a shard's set or a new shard takes it.
func egressShardFiles(engine []string) []string {
	files := []string{"credentials-run.feature", "egress.feature"}
	if slices.Contains(engine, "egress_lifecycle.feature") {
		files = append(files, "egress_lifecycle.feature")
	}
	return files
}

// The offline and lifecycle shards each pin their complete set of feature files.
var (
	shardOfflineFiles   = []string{"credentials-run-native.feature", "harness-home.feature", "offline.feature"}
	shardLifecycleFiles = []string{"lifecycle.feature"}
)

// shardFileIssue reports how the shard's parsed files got differ from its pinned
// set want, or "" when got is that set, whatever the list's order or whitespace.
func shardFileIssue(shard string, got, want []string) string {
	slices.Sort(got)
	if slices.Equal(got, want) {
		return ""
	}
	return "shard " + shard + " files: the pinned set is " + strings.Join(want, " ") +
		", whatever the list's order or whitespace"
}

// egressShardIssue reports how the egress shard's parsed files got differ from its
// pinned set for the engine inventory engine, or "" when got is that set, whatever
// the list's order or whitespace.
func egressShardIssue(got, engine []string) string {
	return shardFileIssue("egress", got, egressShardFiles(engine))
}

// shardReportLine is the one-line shard report of the real partition: every file the
// entry names, on one line, whatever whitespace or line breaks its YAML value used.
func shardReportLine(shard string, count, ids int, files []string) string {
	return fmt.Sprintf("shard %s: %d row(s), %d ID(s) in %s", shard, count, ids, strings.Join(files, " "))
}

// realEngineFeatures is engineFeatureFiles over this folder, read once.
var realEngineFeatures = sync.OnceValues(func() ([]string, error) { return engineFeatureFiles(".") })

// engineShardIssues holds the sharded engine job and its aggregate to T-45's
// contract, against the real feature files for the partition of the features.
func engineShardIssues(data []byte) []string {
	var workflow engineWorkflow
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
	require(shard.Timeout == 15, "shard job budget")
	require(!shard.ContinueOnError && len(shard.Needs) == 0, "shard failure propagation and independence")
	require(shard.Strategy.FailFast != nil && !*shard.Strategy.FailFast, "fail-fast false")
	require(len(shard.Env) == 0, "step-local feature input and engine switch")

	include := shard.Strategy.Matrix.Include
	var names []string
	for _, entry := range include {
		names = append(names, entry.Shard)
	}
	slices.Sort(names)
	require(slices.Equal(names, []string{"egress", "lifecycle", "offline", "other"}), "exactly four named shards: egress, lifecycle, offline and other")
	want, err := realEngineFeatures()
	if err != nil {
		issues = append(issues, "feature inventory: "+err.Error())
	}
	placed := map[string]int{}
	for _, entry := range include {
		fields := strings.Fields(entry.Features)
		require(len(fields) > 0, "empty shard "+entry.Shard)
		for _, name := range fields {
			require(name == filepath.Base(name) && !strings.ContainsAny(name, `/\:`) && strings.HasSuffix(name, ".feature"), "feature path "+strconv.Quote(name)+" in shard "+entry.Shard)
			held, pending := pendingEngineFeatures[name]
			if pending && entry.Shard != held.shard {
				require(false, "pending feature "+strconv.Quote(name)+" in shard "+entry.Shard+": only "+held.shard+" may name it")
			} else {
				require(err != nil || slices.Contains(want, name) || pending, "unknown engine feature "+strconv.Quote(name))
			}
			placed[name]++
		}
	}
	for _, name := range want {
		require(placed[name] != 0, "feature in no shard: "+name)
	}
	for name, count := range placed {
		require(count < 2, "feature in two shards: "+name)
	}
	for _, entry := range include {
		var pinned []string
		switch entry.Shard {
		case "offline":
			pinned = shardOfflineFiles
		case "lifecycle":
			pinned = shardLifecycleFiles
		case "other":
			pinned = shardOtherFiles
		case "egress":
			require(err == nil, "shard egress files: the pin needs the engine inventory")
			pinned = egressShardFiles(want)
		default:
			continue
		}
		if issue := shardFileIssue(entry.Shard, strings.Fields(entry.Features), pinned); issue != "" {
			issues = append(issues, issue)
		}
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
		require(strings.Contains(step.Run, "(( elapsed > 180000 ))") && strings.Contains(step.Run, "(( elapsed > 600000 ))") && strings.Contains(step.Run, "(( elapsed > 600000 && result == 0 ))") && !strings.Contains(step.Run, "elapsed > 180000 && result == 0") && !strings.Contains(step.Run, "elapsed > 240000"), "soft step budget and hang guard")
		require(step.Timeout == 11 && step.If == "" && !step.ContinueOnError, "shard step budget and failure propagation")
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

func readShardWorkflow(t *testing.T) ([]byte, engineWorkflow) {
	t.Helper()
	data, err := os.ReadFile("../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow engineWorkflow
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
	t.Run("home red inventory", func(t *testing.T) {
		ids := []string{"@ID-HOME-13", "@ID-HOME-14", "@ID-HOME-15", "@ID-HOME-23"}
		for _, entry := range workflow.Jobs[shardJobID].Strategy.Matrix.Include {
			selection, err := selectScenarios("", ".", shardEnvironment(map[string]string{
				shardFeaturesVariable: entry.Features,
				"GITHUB_EVENT_NAME":   "workflow_dispatch",
				redVariable:           strings.Join(ids, " "),
			}))
			if err != nil {
				t.Fatal(err)
			}
			if entry.Shard == "offline" {
				if selection.count != 9 || selection.skip != "" || !slices.Equal(selection.ids, ids) {
					t.Errorf("offline must select all nine home engine rows and four IDs: count=%d ids=%v skip=%q", selection.count, selection.ids, selection.skip)
				}
			} else if selection.count != 0 || !strings.Contains(selection.skip, "other engine shard") {
				t.Errorf("shard %s must leave home rows to offline: count=%d skip=%q", entry.Shard, selection.count, selection.skip)
			}
		}
	})
	t.Run("selector", testShardSelector)
	t.Run("multiline egress report", func(t *testing.T) {
		// A YAML literal block names the same two files a space-separated list does, and
		// the report still lists them all on one line. This control runs before slice-28
		// lands: it formats an in-memory entry and selects nothing.
		const multiline = `jobs:
  engine-shard:
    strategy:
      matrix:
        include:
          - shard: egress
            features: |
              egress_lifecycle.feature
              egress.feature
`
		var workflow engineWorkflow
		if err := yaml.Unmarshal([]byte(multiline), &workflow); err != nil {
			t.Fatal(err)
		}
		include := workflow.Jobs[shardJobID].Strategy.Matrix.Include
		if len(include) != 1 || include[0].Shard != "egress" {
			t.Fatalf("the multiline entry did not parse as one egress shard: %v", include)
		}
		line := shardReportLine(include[0].Shard, 27, 27, strings.Fields(include[0].Features))
		if strings.Contains(line, "\n") {
			t.Errorf("the report is not one line: %q", line)
		}
		if !strings.HasSuffix(line, " in egress_lifecycle.feature egress.feature") {
			t.Errorf("the report does not list every file of the literal block: %q", line)
		}
	})
	t.Run("not-yet-listed egress file", func(t *testing.T) {
		// A new engine file must have an explicit shard assignment.
		const unassigned = "unassigned.feature"
		engine, err := realEngineFeatures()
		if err != nil {
			t.Fatal(err)
		}
		engine = append(slices.Clone(engine), unassigned)
		issue := egressShardIssue([]string{"egress.feature", unassigned}, engine)
		if issue == "" {
			t.Error("the egress pin allowed a not-yet-listed engine file")
		}
		if strings.Contains(issue, unassigned) {
			t.Errorf("the pinned set includes the unrelated file: %q", issue)
		}
	})
	t.Run("broken workflows", func(t *testing.T) {
		if issues := engineShardIssues(data); len(issues) != 0 {
			t.Errorf("the unmodified workflow is not clean: %v", issues)
		}
		testBrokenShardWorkflows(t)
	})
}

// Each shard selects exactly its files' rows of the unsharded selection, and the
// four together select that selection once.
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
	engineFiles, err := realEngineFeatures()
	if err != nil {
		t.Fatal(err)
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
		t.Log(shardReportLine(entry.Shard, selection.count, len(selection.ids), files))
		for _, line := range selection.pending {
			t.Log(line)
		}
		for _, file := range files {
			if held, pending := pendingEngineFeatures[file]; pending && !slices.Contains(engineFiles, file) && !slices.ContainsFunc(selection.pending, func(line string) bool { return strings.Contains(line, file) && strings.Contains(line, held.item) }) {
				t.Errorf("shard %s does not log %s as pending for %s", entry.Shard, file, held.item)
			}
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
// workflow has its shards. The offline, lifecycle and other shards hold their fixed
// file sets and the egress shard its pinned allowlist, so a new engine file fails the
// baseline visibly until a shard's set or a new shard takes it.
func shardBaseline(t *testing.T) []byte {
	t.Helper()
	engine, err := realEngineFeatures()
	if err != nil {
		t.Fatal(err)
	}
	other := slices.Clone(shardOtherFiles)
	egress := egressShardFiles(engine)
	text := fmt.Sprintf(`jobs:
  engine-shard:
    name: Engine shard (${{ matrix.shard }})
    if: ${{ github.event_name != 'pull_request' || !github.event.pull_request.draft }}
    runs-on: ubuntu-latest
    timeout-minutes: 15
    strategy:
      fail-fast: false
      matrix:
        include:
          - {shard: offline, features: "offline.feature harness-home.feature credentials-run-native.feature"}
          - {shard: lifecycle, features: lifecycle.feature}
          - {shard: other, features: "%s"}
          - {shard: egress, features: "%s"}
    steps:
      - name: Engine scenarios
        timeout-minutes: 11
        env:
          ITOS_CONTAINER_ENGINE_TESTS: "1"
          ITOS_CONTAINER_ENGINE_FEATURES: ${{ matrix.features }}
        run: |
          if (( elapsed > 180000 )); then echo warn; fi
          if (( elapsed > 600000 )); then echo error; fi
          if (( elapsed > 600000 && result == 0 )); then result=1; fi
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
`, strings.Join(other, " "), strings.Join(egress, " "))
	if issues := engineShardIssues([]byte(text)); len(issues) != 0 {
		t.Fatalf("invalid negative-control baseline: %v", issues)
	}
	return []byte(text)
}

func testBrokenShardWorkflows(t *testing.T) {
	baseline := shardBaseline(t)
	failFast := true
	editShard := func(change func(*workflowJob)) func(*engineWorkflow) {
		return func(w *engineWorkflow) {
			job := w.Jobs[shardJobID]
			change(&job)
			w.Jobs[shardJobID] = job
		}
	}
	editAggregate := func(change func(*workflowJob)) func(*engineWorkflow) {
		return func(w *engineWorkflow) {
			job := w.Jobs["engine"]
			change(&job)
			w.Jobs["engine"] = job
		}
	}
	replaceFeatures := func(shard, from, to string) func(*engineWorkflow) {
		return editShard(func(j *workflowJob) {
			for i := range j.Strategy.Matrix.Include {
				if j.Strategy.Matrix.Include[i].Shard == shard {
					j.Strategy.Matrix.Include[i].Features = strings.Replace(j.Strategy.Matrix.Include[i].Features, from, to, 1)
				}
			}
		})
	}
	editGate := func(change func(*workflowStep)) func(*engineWorkflow) {
		return editAggregate(func(j *workflowJob) { change(&j.Steps[0]) })
	}
	for _, tc := range []struct {
		name, issue string
		breakIt     func(*engineWorkflow)
	}{
		{"a feature dropped from every shard", "feature in no shard", replaceFeatures("other", "hardlinks.feature", "")},
		{"a feature in two shards", "feature in two shards", replaceFeatures("lifecycle", "lifecycle.feature", "lifecycle.feature offline.feature")},
		{"a feature twice in one shard", "feature in two shards", replaceFeatures("offline", "offline.feature", "offline.feature offline.feature")},
		{"an unknown feature", "unknown engine feature", replaceFeatures("other", "doctor.feature", "doctor.feature absent.feature")},
		{"a pending file in the other shard", "pending feature", replaceFeatures("other", "doctor.feature", "doctor.feature credentials-run.feature")},
		{"the pending file missing from egress", "shard egress files", replaceFeatures("egress", "credentials-run.feature", "")},
		{"a path", "feature path", replaceFeatures("other", "doctor.feature", "features/doctor.feature")},
		{"an absolute path", "feature path", replaceFeatures("other", "doctor.feature", "/doctor.feature")},
		{"an empty shard", "empty shard", editShard(func(j *workflowJob) { j.Strategy.Matrix.Include[2].Features = " " })},
		{"the old three-shard layout", "exactly four named shards", editShard(func(j *workflowJob) {
			j.Strategy.Matrix.Include = j.Strategy.Matrix.Include[:3]
		})},
		{"a fifth shard", "exactly four named shards", editShard(func(j *workflowJob) {
			j.Strategy.Matrix.Include = append(j.Strategy.Matrix.Include, shardEntry{Shard: "extra", Features: "doctor.feature"})
		})},
		{"a renamed shard", "exactly four named shards", editShard(func(j *workflowJob) { j.Strategy.Matrix.Include[0].Shard = "first" })},
		{"a file swapped between egress and other", "shard egress files", func(w *engineWorkflow) {
			replaceFeatures("egress", "egress.feature", "anchors.feature")(w)
			replaceFeatures("other", "anchors.feature", "egress.feature")(w)
		}},
		{"an unrelated file in the egress shard", "shard egress files", func(w *engineWorkflow) {
			replaceFeatures("egress", "egress.feature", "egress.feature recovery.feature")(w)
			replaceFeatures("other", "recovery.feature", "")(w)
		}},
		{"a file moved from other to offline", "shard offline files", func(w *engineWorkflow) {
			replaceFeatures("other", "hardlinks.feature", "")(w)
			replaceFeatures("offline", "offline.feature", "offline.feature hardlinks.feature")(w)
		}},
		{"a file moved from other to lifecycle", "shard lifecycle files", func(w *engineWorkflow) {
			replaceFeatures("other", "hardlinks.feature", "")(w)
			replaceFeatures("lifecycle", "lifecycle.feature", "lifecycle.feature hardlinks.feature")(w)
		}},
		{"the aggregate without always()", "aggregate guard", editAggregate(func(j *workflowJob) { j.If = eventGuard })},
		{"the aggregate without the event guard", "aggregate guard", editAggregate(func(j *workflowJob) { j.If = "${{ always() }}" })},
		{"the aggregate needing more", "aggregate needs", editAggregate(func(j *workflowJob) { j.Needs = []string{shardJobID, "platform"} })},
		{"the aggregate needing nothing", "aggregate needs", editAggregate(func(j *workflowJob) { j.Needs = nil })},
		{"the aggregate needing another job", "aggregate needs", editAggregate(func(j *workflowJob) { j.Needs = []string{"platform"} })},
		{"a gate accepting skipped", "aggregate gate", editGate(func(s *workflowStep) { s.Run = `test "$SHARD_RESULT" = success || test "$SHARD_RESULT" = skipped` })},
		{"a gate refusing only failure", "aggregate gate", editGate(func(s *workflowStep) { s.Run = `test "$SHARD_RESULT" != failure` })},
		{"a gate reading another result", "aggregate gate", editGate(func(s *workflowStep) { s.Env = map[string]string{"SHARD_RESULT": "${{ needs.engine.result }}"} })},
		{"a skipped gate", "aggregate gate", editGate(func(s *workflowStep) { s.If = "false" })},
		{"a gate that cannot fail", "aggregate gate", editGate(func(s *workflowStep) { s.ContinueOnError = true })},
		{"continue-on-error on the shard job", "shard failure propagation", editShard(func(j *workflowJob) { j.ContinueOnError = true })},
		{"continue-on-error on the aggregate", "aggregate failure propagation", editAggregate(func(j *workflowJob) { j.ContinueOnError = true })},
		{"fail-fast true", "fail-fast false", editShard(func(j *workflowJob) { j.Strategy.FailFast = &failFast })},
		{"fail-fast unset", "fail-fast false", editShard(func(j *workflowJob) { j.Strategy.FailFast = nil })},
		{"the feature input moved to job env", "step-local feature input", editShard(func(j *workflowJob) {
			j.Env = map[string]string{shardFeaturesVariable: "${{ matrix.features }}"}
			delete(j.Steps[0].Env, shardFeaturesVariable)
		})},
		{"the feature input missing", "step-local feature input", editShard(func(j *workflowJob) { delete(j.Steps[0].Env, shardFeaturesVariable) })},
		{"the feature input from another matrix key", "step-local feature input", editShard(func(j *workflowJob) { j.Steps[0].Env[shardFeaturesVariable] = "${{ matrix.shard }}" })},
		{"the feature input also on the aggregate", "step-local feature input", editAggregate(func(j *workflowJob) {
			j.Steps[0].Env[shardFeaturesVariable] = "${{ matrix.features }}"
		})},
		{"a second engine switch", "engine switch", editAggregate(func(j *workflowJob) { j.Steps[0].Env["ITOS_CONTAINER_ENGINE_TESTS"] = "1" })},
		{"the engine switch in job env", "engine switch", editShard(func(j *workflowJob) { j.Env = map[string]string{"ITOS_CONTAINER_ENGINE_TESTS": "1"} })},
		{"the guard raised above 600 seconds", "soft step budget and hang guard", editShard(func(j *workflowJob) {
			j.Steps[0].Run = strings.ReplaceAll(j.Steps[0].Run, "600000", "900000")
		})},
		{"the guard removed", "soft step budget and hang guard", editShard(func(j *workflowJob) {
			j.Steps[0].Run = strings.ReplaceAll(j.Steps[0].Run, "(( elapsed > 600000 && result == 0 ))", "false")
		})},
		{"the warning made a failure", "soft step budget and hang guard", editShard(func(j *workflowJob) {
			j.Steps[0].Run = strings.ReplaceAll(j.Steps[0].Run, "(( elapsed > 180000 ))", "(( elapsed > 180000 && result == 0 ))")
		})},
		{"a step limit not above the guard", "shard step budget and failure propagation", editShard(func(j *workflowJob) { j.Steps[0].Timeout = 10 })},
		{"no budget", "soft step budget and hang guard", editShard(func(j *workflowJob) { j.Steps[0].Run = "true" })},
		{"a shard guard lost", "shard event guard", editShard(func(j *workflowJob) { j.If = "" })},
		{"a shard that needs another job", "shard failure propagation and independence", editShard(func(j *workflowJob) { j.Needs = []string{"ci"} })},
		{"a shard on another runner", "shard native runner", editShard(func(j *workflowJob) { j.Runner = "windows-latest" })},
		{"a longer shard job", "shard job budget", editShard(func(j *workflowJob) { j.Timeout = 20 })},
		{"a shorter shard job", "shard job budget", editShard(func(j *workflowJob) { j.Timeout = 10 })},
	} {
		t.Run("reject "+tc.name, func(t *testing.T) {
			var broken engineWorkflow
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
