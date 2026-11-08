package features

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
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
		workflowStep{Name: "Engine scenarios", Shell: "bash", Timeout: 8, Env: maps.Clone(fixtureEnvironment), Run: engineInvocation},
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
	require(run.Run == engineInvocation && run.Shell == "bash" && run.If == "" && run.Timeout == 8 && !run.ContinueOnError, "complete invocation")
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

// Source checks describe executable call sites, not diagnostic words anywhere in
// the file. They deliberately do not execute the engine driver or a fake Docker.
func fixtureFunction(script, name string) string {
	match := regexp.MustCompile(`(?ms)^` + regexp.QuoteMeta(name) + `\(\) \{\n(.*?)^\}`).FindStringSubmatch(script)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func fixtureGuardIssues(driver, checks string) []string {
	var issues []string
	// This must be the first check: on the old helper it identifies the silent
	// predicate itself, rather than complaining that a new helper is absent.
	if regexp.MustCompile(`(?ms)^  jq -e --arg id "\$built" .*?"\$state/manifest.json" >/dev/null\n`).MatchString(fixtureFunction(driver, "prepare")) {
		issues = append(issues, "manifest predicate can fail without fail() diagnostics")
	}
	for _, rule := range []struct{ function, prefix string }{
		{"prepare", `d image push "$tag"`},
		{"prepare", `d image save --output "$state/built.tar" "$built"`},
		{"prepare", `tar -xOf "$state/built.tar" manifest.json`},
		{"prepare", `config_path=$(daemon_config_path `},
		{"prepare", `tar -xOf "$state/built.tar" "$config_path"`},
		{"prepare", `daemon_config=$(daemon_config_digest `},
		{"prepare", `curl --fail --silent --show-error --noproxy '*' --proto '=http' --max-time 10`},
		{"prepare", `digest=$(validate_manifest `},
		{"prepare", `remove_image_ref tag`},
		{"prepare", `remove_image_ref reference`},
		{"prepare", `d image pull --platform linux/amd64 "$reference"`},
		{"prepare", `cached_fixture "$reference" "$built" "$fixture_uid" "$fixture_gid"`},
		{"prepare", `printf '%s\n' "$reference" >"$state/reference"`},
		{"prepare", `printf '%s\n' "$smoke_name" >"$state/smoke.intent"`},
		{"prepare", `d container create --name "$smoke_name"`},
		{"prepare", `smoke_id=$(cat "$state/smoke.cid")`},
		{"prepare", `timeout --signal=TERM --kill-after=1s 9s`},
		{"prepare", `d container inspect "$smoke_id"`},
		{"prepare", `validate_smoke `},
		{"prepare", `remove_container smoke`},
		{"prepare", `remove_container registry`},
		{"prepare", `printf 'image=%s\nimage_id=%s\nuid=%s\ngid=%s\n'`},
		{"cached_fixture", `d image inspect "$reference"`},
		{"cached_fixture", `validate_cached `},
		{"inspect_image", `message=$(cat `},
		{"remove_image_ref", `ref=$(cat `},
		{"remove_image_ref", `repository=$(cat `},
		{"remove_image_ref", `built=$(cat `},
		{"remove_image_ref", `observed=$(jq `},
		{"remove_image_ref", `base=$(cat `},
		{"remove_image_ref", `registry=$(cat `},
		{"remove_image_ref", `d image rm `},
		{"remove_container", `name=$(cat `},
		{"remove_container", `expected=$(cat `},
		{"remove_container", `cid=$(cat `},
		{"remove_container", `message=$(cat `},
		{"remove_container", `observed=$(jq `},
		{"remove_container", `printf '%s\n' "$observed" >`},
		{"remove_container", `d container rm `},
		{"cleanup", `recorded=$(cat `},
		{"validate_manifest", `header_data=$(cat `},
		{"validate_manifest", `actual=$(sha256sum `},
		{"validate_manifest", `manifest_observation `},
		{"validate_manifest", `jq -e -s --arg id "$daemon_config" `},
		{"daemon_config_path", `member=$(jq `},
		{"daemon_config_digest", `actual=$(sha256sum `},
		{"daemon_config_digest", `jq -e -s `},
		{"validate_cached", `jq -e -s --arg ref `},
		{"validate_smoke", `jq -e -s `},
		{"validate_smoke", `observed=$(cat `},
	} {
		source := driver
		if strings.HasPrefix(rule.function, "validate_") || strings.HasPrefix(rule.function, "daemon_config_") {
			source = checks
		}
		body := strings.ReplaceAll(fixtureFunction(source, rule.function), "\\\n", "")
		found := false
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(strings.SplitN(line, " # ", 2)[0])
			if strings.HasPrefix(line, rule.prefix) {
				found = true
				if !regexp.MustCompile(`\|\| fail(_manifest_read)? `).MatchString(line) {
					issues = append(issues, "unguarded "+rule.function+": "+rule.prefix)
				}
			}
		}
		if !found {
			issues = append(issues, "missing executable "+rule.function+": "+rule.prefix)
		}
	}
	bounded := fixtureFunction(driver, "bounded_cleanup")
	finish := fixtureFunction(driver, "finish_preparation")
	if strings.Contains(bounded, "set -e") || !strings.Contains(bounded, `|| result=$?`) ||
		!strings.Contains(finish, `local result=$?`) || !strings.Contains(finish, `bounded_cleanup || printf`) || !strings.Contains(finish, `exit "$result"`) {
		issues = append(issues, "cleanup must preserve original preparation status")
	}
	prepare := fixtureFunction(driver, "prepare")
	if !strings.Contains(prepare, `digest=$(validate_manifest "$state/manifest.json" "$state/manifest.headers" "$built" "$daemon_config")`) {
		issues = append(issues, "manifest validation must use the independently measured daemon config")
	}
	if strings.Count(prepare, `>>"$GITHUB_OUTPUT"`) != 1 {
		issues = append(issues, "exactly one final fixture output publication")
	}
	if strings.Count(prepare, `cached_fixture "$reference" "$built" "$fixture_uid" "$fixture_gid"`) != 2 ||
		strings.LastIndex(prepare, "cached_fixture ") > strings.Index(prepare, "printf 'image=") {
		issues = append(issues, "publish only after both cache checks and cleanup")
	}
	remaining := prepare
	for _, call := range []string{"digest=$(validate_manifest ", "remove_image_ref tag", "remove_image_ref reference", "if inspect_image ", "d image pull ", "cached_fixture ", "validate_smoke ", "remove_container smoke", "remove_container registry", "cached_fixture ", "printf 'image="} {
		at := strings.Index(remaining, call)
		if at < 0 {
			issues = append(issues, "missing or out-of-order preparation call: "+call)
			break
		}
		remaining = remaining[at+len(call):]
	}
	if !strings.Contains(driver, `source "${BASH_SOURCE[0]%/*}/../lib/engine-fixture-checks.sh"`) {
		issues = append(issues, "production must source shared validators")
	}
	observation := fixtureFunction(checks, "manifest_observation")
	// Only a printed jq object in the called summary function counts. A comment,
	// an unused shell string or a different function cannot supply these fields.
	match := regexp.MustCompile(`(?s)summary=\$\(jq .*?'(.*?)' "\$manifest" 2>/dev/null\)`).FindStringSubmatch(observation)
	for _, field := range []string{"schemaVersion", "mediaType", "hasManifests", "configDigest", "builtID", "advertisedDigest", "measuredDigest"} {
		if len(match) != 2 || !regexp.MustCompile(`\b`+field+`:`).MatchString(match[1]) {
			issues = append(issues, "missing manifest observation "+field)
		}
	}
	if !strings.Contains(observation, `printf 'manifest observation: %s\n' "$summary" >&2 || fail `) {
		issues = append(issues, "manifest summary must be printed with failure propagation")
	}
	return issues
}

func requireFixture(t *testing.T, ok bool, format string, args ...any) {
	t.Helper()
	if !ok {
		t.Fatalf(format, args...)
	}
}

func TestEngineFixturePostPushDiagnostics(t *testing.T) {
	driverBytes, err := os.ReadFile("../tools/bin/engine-fixture")
	requireFixture(t, err == nil, "%v", err)
	driver := string(driverBytes)
	// Assert the old, executable predicate before trying to load the new library.
	if issues := fixtureGuardIssues(driver, ""); len(issues) > 0 && issues[0] == "manifest predicate can fail without fail() diagnostics" {
		t.Fatal(issues[0])
	}
	checksBytes, err := os.ReadFile("../tools/lib/engine-fixture-checks.sh")
	requireFixture(t, err == nil, "%v", err)
	checks := string(checksBytes)
	issues := fixtureGuardIssues(driver, checks)
	requireFixture(t, len(issues) == 0, "%s", strings.Join(issues, "\n"))
	for _, tc := range []struct{ name, old, replacement string }{
		{"unguarded push", `d image push "$tag" || fail`, `d image push "$tag" # || fail`},
		{"unguarded cache inspect", `>"$state/cached-image.json" || fail`, `>"$state/cached-image.json" # || fail`},
		{"unguarded validator", `"$built" "$daemon_config") || fail`, `"$built" "$daemon_config") # || fail`},
		{"unguarded daemon export", `d image save --output "$state/built.tar" "$built" || fail`, `d image save --output "$state/built.tar" "$built" # || fail`},
		{"unguarded config extraction", `>"$state/daemon-config.json" || fail`, `>"$state/daemon-config.json" # || fail`},
		{"config identity bypass", `"$built" "$daemon_config")`, `"$built" "$built")`},
		{"unguarded owned removal", `d image rm "$ref" || fail`, `d image rm "$ref" # || fail`},
		{"unguarded ownership read", `base=$(cat "$state/base.iid") || fail`, `base=$(cat "$state/base.iid") # || fail`},
		{"early output publication", `remove_container registry || fail`, `printf 'image=bad' >>"$GITHUB_OUTPUT"; remove_container registry || fail`},
		{"cleanup overwrites status", `exit "$result"`, `exit 1`},
		{"unguarded smoke state", `"$file" >/dev/null 2>&1 || fail "smoke state:`, `"$file" >/dev/null 2>&1 # || fail "smoke state:`},
		{"lost observation", `mediaType:`, `unusedMediaType:`},
		{"unused observation string", `mediaType:`, `unusedMediaType:`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			brokenDriver := strings.ReplaceAll(driver, tc.old, tc.replacement)
			brokenChecks := strings.ReplaceAll(checks, tc.old, tc.replacement)
			if tc.name == "unused observation string" {
				brokenChecks += "\n# mediaType: is only a comment\nunused='mediaType:'\n"
			}
			requireFixture(t, brokenDriver != driver || brokenChecks != checks, "negative control changed no source")
			requireFixture(t, len(fixtureGuardIssues(brokenDriver, brokenChecks)) != 0, "accepted missing executable guard/observation")
		})
	}
}

func TestEngineFixtureManifestValidation(t *testing.T) {
	built := "sha256:" + strings.Repeat("a", 64)
	other := "sha256:" + strings.Repeat("b", 64)
	manifest := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"digest":"` + built + `"}}`
	for _, tc := range []struct{ name, body, header, id, diagnostic string }{
		{"OCI image", manifest, "", built, ""},
		{"Docker v2", strings.ReplaceAll(manifest, "application/vnd.oci.image.manifest.v1+json", "application/vnd.docker.distribution.manifest.v2+json"), "", built, ""},
		{"schema", strings.ReplaceAll(manifest, `"schemaVersion":2`, `"schemaVersion":1`), "", built, "manifest validation"},
		{"schema type", strings.ReplaceAll(manifest, `"schemaVersion":2`, `"schemaVersion":"2"`), "", built, "manifest validation"},
		{"media", strings.ReplaceAll(manifest, "application/vnd.oci.image.manifest.v1+json", "unknown"), "", built, "manifest validation"},
		{"media type", strings.ReplaceAll(manifest, `"application/vnd.oci.image.manifest.v1+json"`, `[]`), "", built, "manifest validation"},
		{"missing media", strings.ReplaceAll(manifest, `"mediaType":`, `"unused":`), "", built, "manifest validation"},
		{"OCI index", strings.ReplaceAll(manifest, "application/vnd.oci.image.manifest.v1+json", "application/vnd.oci.image.index.v1+json"), "", built, "manifest validation"},
		{"Docker list", strings.ReplaceAll(manifest, "application/vnd.oci.image.manifest.v1+json", "application/vnd.docker.distribution.manifest.list.v2+json"), "", built, "manifest validation"},
		{"empty manifests", strings.ReplaceAll(manifest, `"config":`, `"manifests":[],"config":`), "", built, "manifest validation"},
		{"null manifests", strings.ReplaceAll(manifest, `"config":`, `"manifests":null,"config":`), "", built, "manifest validation"},
		{"missing config", strings.ReplaceAll(manifest, `"config":`, `"unused":`), "", built, "manifest validation"},
		{"missing config digest", strings.ReplaceAll(manifest, `"digest":`, `"unused":`), "", built, "manifest validation"},
		{"config type", strings.ReplaceAll(manifest, `{"digest":"`+built+`"}`, `false`), "", built, "manifest validation"},
		{"config digest type", strings.ReplaceAll(manifest, `"`+built+`"`, `123`), "", built, "manifest validation"},
		{"wrong config", strings.ReplaceAll(manifest, built, other), "", built, "manifest validation"},
		{"malformed", "{", "", built, "manifest JSON"},
		{"empty", "", "", built, "manifest JSON"},
		{"array", "[]", "", built, "manifest validation"},
		{"null", "null", "", built, "manifest validation"},
		{"multiple documents", manifest + "\n" + manifest, "", built, "manifest JSON"},
		{"absent header", manifest, "HTTP/1.1 200 OK\r\n\r\n", built, "manifest header"},
		{"duplicate header", manifest, "Docker-Content-Digest: DIGEST\r\nDocker-Content-Digest: DIGEST\r\n", built, "manifest header"},
		{"malformed header", manifest, "Docker-Content-Digest: invalid\r\n", built, "manifest header"},
		{"digest byte mismatch", manifest, "Docker-Content-Digest: " + other + "\r\n", built, "manifest bytes"},
		{"config as manifest", manifest, "Docker-Content-Digest: " + built + "\r\n", built, "manifest bytes"},
		{"invalid built", manifest, "", "bad-id", "built identity"},
		{"escaped bounded observation", strings.ReplaceAll(manifest, "application/vnd.oci.image.manifest.v1+json", `line\n\u001b`+strings.Repeat("x", 5000)), "", built, "manifest validation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			digest := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(tc.body)))
			header := tc.header
			if header == "" {
				header = "HTTP/1.1 200 OK\r\nDocker-Content-Digest: DIGEST\r\n\r\n"
			}
			header = strings.ReplaceAll(header, "DIGEST", digest)
			id := strings.ReplaceAll(tc.id, "DIGEST", digest)
			stdout, stderr, err := runFixtureValidator(t, "validate_manifest", fixtureResponseFile(t, "manifest.json", tc.body), fixtureResponseFile(t, "headers", header), id, built)
			if tc.diagnostic == "" {
				requireFixture(t, err == nil && strings.TrimSpace(stdout) == digest, "valid manifest rejected: %v stdout=%s stderr=%s", err, stdout, stderr)
			} else {
				requireFixture(t, err != nil && strings.Contains(stderr, "engine-fixture: "+tc.diagnostic) && stdout == "", "invalid manifest lacks failure diagnostic %q: %v stdout=%s stderr=%s", tc.diagnostic, err, stdout, stderr)
			}
			var summary map[string]json.RawMessage
			for _, line := range strings.Split(stderr, "\n") {
				if strings.HasPrefix(line, "manifest observation: ") {
					requireFixture(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "manifest observation: ")), &summary) == nil, "unescaped manifest observation: %s", line)
				}
			}
			for _, field := range []string{"schemaVersion", "mediaType", "hasManifests", "configDigest", "builtID", "advertisedDigest", "measuredDigest"} {
				requireFixture(t, summary[field] != nil, "missing %s observation: %s", field, stderr)
			}
			requireFixture(t, string(summary["builtID"]) == strconv.Quote(id) && string(summary["measuredDigest"]) == strconv.Quote(digest), "summary lost independently measured identities: %s", stderr)
			if tc.diagnostic == "" || tc.name == "wrong config" {
				wantConfig := built
				if tc.name == "wrong config" {
					wantConfig = other
				}
				requireFixture(t, string(summary["schemaVersion"]) == "2" && string(summary["hasManifests"]) == "false" && string(summary["configDigest"]) == strconv.Quote(wantConfig) && string(summary["advertisedDigest"]) == strconv.Quote(digest), "summary lost observed manifest fields: %s", stderr)
			}
			requireFixture(t, tc.name != "empty manifests" || string(summary["hasManifests"]) == "true", "summary hid manifests key: %s", stderr)
			requireFixture(t, len(stderr) <= 2400 && !strings.ContainsRune(stderr, '\x1b'), "unbounded or unescaped diagnostics (%d bytes)", len(stderr))
			requireFixture(t, (tc.name != "malformed" && tc.name != "empty") || strings.Contains(stderr, "invalid JSON"), "missing invalid JSON fallback: %s", stderr)
		})
	}

	testFixtureImageStores(t, built, other, manifest)

	t.Run("missing captured files", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "absent")
		for _, tc := range []struct{ manifest, headers, stage string }{
			{fixtureResponseFile(t, "manifest.json", manifest), missing, "manifest header read"},
			{missing, fixtureResponseFile(t, "headers", "Docker-Content-Digest: "+other+"\r\n"), "manifest byte hashing"},
		} {
			stdout, stderr, err := runFixtureValidator(t, "validate_manifest", tc.manifest, tc.headers, built)
			requireFixture(t, err != nil && stdout == "" && strings.Contains(stderr, "engine-fixture: "+tc.stage) && strings.Contains(stderr, "manifest observation:"), "read failure lost stage/observation: %v %s", err, stderr)
		}
	})

	reference := "127.0.0.1:1234/fixture@" + other
	cached := `[{"Id":"` + built + `","RepoDigests":["` + reference + `"],"Os":"linux","Architecture":"amd64","Config":{"User":"1001:1002","Entrypoint":[],"Volumes":{},"Labels":{}}}]`
	smoke := `[{"State":{"Status":"exited","ExitCode":0,"OOMKilled":false,"Error":""}}]`
	for _, tc := range []struct{ name, source, old, replacement string }{
		{"cache valid", cached, "", ""},
		{"cache identity", cached, built, other},
		{"cache reference", cached, reference, "elsewhere@" + other},
		{"cache OS", cached, "linux", "windows"},
		{"cache architecture", cached, "amd64", "arm64"},
		{"cache root", cached, "1001:1002", "0:1002"},
		{"cache gid", cached, "1001:1002", "1001:1003"},
		{"cache entrypoint", cached, `"Entrypoint":[]`, `"Entrypoint":["/bin/sh"]`},
		{"cache entrypoint type", cached, `"Entrypoint":[]`, `"Entrypoint":false`},
		{"cache volumes type", cached, `"Volumes":{}`, `"Volumes":false`},
		{"cache labels type", cached, `"Labels":{}`, `"Labels":false`},
		{"cache extra inspection", cached, cached, cached + "\n" + cached},
		{"cache volumes", cached, `"Volumes":{}`, `"Volumes":{"/data":{}}`},
		{"cache metadata", cached, `"Labels":{}`, `"Labels":{"devcontainer.metadata":"{}"}`},
		{"cache malformed", cached, cached, "{"},
		{"smoke valid", smoke, "", ""},
		{"smoke status", smoke, "exited", "running"},
		{"smoke exit", smoke, `"ExitCode":0`, `"ExitCode":1`},
		{"smoke OOM", smoke, "false", "true"},
		{"smoke error", smoke, `"Error":""`, `"Error":"synthetic error"`},
		{"smoke extra inspection", smoke, smoke, smoke + "\n" + smoke},
		{"smoke missing OOM", smoke, `"OOMKilled":false`, `"unused":false`},
		{"smoke exit type", smoke, `"ExitCode":0`, `"ExitCode":"0"`},
		{"smoke malformed", smoke, smoke, "{"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.ReplaceAll(tc.source, tc.old, tc.replacement)
			file := fixtureResponseFile(t, "inspection.json", body)
			function, args, diagnostic := "validate_cached", []string{file, reference, built, "1001", "1002"}, "cached validation"
			if strings.HasPrefix(tc.name, "smoke") {
				function, args, diagnostic = "validate_smoke", []string{file, fixtureResponseFile(t, "smoke.out", "uid=1001 gid=1002\nT-20-engine-fixture-ok\n"), "1001", "1002"}, "smoke state"
			}
			_, stderr, err := runFixtureValidator(t, function, args...)
			if tc.old == "" {
				requireFixture(t, err == nil, "valid inspection rejected: %v %s", err, stderr)
			} else {
				requireFixture(t, err != nil && strings.Contains(stderr, "engine-fixture: "+diagnostic) && strings.Contains(stderr, "observed="), "inspection failure missing observation: %v %s", err, stderr)
			}
		})
	}
	for _, output := range []string{"", "uid=0 gid=1002\nT-20-engine-fixture-ok\n", "uid=1001 gid=1002\nwrong marker\n"} {
		_, stderr, err := runFixtureValidator(t, "validate_smoke", fixtureResponseFile(t, "smoke.json", smoke), fixtureResponseFile(t, "smoke.out", output), "1001", "1002")
		requireFixture(t, err != nil && strings.Contains(stderr, "engine-fixture: smoke output") && strings.Contains(stderr, "observed="), "smoke output failure missing observation: %v %s", err, stderr)
	}
}
