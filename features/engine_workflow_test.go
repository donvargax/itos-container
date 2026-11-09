package features

import (
	"maps"
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
	Permissions     map[string]string `yaml:"permissions"`
	Container       any               `yaml:"container"`
	Services        map[string]any    `yaml:"services"`
	ContinueOnError bool              `yaml:"continue-on-error"`
	Strategy        struct {
		Matrix struct {
			OS []string `yaml:"os"`
		} `yaml:"matrix"`
	} `yaml:"strategy"`
	Name    string         `yaml:"name"`
	Uses    string         `yaml:"uses"`
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
		{"missing ordinary job", "four ordinary jobs", func(w *engineWorkflow) { delete(w.Jobs, "platform-macos") }},
		{"extra engine job", "four ordinary jobs", func(w *engineWorkflow) { w.Jobs["extra-engine"] = w.Jobs["engine"] }},
		{"renamed release job", "four ordinary jobs", func(w *engineWorkflow) {
			w.Jobs["publish"] = w.Jobs["release"]
			delete(w.Jobs, "release")
		}},
		{"release without engine dependency", "release prerequisites", func(w *engineWorkflow) {
			j := w.Jobs["release"]
			j.Needs = []string{"ci", "platform"}
			w.Jobs["release"] = j
		}},
		{"release with manual dependency", "release prerequisites", func(w *engineWorkflow) {
			j := w.Jobs["release"]
			j.Needs = append(j.Needs, "platform-macos")
			w.Jobs["release"] = j
		}},
		{"release after failed prerequisites", "release guard", func(w *engineWorkflow) {
			j := w.Jobs["release"]
			j.If = strings.ReplaceAll(j.If, "success()", "always()")
			w.Jobs["release"] = j
		}},
		{"release on pull requests", "release guard", func(w *engineWorkflow) {
			j := w.Jobs["release"]
			j.If = strings.ReplaceAll(j.If, "'push'", "'pull_request'")
			w.Jobs["release"] = j
		}},
		{"release on another branch", "release guard", func(w *engineWorkflow) {
			j := w.Jobs["release"]
			j.If = strings.ReplaceAll(j.If, "refs/heads/main", "refs/heads/other")
			w.Jobs["release"] = j
		}},
		{"wrong release workflow", "release callee", func(w *engineWorkflow) {
			j := w.Jobs["release"]
			j.Uses = "./.github/workflows/other.yml"
			w.Jobs["release"] = j
		}},
		{"missing release permission", "release permissions", func(w *engineWorkflow) { delete(w.Jobs["release"].Permissions, "attestations") }},
		{"excess release permission", "release permissions", func(w *engineWorkflow) { w.Jobs["release"].Permissions["packages"] = "write" }},
		{"ordinary write permission", "preserve inherited read permissions", func(w *engineWorkflow) {
			j := w.Jobs["platform"]
			j.Permissions = map[string]string{"contents": "write"}
			w.Jobs["platform"] = j
		}},
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
		{"missing scenario coverage", "complete invocation", func(w *engineWorkflow) {
			changeStep(w, "engine", "Engine scenarios", func(s *workflowStep) {
				s.Run = strings.ReplaceAll(s.Run, "export ITOS_CC_TEST_COVERDIR\n", "")
			})
		}},
		{"skipped runtime step", "complete invocation", func(w *engineWorkflow) {
			changeStep(w, "engine", "Engine scenarios", func(s *workflowStep) { s.If = "false" })
		}},
		{"wrong platform", "native runner", func(w *engineWorkflow) { j := w.Jobs["engine"]; j.Runner = "windows-latest"; w.Jobs["engine"] = j }},
		{"job container", "native runner", func(w *engineWorkflow) { j := w.Jobs["engine"]; j.Container = "image"; w.Jobs["engine"] = j }},
		{"missing runtime budget", "complete invocation", func(w *engineWorkflow) {
			changeStep(w, "engine", "Engine scenarios", func(s *workflowStep) { s.Timeout = 0 })
		}},
		{"ignored binary failure", "complete invocation", func(w *engineWorkflow) {
			changeStep(w, "engine", "Engine scenarios", func(s *workflowStep) {
				s.Run = strings.ReplaceAll(s.Run, `exit "$test_status"`, "exit 0")
			})
		}},
		{"missing kernel evidence", "failure diagnostics", func(w *engineWorkflow) {
			changeStep(w, "engine", "Engine failure diagnostics", func(s *workflowStep) {
				s.Run = strings.ReplaceAll(s.Run, "sudo dmesg | tail -50", "true")
			})
		}},
		{"missing independent session", "complete invocation", func(w *engineWorkflow) {
			changeStep(w, "engine", "Engine scenarios", func(s *workflowStep) {
				s.Run = strings.ReplaceAll(s.Run, "setsid --wait ", "")
			})
		}},
		{"missing retained test output", "failure diagnostics", func(w *engineWorkflow) {
			changeStep(w, "engine", "Engine failure diagnostics", func(s *workflowStep) {
				s.Run = strings.ReplaceAll(s.Run, "$RUNNER_TEMP/slice-11-features.log", "$RUNNER_TEMP/missing.log")
			})
		}},
		{"skipped failure diagnostics", "failure diagnostics", func(w *engineWorkflow) {
			changeStep(w, "engine", "Engine failure diagnostics", func(s *workflowStep) { s.If = "success()" })
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
const engineFailureDiagnostics = `set +e
timeout 40 tools/bin/engine-failure-report "$RUNNER_TEMP/slice-11-features.log"
sudo dmesg | tail -50
free -m
if [[ -f "$RUNNER_TEMP/slice-11-features.log" ]]; then
  tail -100 "$RUNNER_TEMP/slice-11-features.log"
fi
`

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
	require(slices.Equal(slices.Sorted(maps.Keys(workflow.Jobs)), []string{
		"ci", "engine", "platform", "platform-macos", "platform-windows", "release",
	}), "preserve four ordinary jobs, exactly one engine job and the named release job")
	release := workflow.Jobs["release"]
	needs := slices.Clone(release.Needs)
	slices.Sort(needs)
	require(slices.Equal(needs, []string{"ci", "engine", "platform"}), "release prerequisites")
	require(release.If == "${{ success() && github.event_name == 'push' && github.ref == 'refs/heads/main' }}", "release guard")
	require(release.Uses == "./.github/workflows/release.yml", "release callee")
	require(maps.Equal(release.Permissions, map[string]string{
		"contents": "write", "id-token": "write", "attestations": "write",
	}), "release permissions")
	for name, job := range workflow.Jobs {
		if name != "release" {
			require(len(job.Permissions) == 0, "preserve inherited read permissions for "+name)
		}
	}
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
	require(len(engine.Steps) == 7, "engine must have checkout, setup-go, preflight, preparation, scenarios, failure diagnostics and cleanup only")
	if len(engine.Steps) >= 3 {
		checkout, setup, preflight := engine.Steps[0], engine.Steps[1], engine.Steps[2]
		require(checkout.Uses == "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1" && checkout.With["ref"] == "${{ env.TO }}", "checkout head and pin")
		require(setup.Uses == "actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e" && setup.With["go-version-file"] == "go.mod" && setup.With["cache-dependency-path"] == "go.sum", "setup-go toolchain and pin")
		require(preflight.Name == "Docker server preflight" && preflight.Shell == "bash" && !preflight.ContinueOnError && preflight.If == "" && validEnginePreflight(preflight.Run), "bounded read-only local-server preflight without ignored failures")
	}
	for i, run := range engine.Steps {
		if run.Name == "Engine scenarios" {
			require(run.Run == engineInvocation && run.Shell == "bash" && run.Timeout == 5 && run.If == "" && !run.ContinueOnError && validFixtureEnvironment(run.Env), "complete invocation, fixture outputs and step budget")
			enabledPath = "jobs.engine.steps." + strconv.Itoa(i) + ".env.ITOS_CONTAINER_ENGINE_TESTS"
		}
	}
	require(enabledPath != "", "complete invocation missing")
	issues = append(issues, engineFixtureWorkflowIssues(data)...)
	issues = append(issues, engineRedWorkflowIssues(data)...)
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

const redDispatchExpression = "${{ github.event_name == 'workflow_dispatch' && inputs.red_scenarios || '' }}"
const redDispatchSchema = `inputs:
  red_scenarios:
    description: Whitespace-separated complete scenario tags for expected-red evidence
    type: string
    required: false
    default: ''
`

func TestEngineRedWorkflow(t *testing.T) {
	data, err := os.ReadFile("../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("scoped manual input", func(t *testing.T) {
		for _, issue := range engineRedWorkflowIssues(data) {
			t.Error(issue)
		}
	})
	// A valid in-memory baseline lets every negative control run even at the red commit.
	var ready engineWorkflow
	if err := yaml.Unmarshal(data, &ready); err != nil {
		t.Fatal(err)
	}
	setRedDispatch(t, &ready, redDispatchSchema)
	for job, step := range map[string]string{"engine": "Engine scenarios", "platform-macos": "Tests", "platform-windows": "Tests"} {
		changeStep(&ready, job, step, func(s *workflowStep) { s.Env[redVariable] = redDispatchExpression })
	}
	baseline, err := yaml.Marshal(ready)
	if err != nil {
		t.Fatal(err)
	}
	if issues := engineRedWorkflowIssues(baseline); len(issues) != 0 {
		t.Fatalf("invalid baseline: %v", issues)
	}
	for _, tc := range []struct {
		name, issue string
		breakIt     func(*engineWorkflow)
	}{
		{"missing input", "dispatch inputs", func(w *engineWorkflow) { setRedDispatch(t, w, "null") }},
		{"required input", "dispatch inputs", func(w *engineWorkflow) {
			setRedDispatch(t, w, strings.ReplaceAll(redDispatchSchema, "required: false", "required: true"))
		}},
		{"nonempty default", "dispatch inputs", func(w *engineWorkflow) {
			setRedDispatch(t, w, strings.ReplaceAll(redDispatchSchema, "default: ''", "default: '@ID-MEMORY-03'"))
		}},
		{"missing default", "dispatch inputs", func(w *engineWorkflow) {
			setRedDispatch(t, w, strings.ReplaceAll(redDispatchSchema, "    default: ''\n", ""))
		}},
		{"wrong input type", "dispatch inputs", func(w *engineWorkflow) {
			setRedDispatch(t, w, strings.ReplaceAll(redDispatchSchema, "type: string", "type: boolean"))
		}},
		{"missing description", "dispatch inputs", func(w *engineWorkflow) {
			setRedDispatch(t, w, strings.ReplaceAll(redDispatchSchema, "    description: Whitespace-separated complete scenario tags for expected-red evidence\n", ""))
		}},
		{"extra input", "dispatch inputs", func(w *engineWorkflow) { setRedDispatch(t, w, redDispatchSchema+"  other: {type: string}\n") }},
		{"missing engine wiring", "step-local red selection", func(w *engineWorkflow) {
			changeStep(w, "engine", "Engine scenarios", func(s *workflowStep) { delete(s.Env, redVariable) })
		}},
		{"missing macOS wiring", "step-local red selection", func(w *engineWorkflow) {
			changeStep(w, "platform-macos", "Tests", func(s *workflowStep) { delete(s.Env, redVariable) })
		}},
		{"missing Windows wiring", "step-local red selection", func(w *engineWorkflow) {
			changeStep(w, "platform-windows", "Tests", func(s *workflowStep) { delete(s.Env, redVariable) })
		}},
		{"script interpolation", "leaked red selection", func(w *engineWorkflow) {
			changeStep(w, "platform-macos", "Tests", func(s *workflowStep) { s.Run += " ${{ inputs.red_scenarios }}" })
		}},
		{"shell evaluation", "leaked red selection", func(w *engineWorkflow) {
			changeStep(w, "engine", "Engine scenarios", func(s *workflowStep) { s.Run += "\neval \"$ITOS_CONTAINER_RED_SCENARIOS\"" })
		}},
		{"unguarded expression", "step-local red selection", func(w *engineWorkflow) {
			changeStep(w, "engine", "Engine scenarios", func(s *workflowStep) { s.Env[redVariable] = "${{ inputs.red_scenarios }}" })
		}},
		{"global leakage", "leaked red selection", func(w *engineWorkflow) { w.Env[redVariable] = redDispatchExpression }},
		{"job leakage", "leaked red selection", func(w *engineWorkflow) {
			j := w.Jobs["engine"]
			j.Env = map[string]string{redVariable: redDispatchExpression}
			w.Jobs["engine"] = j
		}},
		{"proof leakage", "leaked red selection", func(w *engineWorkflow) {
			j := w.Jobs["ci"]
			j.Steps[len(j.Steps)-1].Env = map[string]string{redVariable: redDispatchExpression}
			w.Jobs["ci"] = j
		}},
		{"ordinary Linux leakage", "leaked red selection", func(w *engineWorkflow) {
			changeStep(w, "platform", "Tests", func(s *workflowStep) { s.Env[redVariable] = redDispatchExpression })
		}},
		{"preflight leakage", "leaked red selection", func(w *engineWorkflow) {
			changeStep(w, "engine", "Docker server preflight", func(s *workflowStep) { s.Env = map[string]string{redVariable: redDispatchExpression} })
		}},
		{"fixture leakage", "leaked red selection", func(w *engineWorkflow) {
			changeStep(w, "engine", "Prepare engine fixture registry", func(s *workflowStep) { s.Env = map[string]string{redVariable: redDispatchExpression} })
		}},
		{"GITHUB_ENV leakage", "leaked red selection", func(w *engineWorkflow) {
			changeStep(w, "engine", "Engine scenarios", func(s *workflowStep) { s.Run += "\necho ITOS_CONTAINER_RED_SCENARIOS=x >> $GITHUB_ENV" })
		}},
		{"ignored step failure", "failure propagation", func(w *engineWorkflow) {
			changeStep(w, "platform-windows", "Tests", func(s *workflowStep) { s.ContinueOnError = true })
		}},
		{"ignored job failure", "failure propagation", func(w *engineWorkflow) {
			j := w.Jobs["platform-macos"]
			j.ContinueOnError = true
			w.Jobs["platform-macos"] = j
		}},
		{"success fallback", "unchanged test command", func(w *engineWorkflow) {
			changeStep(w, "platform-macos", "Tests", func(s *workflowStep) { s.Run += " || true" })
		}},
		{"forced red failure", "unchanged test command", func(w *engineWorkflow) {
			changeStep(w, "engine", "Engine scenarios", func(s *workflowStep) { s.Run += "; exit 1" })
		}},
		{"conditional test step", "failure propagation", func(w *engineWorkflow) {
			changeStep(w, "platform-windows", "Tests", func(s *workflowStep) { s.If = "false" })
		}},
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
			issues := strings.Join(engineRedWorkflowIssues(fixture), "; ")
			if !strings.Contains(issues, tc.issue) {
				t.Errorf("negative control missed %q: %s", tc.issue, issues)
			}
		})
	}
}

func setRedDispatch(t *testing.T, w *engineWorkflow, source string) {
	t.Helper()
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(source), &node); err != nil {
		t.Fatal(err)
	}
	w.On["workflow_dispatch"] = *node.Content[0]
}

func validRedDispatch(node yaml.Node) bool {
	var dispatch map[string]map[string]map[string]any
	if node.Decode(&dispatch) != nil || len(dispatch) != 1 || len(dispatch["inputs"]) != 1 {
		return false
	}
	input := dispatch["inputs"]["red_scenarios"]
	description, ok := input["description"].(string)
	description = strings.ToLower(description)
	return len(input) == 4 && input["type"] == "string" && input["required"] == false && input["default"] == "" && ok && strings.Contains(description, "whitespace-separated") && strings.Contains(description, "complete scenario tags") && strings.Contains(description, "expected-red")
}

func engineRedWorkflowIssues(data []byte) []string {
	var w engineWorkflow
	if err := yaml.Unmarshal(data, &w); err != nil {
		return []string{err.Error()}
	}
	var issues []string
	require := func(ok bool, issue string) {
		if !ok {
			issues = append(issues, issue)
		}
	}
	require(validRedDispatch(w.On["workflow_dispatch"]), "exact optional red dispatch inputs")
	allowed := map[string]bool{}
	for job, name := range map[string]string{"engine": "Engine scenarios", "platform-macos": "Tests", "platform-windows": "Tests"} {
		j := w.Jobs[job]
		require(!j.ContinueOnError, "job failure propagation for "+job)
		found := 0
		for i, step := range j.Steps {
			if step.Name != name {
				continue
			}
			found++
			allowed["jobs."+job+".steps."+strconv.Itoa(i)+".env."+redVariable] = true
			require(step.Env[redVariable] == redDispatchExpression, "step-local red selection for "+job)
			run := "go test ./... -count=1"
			if job == "engine" {
				run = engineInvocation
			}
			require(step.Run == run, "unchanged test command for "+job)
			require(!step.ContinueOnError && step.If == "", "step failure propagation for "+job)
		}
		require(found == 1, "exactly one selected test step for "+job)
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return append(issues, err.Error())
	}
	seen := 0
	walkFixtureScalars(&node, "", func(path, value string) {
		if strings.HasPrefix(path, "on.workflow_dispatch.inputs.red_scenarios.") {
			return
		}
		if strings.Contains(path, redVariable) || strings.Contains(value, redVariable) || strings.Contains(value, "red_scenarios") {
			require(allowed[path] && value == redDispatchExpression, "leaked red selection at "+path)
			seen++
		}
	})
	require(seen == 3, "exactly three step-local red selection values")
	return issues
}
