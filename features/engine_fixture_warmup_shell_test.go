package features

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
	cmd := exec.CommandContext(ctx, "bash", "-euo", "pipefail", "-c", "jq() { command jq --binary \"$@\"; }\n"+script, "T-55", filepath.ToSlash(library))
	cmd.Dir = t.TempDir()
	// Git Bash otherwise rewrites Linux container paths in jq --arg values
	// before calling native jq, and native jq emits CRLF into shell variables.
	cmd.Env = setEnv(os.Environ(), "MSYS_NO_PATHCONV", "1")
	cmd.Env = setEnv(cmd.Env, "MSYS2_ARG_CONV_EXCL", "*")
	cmd.WaitDelay = time.Second
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	requireFixture(t, ctx.Err() == nil, "warm-up wrapper exceeded outer ten-second deadline")
	return stdout.String(), stderr.String(), err
}
