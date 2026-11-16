package features

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

const fixtureWarmupCall = `engine_warmup_run 9s 1s '10 seconds' bash -euo pipefail -c "$(declare -f); warmup_fixture \"\$1\" \"\$2\" \"\$3\"" _ "$reference" "$fixture_uid" "$fixture_gid" || fail "warm-up phase: expected completion; observed status=$?"`

const fixtureWarmupOperation = `local reference=$1 fixture_uid=$2 fixture_gid=$3
printf '%s\n' "$warmup_name" >"$state/warmup.intent" || fail "warm-up intent record: expected writable record; observed status=$?"
d container create --name "$warmup_name" --cidfile "$state/warmup.cid" \
  --label "io.itos.engine-fixture=$owner" --pull never --network none --read-only --cap-drop ALL \
  --security-opt no-new-privileges --user "$fixture_uid:$fixture_gid" "$reference" || fail "warm-up create: expected hardened container; observed status=$?"
remove_container warmup || fail "warm-up removal: expected owned container absent; observed status=$?"`

// Ignore comments and layout, but require complete executable lines. A comment
// or an unused quoted string with the expected words cannot supply a call.
func fixtureWarmupLines(source string) string {
	source = strings.ReplaceAll(source, "\\\n", "")
	var lines []string
	for _, line := range strings.Split(source, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, regexp.MustCompile(`[ \t]+`).ReplaceAllString(line, " "))
		}
	}
	return strings.Join(lines, "\n")
}

func fixtureWarmupPreparationIssues(driver string) []string {
	prepare := fixtureWarmupLines(fixtureFunction(driver, "prepare"))
	call := fixtureWarmupLines(fixtureWarmupCall)
	if strings.Count(prepare, call) != 1 {
		return []string{"prepare must execute exactly one bounded warm-up with failure propagation before fixture outputs"}
	}
	remaining := prepare
	for _, stage := range []string{
		`remove_container smoke || fail`, `remove_container registry || fail`,
		`cached_fixture "$reference" "$built" "$fixture_uid" "$fixture_gid" || fail "final cache check:`,
		call, `printf 'image=%s\nimage_id=%s\nuid=%s\ngid=%s\n'`,
	} {
		at := strings.Index(remaining, "\n"+stage)
		if at < 0 {
			return []string{"warm-up must follow smoke/registry removal and final cache check, before outputs"}
		}
		remaining = remaining[at+1+len(stage):]
	}
	if strings.Count(prepare, `>>"$GITHUB_OUTPUT"`) != 1 || strings.Index(prepare, `>>"$GITHUB_OUTPUT"`) < strings.Index(prepare, call) {
		return []string{"fixture outputs must be published only after warm-up"}
	}
	return nil
}

func fixtureWarmupSourceIssues(driver string) []string {
	issues := fixtureWarmupPreparationIssues(driver)
	if fixtureWarmupLines(fixtureFunction(driver, "warmup_fixture")) != fixtureWarmupLines(fixtureWarmupOperation) {
		issues = append(issues, "warm-up must record intent, create the hardened exact fixture via d, and verify removal")
	}
	for _, rule := range []struct{ source, required string }{
		{fixtureWarmupLines(driver), `source "${BASH_SOURCE[0]%/*}/../lib/engine-fixture-warmup.sh" || {`},
		{fixtureWarmupLines(driver), `warmup_name="itos-fixture-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}-warmup"`},
		{fixtureWarmupLines(driver), `export registry_image base_image workspace state owner recipe registry_name smoke_name warmup_name`},
		{fixtureWarmupLines(fixtureFunction(driver, "cleanup")), `(remove_container warmup) || failed=1`},
		{fixtureWarmupLines(fixtureFunction(driver, "remove_container")), `warmup) [[ $name == "$warmup_name" ]] || fail "${diagnostic}container name: expected=$warmup_name; observed=$(fixture_value "$name")"
expected=$(cat "$state/build.iid") || fail "${diagnostic}container image read: expected build.iid; observed status=$?" ;;`},
		{fixtureWarmupLines(fixtureFunction(driver, "remove_container")), `[[ $kind != warmup ]] || diagnostic='warm-up '`},
		{fixtureWarmupLines(fixtureFunction(driver, "remove_container")), `digest_valid "$expected" || fail`},
		{fixtureWarmupLines(fixtureFunction(driver, "remove_container")), `cid=$name
if [[ -s $state/$kind.cid ]]; then
cid=$(cat "$state/$kind.cid") || fail`},
		{fixtureWarmupLines(fixtureFunction(driver, "remove_container")), `jq -e --arg name "/$name" --arg image "$expected" --arg owner "$owner" 'length == 1 and .[0].Name == $name and .[0].Image == $image and`},
		{fixtureWarmupLines(fixtureFunction(driver, "remove_container")), `.[0].Config.Labels["io.itos.engine-fixture"] == $owner and`},
		{fixtureWarmupLines(fixtureFunction(driver, "remove_container")), `all(.[0].Mounts[]?; .Type == "tmpfs")' "$state/removing-container.json" >/dev/null 2>&1 || fail`},
		{fixtureWarmupLines(fixtureFunction(driver, "remove_container")), `d container rm --force --volumes "$observed" || fail`},
		{fixtureWarmupLines(fixtureFunction(driver, "remove_container")), `if d container inspect "$observed" >"$state/removed-container.json" 2>"$state/container.err"; then`},
		{fixtureWarmupLines(fixtureFunction(driver, "remove_container")), `"Error: No such container: $observed"|"Error response from daemon: No such container: $observed"|"Error: No such object: $observed") return 0 ;;`},
	} {
		if !strings.Contains("\n"+rule.source, "\n"+rule.required) {
			issues = append(issues, "missing executable warm-up integration: "+rule.required)
		}
	}
	return issues
}

func TestEngineFixtureWarmup(t *testing.T) {
	driverBytes, err := os.ReadFile("../tools/bin/engine-fixture")
	requireFixture(t, err == nil, "%v", err)
	driver := string(driverBytes)
	// Assertion-level red on the actual prepare path, before loading new files.
	requireFixture(t, len(fixtureWarmupPreparationIssues(driver)) == 0, "%s", strings.Join(fixtureWarmupPreparationIssues(driver), "\n"))
	requireFixture(t, len(fixtureWarmupSourceIssues(driver)) == 0, "%s", strings.Join(fixtureWarmupSourceIssues(driver), "\n"))
	for _, tc := range []struct{ name, old, replacement string }{
		{"missing call", fixtureWarmupCall, ""},
		{"duplicated call", fixtureWarmupCall, fixtureWarmupCall + "\n  " + fixtureWarmupCall},
		{"commented call", fixtureWarmupCall, "# " + fixtureWarmupCall},
		{"unused call string", fixtureWarmupCall, "unused='" + fixtureWarmupCall + "'"},
		{"unbounded call", "engine_warmup_run 9s 1s '10 seconds'", "bash"},
		{"wrong TERM bound", "engine_warmup_run 9s", "engine_warmup_run 19s"},
		{"wrong KILL bound", "engine_warmup_run 9s 1s", "engine_warmup_run 9s 2s"},
		{"swallowed failure", `|| fail "warm-up phase: expected completion; observed status=$?"`, `|| true`},
		{"early output", fixtureWarmupCall, `printf 'image=bad' >>"$GITHUB_OUTPUT"` + "\n  " + fixtureWarmupCall},
		{"missing cleanup", `(remove_container warmup) || failed=1`, ""},
		{"ignored cleanup", `(remove_container warmup) || failed=1`, `(remove_container warmup) || true`},
		{"wrong image", `"$fixture_uid:$fixture_gid" "$reference" || fail "warm-up create:`, `"$fixture_uid:$fixture_gid" "$base_image" || fail "warm-up create:`},
		{"wrong user", `--user "$fixture_uid:$fixture_gid" "$reference" || fail "warm-up create:`, `--user 0:0 "$reference" || fail "warm-up create:`},
		{"wrong endpoint", `d container create --name "$warmup_name"`, `docker container create --name "$warmup_name"`},
		{"no intent", `>"$state/warmup.intent"`, `>"$state/unused.intent"`},
		{"no CID", `--cidfile "$state/warmup.cid"`, `--cidfile "$state/unused.cid"`},
		{"no owner", `--label "io.itos.engine-fixture=$owner"`, `--label "unused=$owner"`},
		{"wrong name check", `warmup) [[ $name == "$warmup_name" ]]`, `warmup) [[ $name == "$smoke_name" ]]`},
		{"wrong immutable image", `expected=$(cat "$state/build.iid")`, `expected=$(cat "$state/base.iid")`},
		{"missing image validation", `digest_valid "$expected" || fail`, `true || fail`},
		{"no interrupted-create recovery", `cid=$name`, `cid=unknown`},
		{"wrong image check", `.Image == $image and`, `.Image != $image and`},
		{"wrong owner check", `.Config.Labels["io.itos.engine-fixture"] == $owner`, `.Config.Labels["unused"] == $owner`},
		{"wrong mount check", `all(.[0].Mounts[]?; .Type == "tmpfs")`, `true`},
		{"no removal verification", `if d container inspect "$observed" >"$state/removed-container.json"`, `if false >"$state/removed-container.json"`},
		{"transport failure as absence", `"Error: No such object: $observed") return 0 ;;`, `*) return 0 ;;`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			broken := strings.ReplaceAll(driver, tc.old, tc.replacement)
			requireFixture(t, broken != driver, "negative control changed no source")
			requireFixture(t, len(fixtureWarmupSourceIssues(broken)) > 0, "accepted broken warm-up integration")
		})
	}
	t.Run("late call", func(t *testing.T) {
		broken := strings.ReplaceAll(driver, "  "+fixtureWarmupCall+"\n", "")
		publication := regexp.MustCompile(`(?m)^  printf 'image=.*$`)
		broken = publication.ReplaceAllStringFunc(broken, func(line string) string { return line + "\n  " + fixtureWarmupCall })
		requireFixture(t, broken != driver && len(fixtureWarmupSourceIssues(broken)) > 0, "accepted late warm-up")
	})

	t.Run("definitions only", func(t *testing.T) {
		library, err := os.ReadFile("../tools/lib/engine-fixture-warmup.sh")
		requireFixture(t, err == nil, "%v", err)
		remainder := regexp.MustCompile(`(?ms)^engine_warmup_[a-z_]+\(\) \{\n.*?^\}\n?`).ReplaceAllString(string(library), "")
		requireFixture(t, fixtureWarmupLines(remainder) == "", "library contains executable top-level work")
		stdout, stderr, err := runFixtureWarmupShell(t, `source "$1"`)
		requireFixture(t, err == nil && stdout == "" && stderr == "", "sourcing performed work: %v %s %s", err, stdout, stderr)
	})
	for _, tc := range []struct{ name, script, duration, outcome, diagnostic string }{
		{"success", `engine_warmup_run 1s 1s '2 seconds' bash -c 'sleep 0.03'`, "measured", "passed", ""},
		{"nonzero child", `engine_warmup_run 1s 1s '2 seconds' bash -c 'exit 7'`, "measured", "failed", "operation: expected status=0; observed status=7"},
		{"TERM timeout", `engine_warmup_run 0.1s 0.1s '0.2 seconds' bash -c 'exec sleep 3'`, "measured", "failed", "timeout: expected completion within 0.2 seconds; observed status=124"},
		{"KILL timeout", `engine_warmup_run 0.1s 0.1s '0.2 seconds' bash -c 'trap "" TERM; exec sleep 3'`, "measured", "failed", "timeout: expected completion within 0.2 seconds; observed status=137"},
		{"deterministic elapsed", fixtureWarmupClock("1234", "4567") + `engine_warmup_run 1s 1s '2 seconds' true`, "3333", "passed", ""},
		{"zero elapsed", fixtureWarmupClock("1234", "1234") + `engine_warmup_run 1s 1s '2 seconds' true`, "0", "passed", ""},
		{"backwards clock", fixtureWarmupClock("4567", "1234") + `engine_warmup_run 1s 1s '2 seconds' true`, "unknown", "failed", "timing:"},
		{"unavailable start clock", `engine_warmup_now() { return 1; }; engine_warmup_run 1s 1s '2 seconds' true`, "unknown", "failed", "timing:"},
		{"invalid start clock", `engine_warmup_now() { printf 'invalid\n'; }; engine_warmup_run 1s 1s '2 seconds' true`, "unknown", "failed", "timing:"},
		{"unavailable end clock", fixtureWarmupClock("1234", "") + `engine_warmup_run 1s 1s '2 seconds' true`, "unknown", "failed", "timing:"},
		{"invalid end clock", fixtureWarmupClock("1234", "invalid") + `engine_warmup_run 1s 1s '2 seconds' true`, "unknown", "failed", "timing:"},
		{"timing output failure", `engine_warmup_timing() { return 23; }; engine_warmup_run 1s 1s '2 seconds' true`, "", "failed", "timing output:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			started := time.Now()
			stdout, stderr, err := runFixtureWarmupShell(t, "source \"$1\"\n"+tc.script)
			elapsed := time.Since(started)
			requireFixture(t, (err == nil) == (tc.outcome == "passed"), "untruthful result: %v stdout=%s stderr=%s", err, stdout, stderr)
			if tc.diagnostic != "" {
				requireFixture(t, strings.Contains(stderr, "engine-fixture: warm-up "+tc.diagnostic), "missing distinct failure diagnostic: %s", stderr)
			}
			if tc.duration == "" {
				requireFixture(t, stdout == "", "failed output unexpectedly emitted: %s", stdout)
				return
			}
			match := regexp.MustCompile(`^engine warm-up timing: duration_ms=([0-9]+|unknown) outcome=(passed|failed)\n$`).FindStringSubmatch(stdout)
			requireFixture(t, len(match) == 3, "expected exactly one timing line: %q", stdout)
			requireFixture(t, match[2] == tc.outcome, "untruthful outcome: %s", stdout)
			if tc.duration == "measured" {
				ms, parseErr := strconv.ParseInt(match[1], 10, 64)
				requireFixture(t, parseErr == nil && ms >= 0 && ms <= elapsed.Milliseconds()+20, "invalid measured duration: %s", stdout)
				if strings.Contains(tc.name, "timeout") {
					requireFixture(t, ms >= 50 && elapsed < 2*time.Second, "timeout did not bound child: elapsed=%s %s", elapsed, stdout)
				}
				if tc.name == "success" {
					requireFixture(t, ms >= 10, "timing did not include the operation: %s", stdout)
				}
			} else {
				requireFixture(t, match[1] == tc.duration, "duration=%s, want %s", match[1], tc.duration)
			}
		})
	}
	t.Run("timing write failure", func(t *testing.T) {
		if _, err := os.Stat("/dev/full"); err != nil {
			t.Skip("requires the always-full device")
		}
		stdout, stderr, err := runFixtureWarmupShell(t, `source "$1"; engine_warmup_run 1s 1s '2 seconds' true >/dev/full`)
		requireFixture(t, err != nil && stdout == "" && strings.Contains(stderr, "engine-fixture: warm-up timing output:"), "timing write failure was hidden: %v %s %s", err, stdout, stderr)
	})
}

func fixtureWarmupClock(first, second string) string {
	return `engine_warmup_now() {
  if [[ ! -e clock-read ]]; then
    : > clock-read
    printf '%s\n' '` + first + `'
  else
    [[ -n '` + second + `' ]] || return 1
    printf '%s\n' '` + second + `'
  fi
}
`
}

func runFixtureWarmupShell(t *testing.T, script string) (string, string, error) {
	t.Helper()
	library, err := filepath.Abs("../tools/lib/engine-fixture-warmup.sh")
	requireFixture(t, err == nil, "%v", err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-euo", "pipefail", "-c", script, "T-54", library)
	cmd.Dir = t.TempDir()
	cmd.WaitDelay = time.Second
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	requireFixture(t, ctx.Err() == nil, "warm-up wrapper exceeded outer ten-second deadline")
	return stdout.String(), stderr.String(), err
}
