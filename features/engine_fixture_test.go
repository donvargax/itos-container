package features

import (
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const registryFixturePin = "docker.io/library/registry@sha256:5b12b22f21522fe69443079df04c0f3f42cb9977857f3c20404386652a6c6d8e"
const busyboxFixturePin = "docker.io/library/busybox@sha256:5624bb6777cf469b1fb50ae3f981b15d2715ee2fffaf20b9998cfb7843726835"

var fixtureEnvironment = map[string]string{
	"ITOS_CONTAINER_ENGINE_TESTS":         "1",
	"ITOS_CONTAINER_ENGINE_FIXTURE_IMAGE": "${{ steps.fixture.outputs.image }}",
	"ITOS_CONTAINER_ENGINE_FIXTURE_ID":    "${{ steps.fixture.outputs.image_id }}",
	"ITOS_CONTAINER_ENGINE_FIXTURE_UID":   "${{ steps.fixture.outputs.uid }}",
	"ITOS_CONTAINER_ENGINE_FIXTURE_GID":   "${{ steps.fixture.outputs.gid }}",
	"ITOS_CONTAINER_RED_SCENARIOS":        redDispatchExpression,
}

func validFixtureEnvironment(env map[string]string) bool {
	return maps.Equal(env, fixtureEnvironment)
}

func TestEngineFixtureWorkflow(t *testing.T) {
	data, err := os.ReadFile("../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("preparation outputs and cleanup", func(t *testing.T) {
		for _, issue := range engineFixtureWorkflowIssues(data) {
			t.Error(issue)
		}
	})
	// This is a static regression, not an engine fake or a simulated CI process.
	// Give every negative control a valid parsed baseline even at the red commit.
	var ready engineWorkflow
	if err := yaml.Unmarshal(data, &ready); err != nil {
		t.Fatal(err)
	}
	j := ready.Jobs["engine"]
	j.Steps = append(slices.Clone(j.Steps[:3]),
		workflowStep{ID: "fixture", Name: "Prepare engine fixture registry", Shell: "bash", Timeout: 4, Run: "tools/bin/engine-fixture prepare"},
		workflowStep{Name: "Engine scenarios", Shell: "bash", Timeout: 5, Env: maps.Clone(fixtureEnvironment), Run: engineInvocation},
		workflowStep{Name: "Engine failure diagnostics", If: "${{ failure() }}", Shell: "bash", Timeout: 1, Run: engineFailureDiagnostics},
		workflowStep{Name: "Clean up engine fixture", If: "${{ always() }}", Shell: "bash", Timeout: 1, Run: "tools/bin/engine-fixture cleanup"})
	ready.Jobs["engine"] = j
	setRedDispatch(t, &ready, redDispatchSchema)
	baseline, err := yaml.Marshal(ready)
	if err != nil {
		t.Fatal(err)
	}
	if issues := engineFixtureWorkflowIssues(baseline); len(issues) != 0 {
		t.Fatalf("invalid negative-control baseline: %v", issues)
	}
	for _, tc := range []struct {
		name, issue string
		breakIt     func(*engineWorkflow)
	}{
		{"missing preparation", "preparation", func(w *engineWorkflow) {
			j := w.Jobs["engine"]
			j.Steps = slices.Delete(j.Steps, 3, 4)
			w.Jobs["engine"] = j
		}},
		{"missing cleanup", "cleanup", func(w *engineWorkflow) {
			j := w.Jobs["engine"]
			j.Steps = j.Steps[:5]
			w.Jobs["engine"] = j
		}},
		{"ignored preparation failure", "preparation", func(w *engineWorkflow) {
			changeStep(w, "engine", "Prepare engine fixture registry", func(s *workflowStep) { s.ContinueOnError = true })
		}},
		{"preparation success fallback", "preparation", func(w *engineWorkflow) {
			changeStep(w, "engine", "Prepare engine fixture registry", func(s *workflowStep) { s.Run += " || true" })
		}},
		{"conditional preparation", "preparation", func(w *engineWorkflow) {
			changeStep(w, "engine", "Prepare engine fixture registry", func(s *workflowStep) { s.If = "false" })
		}},
		{"unbounded preparation", "preparation", func(w *engineWorkflow) {
			changeStep(w, "engine", "Prepare engine fixture registry", func(s *workflowStep) { s.Timeout = 0 })
		}},
		{"conditional cleanup", "cleanup", func(w *engineWorkflow) {
			changeStep(w, "engine", "Clean up engine fixture", func(s *workflowStep) { s.If = "success()" })
		}},
		{"ignored cleanup failure", "cleanup", func(w *engineWorkflow) {
			changeStep(w, "engine", "Clean up engine fixture", func(s *workflowStep) { s.ContinueOnError = true })
		}},
		{"late preparation", "ordered", func(w *engineWorkflow) {
			j := w.Jobs["engine"]
			j.Steps[3], j.Steps[4] = j.Steps[4], j.Steps[3]
			w.Jobs["engine"] = j
		}},
		{"wrong output", "output wiring", func(w *engineWorkflow) {
			changeStep(w, "engine", "Engine scenarios", func(s *workflowStep) { s.Env["ITOS_CONTAINER_ENGINE_FIXTURE_IMAGE"] = "busybox:latest" })
		}},
		{"leaked fixture environment", "leaked fixture", func(w *engineWorkflow) { w.Env["ITOS_CONTAINER_ENGINE_FIXTURE_ID"] = "id" }},
		{"leaked fixture in command", "leaked fixture", func(w *engineWorkflow) {
			j := w.Jobs["ci"]
			j.Steps[0].Run += "\nexport ITOS_CONTAINER_ENGINE_FIXTURE_UID=1001"
			w.Jobs["ci"] = j
		}},
		{"ordinary acquisition", "acquisition outside engine", func(w *engineWorkflow) {
			j := w.Jobs["platform"]
			j.Steps = append(j.Steps, workflowStep{Run: "docker image pull busybox:latest"})
			w.Jobs["platform"] = j
		}},
		{"ordinary helper", "acquisition outside engine", func(w *engineWorkflow) {
			j := w.Jobs["ci"]
			j.Steps = append(j.Steps, workflowStep{Run: "tools/bin/engine-fixture prepare"})
			w.Jobs["ci"] = j
		}},
		{"filtered engine run", "complete invocation", func(w *engineWorkflow) {
			changeStep(w, "engine", "Engine scenarios", func(s *workflowStep) { s.Run += " -scenarios=@smoke" })
		}},
		{"new dispatch input", "dispatch inputs", func(w *engineWorkflow) {
			var n yaml.Node
			if err := yaml.Unmarshal([]byte("inputs: {fixture: {default: true}}"), &n); err != nil {
				t.Fatal(err)
			}
			w.On["workflow_dispatch"] = *n.Content[0]
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
			if issues := strings.Join(engineFixtureWorkflowIssues(fixture), "; "); !strings.Contains(issues, tc.issue) {
				t.Errorf("negative control missed %q: %s", tc.issue, issues)
			}
		})
	}
	t.Run("reviewed recipe and bounded helper", func(t *testing.T) {
		for _, file := range []struct {
			path  string
			check func(string) []string
		}{
			{"testdata/engine-fixture/Dockerfile", fixtureRecipeIssues},
			{"../tools/bin/engine-fixture", fixtureHelperIssues},
		} {
			contents, err := os.ReadFile(file.path)
			if err != nil {
				t.Error(err)
				continue
			}
			for _, issue := range file.check(string(contents)) {
				t.Error(issue)
			}
		}
	})
	t.Run("recipe and helper negative controls", func(t *testing.T) {
		recipe := strings.Join(fixtureRecipe(), "\n")
		helper := strings.Join(fixtureHelperRequirements(), "\n")
		if len(fixtureRecipeIssues(recipe)) != 0 || len(fixtureHelperIssues(helper)) != 0 {
			t.Fatal("invalid recipe/helper baseline")
		}
		for _, broken := range []string{strings.ReplaceAll(recipe, busyboxFixturePin, "busybox:latest"), recipe + "\nRUN id", recipe + "\nCOPY . /", strings.ReplaceAll(recipe, "${FIXTURE_UID}:${FIXTURE_GID}", "0:0")} {
			if len(fixtureRecipeIssues(broken)) == 0 {
				t.Errorf("accepted broken recipe: %s", broken)
			}
		}
		for _, broken := range []string{strings.ReplaceAll(helper, registryFixturePin, "registry:3"), strings.ReplaceAll(helper, "127.0.0.1::5000", "0.0.0.0::5000"), strings.ReplaceAll(helper, "179s", "300s"), strings.ReplaceAll(helper, "29s", "60s"), strings.ReplaceAll(helper, "--host unix:///var/run/docker.sock", "--host tcp://remote:2375")} {
			if len(fixtureHelperIssues(broken)) == 0 {
				t.Errorf("accepted broken helper: %s", broken)
			}
		}
	})
}

func engineFixtureWorkflowIssues(data []byte) []string {
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
	engine := w.Jobs["engine"]
	var names []string
	prepare, cleanup, run, diagnostics := workflowStep{}, workflowStep{}, workflowStep{}, workflowStep{}
	allowed := map[string]string{}
	for i, step := range engine.Steps {
		names = append(names, step.Name)
		switch step.Name {
		case "Prepare engine fixture registry":
			prepare = step
		case "Clean up engine fixture":
			cleanup = step
		case "Engine failure diagnostics":
			diagnostics = step
		case "Engine scenarios":
			run = step
			for key, value := range fixtureEnvironment {
				if strings.HasPrefix(key, "ITOS_CONTAINER_ENGINE_FIXTURE_") {
					allowed["jobs.engine.steps."+strconv.Itoa(i)+".env."+key] = value
				}
			}
		}
	}
	require(slices.Equal(names, []string{"", "", "Docker server preflight", "Prepare engine fixture registry", "Engine scenarios", "Engine failure diagnostics", "Clean up engine fixture"}), "ordered preflight/preparation/scenarios/diagnostics/cleanup")
	require(prepare.ID == "fixture" && prepare.Run == "tools/bin/engine-fixture prepare" && prepare.Shell == "bash" && prepare.Timeout == 4 && prepare.If == "" && prepare.Uses == "" && len(prepare.Env) == 0 && !prepare.ContinueOnError, "required bounded preparation with failure propagation")
	require(cleanup.Run == "tools/bin/engine-fixture cleanup" && cleanup.If == "${{ always() }}" && cleanup.Shell == "bash" && cleanup.Timeout == 1 && cleanup.Uses == "" && len(cleanup.Env) == 0 && !cleanup.ContinueOnError, "required always cleanup with failure propagation")
	require(diagnostics.Run == engineFailureDiagnostics && diagnostics.If == "${{ failure() }}" && diagnostics.Shell == "bash" && diagnostics.Timeout == 1 && diagnostics.Uses == "" && len(diagnostics.Env) == 0 && !diagnostics.ContinueOnError, "required bounded failure diagnostics")
	require(validFixtureEnvironment(run.Env), "exact fixture output wiring")
	require(run.Run == engineInvocation && run.Shell == "bash" && run.If == "" && run.Timeout == 5 && !run.ContinueOnError, "complete invocation")
	dispatch := w.On["workflow_dispatch"]
	require(validRedDispatch(dispatch), "exact optional red dispatch inputs")
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return append(issues, err.Error())
	}
	seen := 0
	walkFixtureScalars(&node, "", func(path, value string) {
		if strings.Contains(path, "ITOS_CONTAINER_ENGINE_FIXTURE_") || strings.Contains(value, "ITOS_CONTAINER_ENGINE_FIXTURE_") {
			expected, ok := allowed[path]
			require(ok && value == expected, "leaked fixture environment at "+path)
			seen++
		}
		if !strings.HasPrefix(path, "jobs.engine.") && (strings.Contains(value, "engine-fixture") || fixtureAcquisition.MatchString(value)) {
			require(false, "fixture acquisition outside engine at "+path)
		}
	})
	require(seen == 4, "four step-local fixture outputs required")
	return issues
}

var fixtureAcquisition = regexp.MustCompile(`(?s)\bdocker\b[^\n]*(\bpull\b|\bbuild\b|\bpush\b|\brun\b|\bcreate\b)`)

func walkFixtureScalars(node *yaml.Node, path string, visit func(string, string)) {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			walkFixtureScalars(child, path, visit)
		}
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			walkFixtureScalars(node.Content[i+1], strings.TrimPrefix(path+"."+node.Content[i].Value, "."), visit)
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			walkFixtureScalars(child, path+"."+strconv.Itoa(i), visit)
		}
	case yaml.ScalarNode:
		visit(path, node.Value)
	case yaml.AliasNode:
		walkFixtureScalars(node.Alias, path, visit)
	}
}

func fixtureRecipe() []string {
	return []string{"FROM " + busyboxFixturePin, "ARG FIXTURE_UID", "ARG FIXTURE_GID", "USER ${FIXTURE_UID}:${FIXTURE_GID}", "ENTRYPOINT []", `CMD ["/bin/true"]`}
}

func fixtureRecipeIssues(recipe string) []string {
	var lines []string
	for _, line := range strings.Split(recipe, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	if !slices.Equal(lines, fixtureRecipe()) {
		return []string{"fixture recipe must use the pinned base, numeric build identity and empty entrypoint only"}
	}
	return nil
}

// These are static guardrails only. The engine job must prove the manifest/config
// distinction, cold pull, cached execution and ownership-safe cleanup at runtime.
func fixtureHelperRequirements() []string {
	return []string{
		"registry_image='" + registryFixturePin + "'", "base_image='" + busyboxFixturePin + "'",
		`[[ ${CI:-} == true && ${GITHUB_ACTIONS:-} == true ]]`,
		`[[ $(uname -s) == Linux && $(uname -m) == x86_64 ]]`,
		`--host unix:///var/run/docker.sock`,
		`env -u DOCKER_CONTEXT -u DOCKER_HOST -u DOCKER_TLS_VERIFY -u DOCKER_CERT_PATH -u DOCKER_API_VERSION`,
		`timeout --signal=TERM --kill-after=1s 179s`,
		`timeout --signal=TERM --kill-after=1s 29s`,
		`timeout --signal=TERM --kill-after=1s 19s`,
		`--publish 127.0.0.1::5000`, `--env OTEL_TRACES_EXPORTER=none`,
		`--pull never --network none --read-only --cap-drop ALL`,
		`--security-opt no-new-privileges`,
	}
}

func fixtureHelperIssues(script string) []string {
	var issues []string
	for _, required := range fixtureHelperRequirements() {
		if !strings.Contains(script, required) {
			issues = append(issues, "missing fixture helper guard: "+required)
		}
	}
	return issues
}
