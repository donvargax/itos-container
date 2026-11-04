package features

import (
	"os"
	"slices"
	"testing"

	"go.yaml.in/yaml/v3"
)

type engineWorkflow struct {
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Name    string         `yaml:"name"`
	If      string         `yaml:"if"`
	Needs   []string       `yaml:"needs"`
	Runner  string         `yaml:"runs-on"`
	Timeout int            `yaml:"timeout-minutes"`
	Steps   []workflowStep `yaml:"steps"`
}

type workflowStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	Env  map[string]string `yaml:"env"`
	With map[string]string `yaml:"with"`
}

func TestEngineWorkflow(t *testing.T) {
	data, err := os.ReadFile("../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow engineWorkflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	t.Run("required engine job", func(t *testing.T) {
		engine, ok := workflow.Jobs["engine"]
		if !ok {
			t.Fatal("missing engine job")
		}
		if engine.Name != "Engine scenarios (Linux amd64)" || engine.Runner != "ubuntu-latest" {
			t.Errorf("wrong engine check/runner: %+v", engine)
		}
	})
	t.Run("required ci dependency", func(t *testing.T) {
		if !slices.Contains(workflow.Jobs["ci"].Needs, "engine") {
			t.Fatal("ci does not depend on engine")
		}
	})
}
