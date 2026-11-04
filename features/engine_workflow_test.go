package features

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type engineWorkflow struct {
	On          map[string]yaml.Node   `yaml:"on"`
	Permissions map[string]string      `yaml:"permissions"`
	Env         map[string]string      `yaml:"env"`
	Jobs        map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Env             map[string]string `yaml:"env"`
	Container       any               `yaml:"container"`
	Services        map[string]any    `yaml:"services"`
	ContinueOnError bool              `yaml:"continue-on-error"`
	Strategy        struct {
		Matrix struct {
			OS []string `yaml:"os"`
		} `yaml:"matrix"`
	} `yaml:"strategy"`
	Name    string         `yaml:"name"`
	If      string         `yaml:"if"`
	Needs   []string       `yaml:"needs"`
	Runner  string         `yaml:"runs-on"`
	Timeout int            `yaml:"timeout-minutes"`
	Steps   []workflowStep `yaml:"steps"`
}

type workflowStep struct {
	ID              string            `yaml:"id"`
	If              string            `yaml:"if"`
	Shell           string            `yaml:"shell"`
	Timeout         int               `yaml:"timeout-minutes"`
	ContinueOnError bool              `yaml:"continue-on-error"`
	Name            string            `yaml:"name"`
	Uses            string            `yaml:"uses"`
	Run             string            `yaml:"run"`
	Env             map[string]string `yaml:"env"`
	With            map[string]string `yaml:"with"`
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
	t.Run("complete engine contract", func(t *testing.T) {
		for _, issue := range engineWorkflowIssues(data) {
			t.Error(issue)
		}
	})
	for _, tc := range []struct {
		name, issue   string
		breakWorkflow func(*engineWorkflow)
	}{
		{"lost dependency", "ci dependency", func(w *engineWorkflow) { j := w.Jobs["ci"]; j.Needs = nil; w.Jobs["ci"] = j }},
		{"ignored preflight failure", "preflight", func(w *engineWorkflow) {
			changeStep(w, "engine", "Docker server preflight", func(s *workflowStep) { s.ContinueOnError = true })
		}},
		{"preflight success fallback", "preflight", func(w *engineWorkflow) {
			changeStep(w, "engine", "Docker server preflight", func(s *workflowStep) { s.Run += "true\n" })
		}},
		{"preflight remote endpoint", "preflight", func(w *engineWorkflow) {
			changeStep(w, "engine", "Docker server preflight", func(s *workflowStep) {
				s.Run = strings.ReplaceAll(s.Run, "unix:///var/run/docker.sock", "tcp://remote:2375")
			})
		}},
		{"unbounded preflight", "preflight", func(w *engineWorkflow) {
			changeStep(w, "engine", "Docker server preflight", func(s *workflowStep) { s.Run = strings.ReplaceAll(s.Run, "29s", "60s") })
		}},
		{"leaked switch", "switch", func(w *engineWorkflow) { w.Env["ITOS_CONTAINER_ENGINE_TESTS"] = "1" }},
		{"leaked switch in proof command", "switch", func(w *engineWorkflow) {
			j := w.Jobs["ci"]
			j.Steps[len(j.Steps)-1].Run = "ITOS_CONTAINER_ENGINE_TESTS=1 itos ci run"
			w.Jobs["ci"] = j
		}},
		{"lost always guard", "ci guard", func(w *engineWorkflow) { j := w.Jobs["ci"]; j.If = eventGuard; w.Jobs["ci"] = j }},
		{"ignored engine result", "result propagation", func(w *engineWorkflow) {
			changeStep(w, "ci", "Require engine success", func(s *workflowStep) { s.Run = "true" })
		}},
		{"skipped result guard", "result propagation", func(w *engineWorkflow) {
			changeStep(w, "ci", "Require engine success", func(s *workflowStep) { s.If = "false" })
		}},
		{"wrong engine head", "checkout head", func(w *engineWorkflow) { j := w.Jobs["engine"]; j.Steps[0].With["ref"] = "main"; w.Jobs["engine"] = j }},
		{"partial runtime selection", "complete invocation", func(w *engineWorkflow) {
			changeStep(w, "engine", "Engine scenarios", func(s *workflowStep) { s.Run += " -scenarios=@smoke" })
		}},
		{"skipped runtime step", "complete invocation", func(w *engineWorkflow) {
			changeStep(w, "engine", "Engine scenarios", func(s *workflowStep) { s.If = "false" })
		}},
		{"wrong platform", "native runner", func(w *engineWorkflow) { j := w.Jobs["engine"]; j.Runner = "windows-latest"; w.Jobs["engine"] = j }},
		{"job container", "native runner", func(w *engineWorkflow) { j := w.Jobs["engine"]; j.Container = "image"; w.Jobs["engine"] = j }},
		{"missing runtime budget", "complete invocation", func(w *engineWorkflow) {
			changeStep(w, "engine", "Engine scenarios", func(s *workflowStep) { s.Timeout = 0 })
		}},
	} {
		t.Run("reject "+tc.name, func(t *testing.T) {
			var broken engineWorkflow
			if err := yaml.Unmarshal(data, &broken); err != nil {
				t.Fatal(err)
			}
			tc.breakWorkflow(&broken)
			fixture, err := yaml.Marshal(broken)
			if err != nil {
				t.Fatal(err)
			}
			issues := strings.Join(engineWorkflowIssues(fixture), "; ")
			if !strings.Contains(issues, tc.issue) {
				t.Errorf("negative control missed %q: %s", tc.issue, issues)
			}
		})
	}

}

const eventGuard = "${{ github.event_name != 'pull_request' || !github.event.pull_request.draft }}"
const ciGuard = "${{ always() && (github.event_name != 'pull_request' || !github.event.pull_request.draft) }}"
const engineInvocation = "go test ./features -count=1 -v -timeout=5m -run '^TestFeatures$'"

func changeStep(w *engineWorkflow, job, name string, change func(*workflowStep)) {
	j := w.Jobs[job]
	for i := range j.Steps {
		if j.Steps[i].Name == name {
			change(&j.Steps[i])
		}
	}
	w.Jobs[job] = j
}

func engineWorkflowIssues(data []byte) []string {
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
	require(len(workflow.Jobs) == 5, "preserve four ordinary jobs and add exactly one engine job")
	require(len(workflow.On) == 3 && workflow.On["pull_request"].Kind != 0 && workflow.On["push"].Kind != 0 && workflow.On["workflow_dispatch"].Kind != 0, "event triggers")
	var push struct {
		Branches []string `yaml:"branches"`
	}
	pushNode := workflow.On["push"]
	require(pushNode.Decode(&push) == nil && slices.Equal(push.Branches, []string{"main"}), "main push trigger")
	var pr struct {
		Types []string `yaml:"types"`
	}
	prNode := workflow.On["pull_request"]
	require(prNode.Decode(&pr) == nil && slices.Equal(pr.Types, []string{"opened", "synchronize", "reopened", "ready_for_review"}), "PR events")
	require(len(workflow.Permissions) == 2 && workflow.Permissions["contents"] == "read" && workflow.Permissions["actions"] == "read", "preserve read permissions")
	require(workflow.Env["TO"] == "${{ github.event.pull_request.head.sha || github.sha }}", "tested head")
	ci, engine := workflow.Jobs["ci"], workflow.Jobs["engine"]
	require(slices.Contains(ci.Needs, "engine"), "ci dependency")
	require(ci.If == ciGuard, "ci guard must run after failed or skipped dependencies")
	require(!ci.ContinueOnError, "ci result propagation")
	if len(ci.Steps) == 0 {
		require(false, "result propagation")
	} else {
		gate := ci.Steps[0]
		require(gate.Name == "Require engine success" && gate.Env["ENGINE_RESULT"] == "${{ needs.engine.result }}" && gate.Run == `test "$ENGINE_RESULT" = success` && !gate.ContinueOnError && gate.If == "", "result propagation")
	}
	runs := map[string]bool{}
	for _, step := range ci.Steps {
		runs[step.Run] = true
	}
	require(runs[`itos verify "$FROM" "$TO"`] && runs[`itos ci run "$FROM" "$TO"`], "preserve verify/proof commands")
	hasRange := false
	for _, step := range ci.Steps {
		if step.Name == "Range" && strings.Contains(step.Run, `itos ci range --json --head "$TO" --base "$BASE"`) {
			hasRange = true
		}
	}
	require(hasRange, "preserve range command")
	require(engine.Name == "Engine scenarios (Linux amd64)", "stable engine check name")
	require(engine.If == eventGuard, "engine event/draft guard")
	require(engine.Runner == "ubuntu-latest" && engine.Container == nil && len(engine.Services) == 0 && len(engine.Strategy.Matrix.OS) == 0, "native runner")
	require(engine.Timeout == 10 && !engine.ContinueOnError && len(engine.Needs) == 0, "engine job budget and independence")
	require(len(engine.Env) == 0, "engine switch must be step-local")
	enabledPath := ""
	require(len(engine.Steps) == 6, "engine must have checkout, setup-go, preflight, preparation, scenarios and cleanup only")
	if len(engine.Steps) >= 3 {
		checkout, setup, preflight := engine.Steps[0], engine.Steps[1], engine.Steps[2]
		require(checkout.Uses == "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1" && checkout.With["ref"] == "${{ env.TO }}", "checkout head and pin")
		require(setup.Uses == "actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e" && setup.With["go-version-file"] == "go.mod" && setup.With["cache-dependency-path"] == "go.sum", "setup-go toolchain and pin")
		require(preflight.Name == "Docker server preflight" && preflight.Shell == "bash" && !preflight.ContinueOnError && preflight.If == "" && validEnginePreflight(preflight.Run), "bounded read-only local-server preflight without ignored failures")
	}
	for i, run := range engine.Steps {
		if run.Name == "Engine scenarios" {
			require(run.Run == engineInvocation && run.Timeout == 5 && run.If == "" && !run.ContinueOnError && validFixtureEnvironment(run.Env), "complete invocation, fixture outputs and step budget")
			enabledPath = "jobs.engine.steps." + strconv.Itoa(i) + ".env.ITOS_CONTAINER_ENGINE_TESTS"
		}
	}
	require(enabledPath != "", "complete invocation missing")
	issues = append(issues, engineFixtureWorkflowIssues(data)...)
	for _, name := range []string{"platform", "platform-macos", "platform-windows"} {
		job, ok := workflow.Jobs[name]
		require(ok, "preserve "+name)
		guard := "${{ github.event_name == 'workflow_dispatch' }}"
		if name == "platform" {
			guard = eventGuard
			require(slices.Equal(job.Strategy.Matrix.OS, []string{"ubuntu-latest"}), "preserve Linux matrix")
		} else {
			runner := strings.TrimPrefix(name, "platform-") + "-latest"
			require(job.Runner == runner, "preserve portability runner")
		}
		require(job.If == guard, "preserve ordinary event guard")
		found := false
		for _, step := range job.Steps {
			if step.Run == "go test ./... -count=1" && step.Env["RAPID_CHECKS"] == "1" {
				found = true
			}
		}
		require(found, "preserve ordinary full suite")
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return append(issues, err.Error())
	}
	enabled := 0
	inspectEngineSwitch(&node, "", func(path, value string) {
		require(path == enabledPath && value == "1", "leaked or invalid engine switch at "+path)
		enabled++
	})
	require(enabled == 1, "exactly one engine switch")
	return issues
}

// Validate each command of this deliberately small preflight. An exact command
// allowlist prevents shell fallbacks, extra engine operations and unbounded calls.
// The workflow fixture is parsed as YAML first; nothing executes a Docker CLI.
func validEnginePreflight(script string) bool {
	lines := strings.Split(strings.TrimSpace(script), "\n")
	required := []string{
		`timeout --signal=TERM --kill-after=1s 29s bash -euo pipefail <<'PREFLIGHT'`,
		`command -v docker >/dev/null`,
		`test -S /var/run/docker.sock`,
		`server=$(env -u DOCKER_CONTEXT -u DOCKER_HOST -u DOCKER_TLS_VERIFY -u DOCKER_CERT_PATH -u DOCKER_API_VERSION docker --host unix:///var/run/docker.sock version --format '{{json .Server}}')`,
		`jq -e '.Os == "linux" and .Arch == "amd64"' <<<"$server" >/dev/null`,
		`jq -c --arg commit "$TO" '{version: .Version, apiVersion: .ApiVersion, os: .Os, architecture: .Arch, commit: $commit}' <<<"$server"`,
		`PREFLIGHT`,
	}
	return slices.Equal(lines, required)
}

// Traverse even unmodeled YAML fields (defaults, an extra step, etc.), so moving
// the switch outside its one allowed step cannot hide from the regression test.
func inspectEngineSwitch(node *yaml.Node, path string, found func(string, string)) {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			inspectEngineSwitch(child, path, found)
		}
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			key, value := node.Content[i].Value, node.Content[i+1]
			next := strings.TrimPrefix(path+"."+key, ".")
			if key == "ITOS_CONTAINER_ENGINE_TESTS" {
				found(next, value.Value)
			} else {
				inspectEngineSwitch(value, next, found)
			}
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			inspectEngineSwitch(child, path+"."+strconv.Itoa(i), found)
		}
	case yaml.ScalarNode:
		if strings.Contains(node.Value, "ITOS_CONTAINER_ENGINE_TESTS") {
			found(path, node.Value)
		}
	case yaml.AliasNode:
		inspectEngineSwitch(node.Alias, path, found)
	}
}
