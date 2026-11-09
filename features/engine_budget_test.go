package features

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

func TestEngineSuiteBudget(t *testing.T) {
	t.Run("compiled fixture budgets", testEngineBudgetFixture)
	t.Run("quiet observation", TestEngineFixtureQuietBudget)
	t.Run("grace repetitions", TestEngineFixtureGraceBudget)
	data, err := os.ReadFile("../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow engineWorkflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	engine := workflow.Jobs["engine"]
	var step workflowStep
	for _, candidate := range engine.Steps {
		if candidate.Name == "Engine scenarios" {
			step = candidate
		}
	}
	t.Run("restored budgets and full selection", func(t *testing.T) {
		if engine.Timeout != 10 {
			t.Errorf("engine job budget = %d minutes, want 10", engine.Timeout)
		}
		if step.Timeout != 5 {
			t.Errorf("engine step budget = %d minutes, want 5", step.Timeout)
		}
		if !strings.Contains(step.Run, "-test.timeout=4m ") {
			t.Error("missing four-minute Go test timeout")
		}
		// The existing parsed-workflow checks retain the exact complete command,
		// event guards, pins, fixture, diagnostics, failure propagation and cleanup.
		for _, issue := range engineWorkflowIssues(data) {
			t.Error(issue)
		}
	})
	testEngineBudgetScript(t, runtime.GOOS, step.Run)
}

func testEngineBudgetScript(t *testing.T, hostOS, script string) {
	t.Helper()
	if hostOS != "linux" {
		t.Skip("the native workflow wrapper uses Linux uptime and setsid")
	}
	for _, tc := range []struct {
		name, finish, clock, compileClock string
		compileExit, captureExit, want    int
	}{
		{name: "below budget", clock: "279.99", want: 0},
		{name: "exact budget", clock: "280.00", want: 0},
		{name: "successful but over budget", clock: "280.01", want: 1},
		{name: "capture time counts", clock: "281.00", want: 1},
		{name: "compilation time counts", compileClock: "279.00", clock: "281.00", want: 1},
		{name: "missing final timing", clock: "", want: 1},
		{name: "invalid final timing", clock: "invalid", want: 1},
		{name: "backwards timing", clock: "99.00", want: 1},
		{name: "missing compilation timing", compileClock: "invalid", clock: "110.00", want: 1},
		{name: "compile failure", compileExit: 31, clock: "110.00", want: 31},
		{name: "test failure", finish: "exit 7", clock: "110.00", want: 7},
		{name: "over budget preserves test failure", finish: "exit 7", clock: "281.00", want: 7},
		{name: "missing timing preserves test failure", finish: "exit 7", want: 7},
		{name: "capture failure", captureExit: 29, clock: "110.00", want: 29},
		{name: "test and capture failure", finish: "exit 7", captureExit: 29, clock: "110.00", want: 7},
		{name: "signal exit", finish: `kill -TERM "$$"`, clock: "110.00", want: 143},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range []string{"bin", "features"} {
				if err := os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
					t.Fatal(err)
				}
			}
			compileClock := tc.compileClock
			if compileClock == "" {
				compileClock = "102.00"
			}
			// The actual wrapper reads monotonic /proc/uptime with a shell builtin.
			// Replace just that read primitive; every compile, test, pipe, exit trap
			// and budget decision below still comes from the actual workflow.
			clock := `read() {
  if [[ "$*" != '-r engine_uptime _' ]]; then builtin read "$@"; return; fi
  local index value
  index=$(<"$RUNNER_TEMP/clock-index")
  value=$(<"$RUNNER_TEMP/clock-$index")
  printf '%s' "$((index + 1))" > "$RUNNER_TEMP/clock-index"
  if (( index == 0 )); then [[ ! -f $RUNNER_TEMP/compiled ]] || return 1; fi
  if (( index == 1 )); then [[ -f $RUNNER_TEMP/compiled && ! -f $RUNNER_TEMP/ran ]] || return 1; fi
  if (( index == 2 )) && [[ $(<"$RUNNER_TEMP/expect-capture") == yes ]]; then
    [[ -f $RUNNER_TEMP/captured ]] || return 1
  fi
  [[ -n $value ]] || return 1
  printf -v engine_uptime '%s' "$value"
}
`
			compiler := fmt.Sprintf(`#!/bin/bash
set -euo pipefail
[[ $# = 5 && "$1" = test && "$2" = -c && "$3" = -o && "$4" = "$RUNNER_TEMP/slice-11-features.test" && "$5" = ./features ]]
printf 'compiled\n' > "$RUNNER_TEMP/compiled"
if (( %d != 0 )); then exit %d; fi
cp "$RUNNER_TEMP/program" "$4"
`, tc.compileExit, tc.compileExit)
			program := "#!/bin/bash\nset -euo pipefail\nprintf 'ran\\n' > \"$RUNNER_TEMP/ran\"\nprintf 'test output\\n'\n" + tc.finish + "\n"
			capture := fmt.Sprintf("#!/bin/bash\ncat > \"$1\"\ncat \"$1\"\nprintf 'captured\\n' > \"$RUNNER_TEMP/captured\"\nexit %d\n", tc.captureExit)
			files := map[string]string{
				"bin/go": compiler, "program": program, "bin/tee": capture,
				"clock-index": "0", "clock-0": "100.00", "clock-1": compileClock, "clock-2": tc.clock,
				"expect-capture": "yes",
			}
			if tc.compileExit != 0 || compileClock == "invalid" {
				files["expect-capture"] = "no"
			}
			for name, contents := range files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0700); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "bash", "-euo", "pipefail", "-c", clock+script)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "RUNNER_TEMP="+dir, "PATH="+filepath.Join(dir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			if got := diagnosticExitCode(err); got != tc.want {
				t.Errorf("runner exit = %d, want %d: %s", got, tc.want, out)
			}
			if !strings.Contains(string(out), "engine compilation timing:") || !strings.Contains(string(out), "engine step timing:") {
				t.Errorf("missing compilation or whole-step timing: %s", out)
			}
			if tc.want == 0 {
				elapsed, err := time.ParseDuration(tc.clock + "s")
				if err != nil {
					t.Fatal(err)
				}
				want := fmt.Sprintf("engine step timing: duration_ms=%d outcome=passed", (elapsed - 100*time.Second).Milliseconds())
				if !strings.Contains(string(out), "engine compilation timing: duration_ms=2000 outcome=passed") || !strings.Contains(string(out), want) {
					t.Errorf("compilation/whole-step intervals changed: %s", out)
				}
			}
			if tc.compileExit != 0 {
				if _, err := os.Stat(filepath.Join(dir, "ran")); !os.IsNotExist(err) {
					t.Error("test ran after failed compilation")
				}
			} else if tc.compileClock != "invalid" {
				if _, err := os.Stat(filepath.Join(dir, "captured")); err != nil {
					t.Error("whole-step result preceded output capture", err)
				}
			}
		})
	}
}
