package features

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// testPendingEngineFeatures holds T-77's reviewed pending list: only the names in
// pendingEngineFeatures may be absent from a shard's files, and then only until the file exists.
func testPendingEngineFeatures(t *testing.T) {
	if !maps.Equal(pendingEngineFeatures, map[string]pendingEngineFeature{
		"credentials-run.feature": {item: "slice-31", shard: "egress"},
		"claude-home.feature":     {item: "slice-36", shard: "other"},

		"credentials-run-native.feature": {item: "slice-31", shard: "offline"},
	}) {
		t.Fatalf("the pending list is reviewed to hold credentials-run.feature for slice-31 in egress and claude-home.feature for slice-36 in other and credentials-run-native.feature for slice-31 in offline alone: %v", pendingEngineFeatures)
	}
	for _, name := range slices.Sorted(maps.Keys(pendingEngineFeatures)) {
		t.Run(name, func(t *testing.T) { testPendingEngineFeature(t, name, pendingEngineFeatures[name].item) })
	}
	t.Run("a pending name is refused in any shard but its own", testPendingNameInWrongShard)
}

func testPendingNameInWrongShard(t *testing.T) {
	baseline := shardBaseline(t)
	for _, tc := range []struct{ name, from, to, issue string }{
		{"claude-home.feature in egress", "egress.feature", "egress.feature claude-home.feature", "only other may name it"},
		{"claude-home.feature in offline", "offline.feature", "offline.feature claude-home.feature", "only other may name it"},
		{"credentials-run-native.feature in egress", "egress.feature", "egress.feature credentials-run-native.feature", "only offline may name it"},
		{"credentials-run-native.feature in other", "doctor.feature", "doctor.feature credentials-run-native.feature", "only offline may name it"},
		{"credentials-run.feature in other", "doctor.feature", "doctor.feature credentials-run.feature", "only egress may name it"},
	} {
		var workflow engineWorkflow
		if err := yaml.Unmarshal(baseline, &workflow); err != nil {
			t.Fatal(err)
		}
		job := workflow.Jobs[shardJobID]
		for i := range job.Strategy.Matrix.Include {
			job.Strategy.Matrix.Include[i].Features = strings.Replace(job.Strategy.Matrix.Include[i].Features, tc.from, tc.to, 1)
		}
		workflow.Jobs[shardJobID] = job
		fixture, err := yaml.Marshal(workflow)
		if err != nil {
			t.Fatal(err)
		}
		if issues := strings.Join(engineShardIssues(fixture), "; "); !strings.Contains(issues, tc.issue) {
			t.Errorf("%s: want an issue containing %q, got %s", tc.name, tc.issue, issues)
		}
	}
}

func testPendingEngineFeature(t *testing.T, name, item string) {
	shardEnv := func(features string) featureEnvironment {
		return featureEnvironment{engine: "1", ci: "true", actions: "true", os: "linux", arch: "amd64", shard: features}
	}
	dir := writeFeatureFixture(t, tierFixture)
	t.Run("an absent name outside the pending map is refused", func(t *testing.T) {
		for _, names := range []string{"memory.feature absent.feature", "absent.feature", "memory.feature x-" + name} {
			if _, err := selectScenarios("", dir, shardEnv(names)); err == nil || !strings.Contains(err.Error(), "no feature file") {
				t.Errorf("%q: want a refusal naming the missing file, got %v", names, err)
			}
		}
	})
	t.Run("an absent pending name is skipped and logged", func(t *testing.T) {
		selected, err := selectScenarios("", dir, shardEnv("memory.feature "+name))
		if err != nil {
			t.Fatal(err)
		}
		if selected.count != 1 || selected.skip != "" || len(selected.pending) != 1 ||
			!strings.Contains(selected.pending[0], name) || !strings.Contains(selected.pending[0], "pending") || !strings.Contains(selected.pending[0], item) {
			t.Errorf("want one live row and one pending line naming %s and %s, got %+v", name, item, selected)
		}
		if !slices.Equal(selected.paths, []string{filepath.Join(dir, "memory.feature")}) {
			t.Errorf("the pending name reached the shard's paths: %v", selected.paths)
		}
		seen := executeMemory(t, dir, selected, nil)
		if !slices.Equal(seen.steps, []string{"engine"}) {
			t.Errorf("executed %+v; want the engine row", seen)
		}
	})
	t.Run("a shard of pending names alone is refused", func(t *testing.T) {
		if _, err := selectScenarios("", dir, shardEnv(name)); err == nil {
			t.Error("a shard with no file on this tree was accepted")
		}
	})
	t.Run("a pending name is refused outside the engine tier", func(t *testing.T) {
		env := shardEnv("memory.feature " + name)
		env.engine = ""
		if _, err := selectScenarios("", dir, env); err == nil {
			t.Error("a shard list was accepted outside the engine tier")
		}
	})
	t.Run("a present pending file is ordinary", func(t *testing.T) {
		present := writeFeatureFixture(t, tierFixture)
		engineless := "Feature: credentials\n  Scenario: plain\n    Given a plain row\n"
		if err := os.WriteFile(filepath.Join(present, name), []byte(engineless), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := selectScenarios("", present, shardEnv("memory.feature "+name)); err == nil || !strings.Contains(err.Error(), "@engine case") {
			t.Errorf("a present pending file with no @engine case: want a refusal, got %v", err)
		}
		engine := "@engine @ID-PENDING-99\nFeature: credentials\n  Scenario: live\n    Given a plain row\n"
		if err := os.WriteFile(filepath.Join(present, name), []byte(engine), 0600); err != nil {
			t.Fatal(err)
		}
		selected, err := selectScenarios("", present, shardEnv("memory.feature "+name))
		if err != nil {
			t.Fatal(err)
		}
		if len(selected.pending) != 0 || selected.count != 2 || !slices.Contains(selected.ids, "@ID-PENDING-99") {
			t.Errorf("a present file is ordinary: want no pending line and its row selected, got %+v", selected)
		}
	})
}
