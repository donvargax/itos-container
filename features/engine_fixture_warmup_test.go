package features

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
)

const fixtureWarmupCall = `engine_warmup_run 9s 1s '10 seconds' bash -euo pipefail -c "$(declare -f); warmup_fixture \"\$1\" \"\$2\" \"\$3\"" _ "$reference" "$fixture_uid" "$fixture_gid" || fail "warm-up phase: expected completion; observed status=$?"`

const fixtureProductWarmupCall = `engine_warmup_product_run 9s 1s '10 seconds' bash -euo pipefail -c "$(declare -f); product_warmup_fixture \"\$1\" \"\$2\" \"\$3\"" _ "$built" "$fixture_uid" "$fixture_gid" || fail "warm-up product phase: expected completion; observed status=$?"`

func fixtureProductPreparationIssues(driver string) []string {
	prepare := fixtureWarmupLines(fixtureFunction(driver, "prepare"))
	plain, product := fixtureWarmupLines(fixtureWarmupCall), fixtureWarmupLines(fixtureProductWarmupCall)
	if strings.Count(prepare, product) != 1 || !strings.Contains("\n"+prepare, "\n"+product) {
		return []string{"prepare must execute exactly one bounded product warm-up after plain warm-up and before publication"}
	}
	if !strings.Contains(prepare, plain) || strings.Index(prepare, product) < strings.Index(prepare, plain) ||
		strings.Index(prepare, `>>"$GITHUB_OUTPUT"`) < strings.Index(prepare, product) || strings.Count(prepare, `>>"$GITHUB_OUTPUT"`) != 1 {
		return []string{"both warm-ups must succeed in order before the sole fixture output publication"}
	}
	return nil
}

func TestEngineFixtureWarmupProduct(t *testing.T) {
	driverBytes, err := os.ReadFile("../tools/bin/engine-fixture")
	requireFixture(t, err == nil, "%v", err)
	driver := string(driverBytes)
	// The first assertion checks the executable path before calling any new helper.
	requireFixture(t, len(fixtureProductPreparationIssues(driver)) == 0, "%s", strings.Join(fixtureProductPreparationIssues(driver), "\n"))
	for _, tc := range []struct{ name, replacement string }{
		{"missing", ""}, {"duplicate", fixtureProductWarmupCall + "\n" + fixtureProductWarmupCall},
		{"commented", "# " + fixtureProductWarmupCall}, {"unused string", "unused='" + fixtureProductWarmupCall + "'"},
		{"unbounded", strings.ReplaceAll(fixtureProductWarmupCall, "engine_warmup_product_run 9s 1s '10 seconds'", "bash")},
		{"wrong bound", strings.ReplaceAll(fixtureProductWarmupCall, "9s 1s", "19s 2s")},
		{"swallowed failure", strings.ReplaceAll(fixtureProductWarmupCall, `|| fail "warm-up product phase: expected completion; observed status=$?"`, "|| true")},
		{"wrong image", strings.ReplaceAll(fixtureProductWarmupCall, `"$built"`, `"$base_image"`)},
		{"early publication", `printf 'image=bad' >>"$GITHUB_OUTPUT"` + "\n" + fixtureProductWarmupCall},
	} {
		t.Run(tc.name, func(t *testing.T) {
			broken := strings.ReplaceAll(driver, fixtureProductWarmupCall, tc.replacement)
			requireFixture(t, broken != driver && len(fixtureProductPreparationIssues(broken)) > 0, "accepted broken product preparation")
		})
	}
	t.Run("late", func(t *testing.T) {
		broken := strings.ReplaceAll(driver, fixtureProductWarmupCall, "")
		broken = strings.ReplaceAll(broken, `>>"$GITHUB_OUTPUT"`, `>>"$GITHUB_OUTPUT"`+"\n"+fixtureProductWarmupCall)
		requireFixture(t, len(fixtureProductPreparationIssues(broken)) > 0, "accepted late product phase")
	})
	t.Run("source obligations", func(t *testing.T) { fixtureProductSourceControls(t, driver) })
	t.Run("request shape", testFixtureProductRequest)
	t.Run("ownership", testFixtureProductOwnership)
	t.Run("response", testFixtureProductResponse)
	t.Run("exact absence", testFixtureProductAbsence)
	t.Run("phase ordering", func(t *testing.T) { testFixtureProductOrdering(t, driver) })
}

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
	for _, phase := range []string{"plain", "product"} {
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
			{"timing output failure", fixtureWarmupClock("1234", "4567") + `engine_warmup_timing() { return 23; }; engine_warmup_run 1s 1s '2 seconds' true`, "", "failed", "timing output:"},
		} {
			t.Run(phase+"/"+tc.name, func(t *testing.T) {
				if tc.duration == "measured" {
					if _, err := os.ReadFile("/proc/uptime"); err != nil {
						t.Skip("requires the Linux monotonic uptime source")
					}
					if _, err := exec.LookPath("timeout"); err != nil {
						t.Skip("requires coreutils timeout")
					}
				}
				if phase == "product" {
					tc.script = strings.ReplaceAll(tc.script, "engine_warmup_run", "engine_warmup_product_run")
				}
				started := time.Now()
				stdout, stderr, err := runFixtureWarmupShell(t, "source \"$1\"\n"+tc.script)
				elapsed := time.Since(started)
				requireFixture(t, (err == nil) == (tc.outcome == "passed"), "untruthful result: %v stdout=%s stderr=%s", err, stdout, stderr)
				if tc.diagnostic != "" {
					requireFixture(t, strings.Contains(stderr, "engine-fixture: warm-up "+phase+" "+tc.diagnostic), "missing distinct failure diagnostic: %s", stderr)
				}
				if tc.duration == "" {
					requireFixture(t, stdout == "", "failed output unexpectedly emitted: %s", stdout)
					return
				}
				label := "engine warm-up timing"
				if phase == "product" {
					label = "engine product warm-up timing"
				}
				match := regexp.MustCompile(`^` + label + `: duration_ms=([0-9]+|unknown) outcome=(passed|failed)\n$`).FindStringSubmatch(stdout)
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
		t.Run(phase+"/timing write failure", func(t *testing.T) {
			if _, err := os.Stat("/dev/full"); err != nil {
				t.Skip("requires the always-full device")
			}
			function := "engine_warmup_run"
			if phase == "product" {
				function = "engine_warmup_product_run"
			}
			stdout, stderr, err := runFixtureWarmupShell(t, "source \"$1\"\n"+fixtureWarmupClock("1234", "4567")+function+` 1s 1s '2 seconds' true >/dev/full`)
			requireFixture(t, err != nil && stdout == "" && strings.Contains(stderr, "engine-fixture: warm-up "+phase+" timing output:"), "timing write failure was hidden: %v %s %s", err, stdout, stderr)
		})
	}
}

func fixtureWarmupClock(first, second string) string {
	// Clock/output and ordering controls run only harmless commands. Real child
	// termination is checked separately by the measured cases with coreutils.
	return `timeout() { shift 3; "$@"; }
engine_warmup_now() {
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
	cmd := exec.CommandContext(ctx, "bash", "-euo", "pipefail", "-c", script, "T-55", library)
	cmd.Dir = t.TempDir()
	cmd.WaitDelay = time.Second
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	requireFixture(t, ctx.Err() == nil, "warm-up wrapper exceeded outer ten-second deadline")
	return stdout.String(), stderr.String(), err
}

// Pin the actual acquisition/cleanup call sites, including their arguments and
// failure propagation. Comments, dead strings and a second plain create cannot
// supply any of these executable lines.
func fixtureProductRules() []struct{ function, line string } {
	return []struct{ function, line string }{
		{"product_warmup_fixture", `digest_valid "$image" || fail 'warm-up product image: expected immutable build.iid'`},
		{"product_warmup_fixture", `source=$(cat "$state/product.source") || fail 'warm-up product source record missing'`},
		{"product_warmup_fixture", `device=$(cat "$state/product.device") || fail 'warm-up product device record missing'`},
		{"product_warmup_fixture", `inode=$(cat "$state/product.inode") || fail 'warm-up product inode record missing'`},
		{"product_warmup_fixture", `if product_inspect container "$product_name" "$state/product-collision.json"; then`},
		{"product_warmup_fixture", `if product_inspect volume "$product_volume" "$state/product-volume-collision.json"; then`},
		{"product_warmup_fixture", `printf '%s\n' "$product_volume" >"$state/product-volume.intent" || fail 'warm-up product volume intent recording failed'`},
		{"product_warmup_fixture", `labels=$(engine_warmup_anchor_labels "$owner" "$device" "$inode") || fail 'warm-up product anchor labels failed'`},
		{"product_warmup_fixture", `jq -er 'to_entries[] | "--label", (.key+"="+.value)' <<<"$labels" >"$state/product-labels.args" || fail 'warm-up product anchor label arguments failed'`},
		{"product_warmup_fixture", `mapfile -t label_args <"$state/product-labels.args" || fail 'warm-up product anchor label argument read failed'`},
		{"product_warmup_fixture", `d volume create --driver local --opt type=none --opt o=bind --opt "device=$source" "${label_args[@]}" "$product_volume" >/dev/null || fail 'warm-up product anchor allocation failed'`},
		{"product_warmup_fixture", `printf '%s\n' "$product_name" >"$state/product.intent" || fail 'warm-up product container intent recording failed'`},
		{"product_warmup_fixture", `engine_warmup_request "$image" "$fixture_uid" "$fixture_gid" "$product_volume" "$owner" "$state/cached-image.json" >"$state/product-request.json" || fail 'warm-up product request construction failed'`},
		{"product_warmup_fixture", `http=$(curl -q --fail --silent --show-error --noproxy '*' --proxy '' --proto '=http' --unix-socket /var/run/docker.sock --header 'Content-Type: application/json' --data-binary "@$state/product-request.json" --output "$state/product-response.json" --write-out '%{http_code}' "http://localhost/containers/create?name=$product_name") || fail 'warm-up product create transport failed'`},
		{"product_warmup_fixture", `cid=$(engine_warmup_response "$http" "$state/product-response.json") || fail 'warm-up product create: expected HTTP 201 and full ID'`},
		{"product_warmup_fixture", `printf '%s\n' "$cid" >"$state/product.cid" || fail 'warm-up product CID recording failed'`},
		{"product_warmup_fixture", `remove_product_container || fail 'warm-up product container teardown failed'`},
		{"product_warmup_fixture", `remove_product_volume || fail 'warm-up product anchor teardown failed'`},
		{"remove_product_container", `[[ -f $state/product.intent ]] || return 0`},
		{"remove_product_container", `[[ $name == "$product_name" ]] || fail 'warm-up product container intent name mismatch'`},
		{"remove_product_container", `image=$(cat "$state/build.iid") || fail 'warm-up product image read failed'`},
		{"remove_product_container", `digest_valid "$image" || fail 'warm-up product immutable image invalid'`},
		{"remove_product_container", `cid=$name`},
		{"remove_product_container", `if [[ -s $state/product.cid ]]; then`},
		{"remove_product_container", `[[ $cid =~ ^[0-9a-f]{64}$ ]] || fail 'warm-up product CID invalid'`},
		{"remove_product_container", `if product_inspect container "$cid" "$state/product-container.json"; then`},
		{"remove_product_container", `engine_warmup_container_owned "$state/product-container.json" "$name" "$image" "$owner" "$product_volume" || fail 'warm-up product container ownership or mounts mismatch'`},
		{"remove_product_container", `[[ $observed =~ ^[0-9a-f]{64}$ && ( $cid == "$name" || $cid == "$observed" ) ]] || fail 'warm-up product inspected ID mismatch'`},
		{"remove_product_container", `d container rm --force "$observed" || fail 'warm-up product container removal failed'`},
		{"remove_product_container", `if product_inspect container "$observed" "$state/product-container-removed.json"; then`},
		{"remove_product_container", `fail 'warm-up product container remains after removal'`},
		{"remove_product_volume", `[[ -f $state/product-volume.intent ]] || return 0`},
		{"remove_product_volume", `[[ $name == "$product_volume" ]] || fail 'warm-up product volume intent name mismatch'`},
		{"remove_product_volume", `source=$(cat "$state/product.source") || fail 'warm-up product source read failed'`},
		{"remove_product_volume", `device=$(cat "$state/product.device") || fail 'warm-up product device read failed'`},
		{"remove_product_volume", `inode=$(cat "$state/product.inode") || fail 'warm-up product inode read failed'`},
		{"remove_product_volume", `engine_warmup_volume_owned "$state/product-volume.json" "$name" "$owner" "$source" "$device" "$inode" || fail 'warm-up product volume ownership or source/options mismatch'`},
		{"remove_product_volume", `d volume rm "$name" || fail 'warm-up product volume removal failed'`},
		{"remove_product_volume", `if product_inspect volume "$name" "$state/product-volume-removed.json"; then`},
		{"remove_product_volume", `fail 'warm-up product volume remains after removal'`},
		{"product_inspect", `if d "$kind" inspect "$name" >"$output" 2>"$state/product.err"; then return 0; fi`},
		{"product_inspect", `if engine_warmup_absence "$kind" "$name" "$state/product.err"; then return 1; fi`},
		{"product_inspect", `fail "warm-up product $kind inspection: expected exact identity/absence; observed transport or permission failure"`},
		{"cleanup", `(remove_product_container) || failed=1`},
		{"cleanup", `(remove_product_volume) || failed=1`},
		{"prepare", `printf '%s\n' "$product_source" >"$state/product.source"`},
		{"prepare", `stat -Lc '%d' "$product_source" >"$state/product.device"`},
		{"prepare", `stat -Lc '%i' "$product_source" >"$state/product.inode"`},
	}
}

func fixtureProductSourceIssues(driver string) []string {
	issues := fixtureProductPreparationIssues(driver)
	for _, rule := range fixtureProductRules() {
		body := fixtureWarmupLines(fixtureFunction(driver, rule.function))
		if strings.Count("\n"+body+"\n", "\n"+fixtureWarmupLines(rule.line)+"\n") != 1 {
			issues = append(issues, "missing executable "+rule.function+": "+rule.line)
		}
	}
	for _, line := range fixtureProductGlobals() {
		if !slices.Contains(strings.Split(fixtureWarmupLines(driver), "\n"), line) {
			issues = append(issues, "missing attempt-bound resource identity: "+line)
		}
	}
	operation := fixtureWarmupLines(fixtureFunction(driver, "product_warmup_fixture"))
	last := -1
	for _, stage := range []string{`if product_inspect container`, `if product_inspect volume`,
		`printf '%s\n' "$product_volume"`, `d volume create`, `printf '%s\n' "$product_name"`,
		`engine_warmup_request`, `http=$(curl`, `cid=$(engine_warmup_response`,
		`printf '%s\n' "$cid"`, `remove_product_container`, `remove_product_volume`} {
		at := strings.Index("\n"+operation, "\n"+stage)
		if at <= last {
			issues = append(issues, "product acquisition/teardown out of order: "+stage)
		}
		last = at
	}
	for _, function := range []string{"product_warmup_fixture", "cleanup"} {
		body := fixtureFunction(driver, function)
		if strings.Index(body, "remove_product_container") > strings.Index(body, "remove_product_volume") {
			issues = append(issues, "container must be removed before its anchor")
		}
	}
	// The supervisor must hold the descriptor through the timeout child and its
	// failure trap. Opening it inside the killed child would drop it too soon.
	main := driver[strings.LastIndex(driver, "\nif [[ $1 == prepare ]]; then"):]
	for _, line := range []string{
		`trap finish_preparation EXIT`,
		`mkdir -- "$product_checkout" || fail 'warm-up product empty checkout creation failed'`,
		`exec {product_fd}<"$product_checkout" || fail 'warm-up product checkout descriptor failed'`,
		`product_source="/proc/$$/fd/$product_fd"`, `export product_source`,
		`timeout --signal=TERM --kill-after=1s 179s bash -euo pipefail -c "$(declare -f); prepare"`,
	} {
		if !slices.Contains(strings.Split(fixtureWarmupLines(main), "\n"), line) {
			issues = append(issues, "missing supervisor descriptor/prepare bound: "+line)
		}
	}
	return issues
}

func fixtureProductGlobals() []string {
	return []string{
		`product_name="itos-fixture-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}-product"`,
		`product_volume="itos-fixture-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}-anchor"`,
		`product_checkout="$runner_temp/$owner-product-checkout"`,
		`[[ $product_checkout != "$workspace" && $product_checkout != "$workspace/"* ]] || fail 'warm-up product checkout must be outside the repository'`,
		`export product_name product_volume product_checkout`,
	}
}

func fixtureProductSourceControls(t *testing.T, driver string) {
	t.Helper()
	requireFixture(t, len(fixtureProductSourceIssues(driver)) == 0, "%s", strings.Join(fixtureProductSourceIssues(driver), "\n"))
	// Delete each entire executable requirement. Newlines and continuations in
	// the driver are normalized first, so this also removes multi-line calls.
	for _, rule := range fixtureProductRules() {
		body := fixtureFunction(driver, rule.function)
		brokenBody := strings.ReplaceAll(fixtureWarmupLines(body), fixtureWarmupLines(rule.line), "# removed")
		broken := strings.Replace(driver, body, brokenBody+"\n", 1)
		requireFixture(t, broken != driver && len(fixtureProductSourceIssues(broken)) > 0, "accepted omitted %s", rule.line)
	}
	for _, line := range fixtureProductGlobals() {
		broken := strings.ReplaceAll(driver, line, "# removed")
		requireFixture(t, broken != driver && len(fixtureProductSourceIssues(broken)) > 0, "accepted missing identity %s", line)
	}
	for _, tc := range []struct{ old, replacement string }{
		{`--opt type=none`, `--opt type=tmpfs`}, {`--opt o=bind`, `--opt o=rw`},
		{`--opt "device=$source"`, `--opt "device=$workspace"`},
		{`--unix-socket /var/run/docker.sock`, `--unix-socket /tmp/other.sock`},
		{`curl -q --fail`, `curl -q`}, {`--noproxy '*' --proxy ''`, ``},
		{`engine_warmup_request "$image"`, `engine_warmup_request "$base_image"`},
		{`engine_warmup_container_owned "$state/product-container.json"`, `true`},
		{`engine_warmup_volume_owned "$state/product-volume.json"`, `true`},
		{`if engine_warmup_absence "$kind" "$name" "$state/product.err"; then return 1; fi`, `return 1`},
		{`(remove_product_container) || failed=1`, `(remove_product_container) || true`},
		{`(remove_product_volume) || failed=1`, `(remove_product_volume) || true`},
	} {
		broken := strings.ReplaceAll(driver, tc.old, tc.replacement)
		requireFixture(t, broken != driver && len(fixtureProductSourceIssues(broken)) > 0, "accepted altered %s", tc.old)
	}
}

func fixtureProductLabels() map[string]string {
	return map[string]string{"io.itos.engine-fixture": "T-55-owner", "io.itos-container.intent": "T-55-owner-product",
		"io.itos-container.project": "T-55-owner-project", "io.itos-container.fingerprint": "T-55-owner-fingerprint"}
}

func fixtureProductExpected() container.CreateRequest {
	pids, init := int64(4096), false
	return container.CreateRequest{
		Config: &container.Config{Image: "sha256:" + strings.Repeat("a", 64), User: "1001:1002", WorkingDir: "/workspace",
			Cmd: []string{"/bin/true"}, Entrypoint: []string{}, Env: []string{"HOME=/tmp", "PATH=/bin", "TMPDIR=/tmp", "VALUE=a=b"},
			AttachStdin: true, AttachStdout: true, AttachStderr: true, OpenStdin: true, StdinOnce: true,
			Healthcheck: &container.HealthConfig{Test: []string{"NONE"}}, Labels: fixtureProductLabels()},
		HostConfig: &container.HostConfig{NetworkMode: "none", ReadonlyRootfs: true, CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges"},
			Resources: container.Resources{PidsLimit: &pids}, RestartPolicy: container.RestartPolicy{Name: "no"},
			IpcMode: "private", CgroupnsMode: "private", Init: &init, LogConfig: container.LogConfig{Type: "none"},
			Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: "T-55-anchor", Target: "/workspace", VolumeOptions: &mount.VolumeOptions{NoCopy: true}}},
			Tmpfs:  map[string]string{"/tmp": "rw,exec,nosuid,nodev,mode=1777"}},
	}
}

func fixtureWarmupData(name, data string) string {
	return "cat >" + name + " <<'T55_DATA'\n" + data + "\nT55_DATA\n"
}

func fixtureProductShape(t *testing.T, data string) bool {
	t.Helper()
	var actual, expected any
	if json.Unmarshal([]byte(data), &actual) != nil {
		return false
	}
	requireFixture(t, json.Unmarshal([]byte(fixtureJSON(t, fixtureProductExpected())), &expected) == nil, "invalid expected request")
	return reflect.DeepEqual(actual, expected)
}

func testFixtureProductRequest(t *testing.T) {
	inspect := `[{"Config":{"Env":["PATH=/old","VALUE=a=b","PATH=/bin","HOME=/wrong","TMPDIR=/wrong"]}}]`
	stdout, stderr, err := runFixtureWarmupShell(t, "source \"$1\"\n"+fixtureWarmupData("image.json", inspect)+
		`engine_warmup_request 'sha256:`+strings.Repeat("a", 64)+`' 1001 1002 T-55-anchor T-55-owner image.json`)
	requireFixture(t, err == nil && fixtureProductShape(t, stdout), "request differs from offline SDK CreateRun shape: %v %s %s", err, stdout, stderr)
	// Independently omit every wire field, including every hardening/resource
	// field and the workspace NoCopy mount, rather than checking only keywords.
	var decoded map[string]any
	requireFixture(t, json.Unmarshal([]byte(stdout), &decoded) == nil, "invalid request JSON")
	for key := range decoded {
		var broken map[string]any
		requireFixture(t, json.Unmarshal([]byte(stdout), &broken) == nil, "invalid request")
		delete(broken, key)
		requireFixture(t, !fixtureProductShape(t, fixtureJSON(t, broken)), "accepted omitted request %s", key)
	}
	for key := range decoded["HostConfig"].(map[string]any) {
		var broken map[string]any
		requireFixture(t, json.Unmarshal([]byte(stdout), &broken) == nil, "invalid request")
		delete(broken["HostConfig"].(map[string]any), key)
		requireFixture(t, !fixtureProductShape(t, fixtureJSON(t, broken)), "accepted omitted HostConfig %s", key)
	}
	for _, replacement := range []string{`[]`, `[{"Type":"bind","Source":"/checkout","Target":"/workspace"}]`, `[{"Type":"volume","Target":"/workspace"}]`} {
		var broken map[string]any
		requireFixture(t, json.Unmarshal([]byte(stdout), &broken) == nil, "invalid request")
		var mounts any
		requireFixture(t, json.Unmarshal([]byte(replacement), &mounts) == nil, "invalid control")
		broken["HostConfig"].(map[string]any)["Mounts"] = mounts
		requireFixture(t, !fixtureProductShape(t, fixtureJSON(t, broken)), "accepted wrong mounts")
	}
	requireFixture(t, !fixtureProductShape(t, `{"Image":"sha256:`+strings.Repeat("a", 64)+`","User":"1001:1002","HostConfig":{"NetworkMode":"none","ReadonlyRootfs":true,"CapDrop":["ALL"],"SecurityOpt":["no-new-privileges"]}}`), "accepted a second plain create")
}

func testFixtureProductResponse(t *testing.T) {
	for _, tc := range []struct {
		status, body string
		valid        bool
	}{
		{"201", `{"Id":"` + strings.Repeat("b", 64) + `"}`, true},
		{"500", `{"Id":"` + strings.Repeat("b", 64) + `"}`, false},
		{"000", `{"Id":"` + strings.Repeat("b", 64) + `"}`, false},
		{"200", `{"Id":"` + strings.Repeat("b", 64) + `"}`, false},
		{"201", `{"Id":"short"}`, false}, {"201", `{"Id":null}`, false},
		{"201", `{"message":"denied"}`, false}, {"201", `invalid`, false},
	} {
		stdout, _, err := runFixtureWarmupShell(t, "source \"$1\"\n"+fixtureWarmupData("response.json", tc.body)+`engine_warmup_response `+tc.status+` response.json`)
		requireFixture(t, (err == nil) == tc.valid, "wrong HTTP/ID verdict for %s %s", tc.status, tc.body)
		if tc.valid {
			requireFixture(t, stdout == strings.Repeat("b", 64)+"\n", "wrong returned ID")
		}
	}
}

func testFixtureProductOwnership(t *testing.T) {
	labels := fixtureProductLabels()
	anchorLabels := fixtureProductLabels()
	for key, value := range map[string]string{"session": "T-55-owner-session", "device": "123", "inode": "456", "kind": "checkout-anchor"} {
		anchorLabels["io.itos-container."+key] = value
	}
	containerData := map[string]any{"Name": "/T-55-product", "Image": "sha256:" + strings.Repeat("a", 64), "Config": map[string]any{"Labels": labels},
		"HostConfig": fixtureProductExpected().HostConfig, "Mounts": []any{
			map[string]any{"Type": "volume", "Name": "T-55-anchor", "Driver": "local", "Destination": "/workspace", "RW": true, "Propagation": "rprivate"},
			map[string]any{"Type": "tmpfs", "Destination": "/tmp"}}}
	volumeData := map[string]any{"Name": "T-55-anchor", "Driver": "local", "Scope": "local", "Labels": anchorLabels,
		"Options": map[string]string{"type": "none", "o": "bind", "device": "/proc/123/fd/10"}}
	for _, kind := range []string{"container", "volume"} {
		data, call := containerData, `engine_warmup_container_owned input.json T-55-product 'sha256:`+strings.Repeat("a", 64)+`' T-55-owner T-55-anchor`
		if kind == "volume" {
			data, call = volumeData, `engine_warmup_volume_owned input.json T-55-anchor T-55-owner /proc/123/fd/10 123 456`
		}
		original := fixtureJSON(t, data)
		check := func(data string, want bool) {
			t.Helper()
			_, stderr, err := runFixtureWarmupShell(t, "source \"$1\"\n"+fixtureWarmupData("input.json", "["+data+"]")+call)
			requireFixture(t, (err == nil) == want, "wrong %s ownership verdict for %s: %v %s", kind, data, err, stderr)
		}
		check(original, true)
		for key := range data {
			var broken map[string]any
			requireFixture(t, json.Unmarshal([]byte(original), &broken) == nil, "invalid ownership fixture")
			delete(broken, key)
			check(fixtureJSON(t, broken), false)
		}
		for _, replacement := range []struct{ old, new string }{
			{`T-55-owner`, `foreign`}, {`T-55-anchor`, `foreign-volume`},
			{`"local"`, `"foreign-driver"`},
		} {
			check(strings.ReplaceAll(original, replacement.old, replacement.new), false)
		}
		if kind == "container" {
			for _, replacement := range []struct{ old, new string }{
				{`/T-55-product`, `/foreign`}, {strings.Repeat("a", 64), strings.Repeat("c", 64)},
				{`"RW":true`, `"RW":false`}, {`"volume"`, `"bind"`},
				{`"NoCopy":true`, `"NoCopy":false`}, {`rw,exec,nosuid,nodev,mode=1777`, `rw`},
				{`"/tmp"`, `"/elsewhere"`},
			} {
				check(strings.ReplaceAll(original, replacement.old, replacement.new), false)
			}
			var broken map[string]any
			requireFixture(t, json.Unmarshal([]byte(original), &broken) == nil, "invalid fixture")
			broken["Mounts"] = append(broken["Mounts"].([]any), map[string]any{"Type": "bind", "Source": "/home", "Destination": "/host"})
			check(fixtureJSON(t, broken), false)
			// Docker may omit HostConfig.Tmpfs from Mounts before start.
			broken["Mounts"] = broken["Mounts"].([]any)[:1]
			check(fixtureJSON(t, broken), true)
		} else {
			for _, replacement := range []struct{ old, new string }{
				{`/proc/123/fd/10`, `/checkout`}, {`"none"`, `"tmpfs"`},
				{`"bind"`, `"rw"`}, {`"123"`, `"987"`}, {`"456"`, `"789"`},
			} {
				check(strings.ReplaceAll(original, replacement.old, replacement.new), false)
			}
		}
	}
}

func testFixtureProductOrdering(t *testing.T, driver string) {
	// Execute only the two real prepare-path call statements, substituting their
	// child commands with harmless exits. No driver, Docker or approval executes.
	// The clock and timeout shim keep this sequencing check portable.
	for _, tc := range []struct {
		plain, product string
		lines          int
		published      bool
	}{
		{"0", "0", 2, true}, {"7", "0", 1, false}, {"0", "8", 2, false},
	} {
		calls := fixtureWarmupCall + "\n" + fixtureProductWarmupCall
		calls = strings.ReplaceAll(calls, `"$(declare -f); warmup_fixture \"\$1\" \"\$2\" \"\$3\"" _ "$reference" "$fixture_uid" "$fixture_gid"`, `'exit `+tc.plain+`'`)
		calls = strings.ReplaceAll(calls, `"$(declare -f); product_warmup_fixture \"\$1\" \"\$2\" \"\$3\"" _ "$built" "$fixture_uid" "$fixture_gid"`, `'exit `+tc.product+`'`)
		requireFixture(t, strings.Contains(driver, fixtureWarmupCall) && strings.Contains(driver, fixtureProductWarmupCall), "not the real calls")
		stdout, _, err := runFixtureWarmupShell(t, "source \"$1\"\nfail() { exit 1; }\n"+fixtureWarmupClock("1234", "1234")+calls+"\nprintf 'published\\n'\n")
		requireFixture(t, (err == nil) == tc.published && strings.Contains(stdout, "published") == tc.published, "failure published outputs: %v %s", err, stdout)
		requireFixture(t, strings.Count(stdout, "timing:") == tc.lines && strings.HasPrefix(stdout, "engine warm-up timing:"), "wrong attempted phases: %s", stdout)
		if tc.lines == 2 {
			requireFixture(t, strings.Contains(stdout, "\nengine product warm-up timing:"), "missing product timing: %s", stdout)
		}
	}
}

func testFixtureProductAbsence(t *testing.T) {
	for _, kind := range []string{"container", "volume"} {
		for _, tc := range []struct {
			message string
			absent  bool
		}{
			{"Error: No such " + kind + ": T-55-resource", true},
			{"Error: No such " + kind + ": foreign", false},
			{"permission denied", false}, {"Cannot connect to the Docker daemon", false},
			{"", false}, {"Error: No such object: T-55-resource", kind == "container"},
			{"Error response from daemon: get T-55-resource: no such volume", kind == "volume"},
		} {
			_, _, err := runFixtureWarmupShell(t, "source \"$1\"\n"+fixtureWarmupData("error.txt", tc.message)+`engine_warmup_absence `+kind+` T-55-resource error.txt`)
			requireFixture(t, (err == nil) == tc.absent, "wrong %s absence for %q", kind, tc.message)
		}
	}
	_, _, err := runFixtureWarmupShell(t, "source \"$1\"\nengine_warmup_absence volume T-55-resource missing.txt")
	requireFixture(t, err != nil, "error read failure treated as absence")
}
