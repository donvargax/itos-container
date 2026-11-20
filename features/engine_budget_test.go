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
	engine := workflow.Jobs["engine-shard"]
	var step workflowStep
	for _, candidate := range engine.Steps {
		if candidate.Name == "Engine scenarios" {
			step = candidate
		}
	}
	t.Run("restored budgets and full selection", func(t *testing.T) {
		if engine.Timeout != 15 {
			t.Errorf("engine job budget = %d minutes, want 15", engine.Timeout)
		}
		if step.Timeout != 11 {
			t.Errorf("engine step budget = %d minutes, want 11", step.Timeout)
		}
		if !strings.Contains(step.Run, "-test.timeout=10m ") {
			t.Error("missing ten-minute Go test timeout")
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
		warn, guard                       bool
	}{
		{name: "below budget", clock: "279.99", want: 0},
		{name: "exact budget", clock: "280.00", want: 0},
		{name: "successful but over budget", clock: "280.01", want: 0, warn: true},
		{name: "over budget passes", clock: "281.00", want: 0, warn: true},
		{name: "exact guard", clock: "700.00", want: 0, warn: true},
		{name: "capture time counts", clock: "700.01", want: 1, warn: true, guard: true},
		{name: "compilation time counts", compileClock: "699.00", clock: "701.00", want: 1, warn: true, guard: true},
		{name: "missing final timing", clock: "", want: 1},
		{name: "invalid final timing", clock: "invalid", want: 1},
		{name: "backwards timing", clock: "99.00", want: 1},
		{name: "missing compilation timing", compileClock: "invalid", clock: "110.00", want: 1},
		{name: "compile failure", compileExit: 31, clock: "110.00", want: 31},
		{name: "test failure", finish: "exit 7", clock: "110.00", want: 7},
		{name: "over budget preserves test failure", finish: "exit 7", clock: "701.00", want: 7, warn: true, guard: true},
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
			if got := strings.Contains(string(out), "::warning::"); got != tc.warn {
				t.Errorf("warning printed = %v, want %v: %s", got, tc.warn, out)
			}
			if got := strings.Contains(string(out), "::error::"); got != tc.guard {
				t.Errorf("hang guard error printed = %v, want %v: %s", got, tc.guard, out)
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

func TestEngineStepBudget(t *testing.T) {
	testEngineStepBudget(t, runtime.GOOS)
}

// testEngineStepBudget runs the real engine_finish from the Engine scenarios step with
// scripted uptime. The 180 s budget warns; only a passing run past 600 s fails.
func testEngineStepBudget(t *testing.T, hostOS string) {
	t.Helper()
	if hostOS != "linux" {
		t.Skip("the engine step wrapper runs under Linux bash")
	}
	data, err := os.ReadFile("../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow engineWorkflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	job := workflow.Jobs["engine-shard"]
	var step workflowStep
	for _, candidate := range job.Steps {
		if candidate.Name == "Engine scenarios" {
			step = candidate
		}
	}
	const trap = "trap engine_finish EXIT"
	lines := strings.Split(step.Run, "\n")
	cut := -1
	for i, line := range lines {
		if line == trap {
			if cut >= 0 {
				t.Fatal("the trap line appears more than once")
			}
			cut = i
		}
	}
	if cut < 0 {
		t.Fatalf("%q is not one whole line of the Engine scenarios step", trap)
	}
	prefix := strings.Join(lines[:cut], "\n") + "\n"

	for _, tc := range []struct {
		name, now         string
		finish            string
		wantExit          int
		wantMS            string
		wantOutcome       string
		wantWarn, wantErr bool
	}{
		{name: "at_179s", now: "17900", wantMS: "179000", wantOutcome: "passed"},
		{name: "at_180s", now: "18000", wantMS: "180000", wantOutcome: "passed"},
		{name: "at_180_01s", now: "18001", wantMS: "180010", wantOutcome: "passed", wantWarn: true},
		{name: "at_181s", now: "18100", wantMS: "181000", wantOutcome: "passed", wantWarn: true},
		{name: "at_600s", now: "60000", wantMS: "600000", wantOutcome: "passed", wantWarn: true},
		{name: "at_600_01s", now: "60001", wantExit: 1, wantMS: "600010", wantOutcome: "failed", wantWarn: true, wantErr: true},
		{name: "at_601s", now: "60100", wantExit: 1, wantMS: "601000", wantOutcome: "failed", wantWarn: true, wantErr: true},
		{name: "failed_run_at_181s", now: "18100", finish: "7", wantExit: 7, wantMS: "181000", wantOutcome: "failed", wantWarn: true},
		{name: "failed_run_at_601s", now: "60100", finish: "7", wantExit: 7, wantMS: "601000", wantOutcome: "failed", wantWarn: true, wantErr: true},
		{name: "unreadable_uptime", now: "", wantExit: 1, wantMS: "unknown", wantOutcome: "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := "engine_now() { return 1; }"
			if tc.now != "" {
				now = fmt.Sprintf("engine_now() { printf '%%s\\n' %s; }", tc.now)
			}
			finish := "0"
			if tc.finish != "" {
				finish = tc.finish
			}
			script := prefix + now + "\nengine_started=0\n" + trap + "\nexit " + finish + "\n"
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "bash", "--noprofile", "--norc", "-eo", "pipefail", "-c", script)
			cmd.Env = append(os.Environ(), "ITOS_CONTAINER_ENGINE_SHARD=egress")
			out, err := cmd.CombinedOutput()
			text := string(out)
			if got := diagnosticExitCode(err); got != tc.wantExit {
				t.Errorf("exit = %d, want %d: %s", got, tc.wantExit, text)
			}
			if want := fmt.Sprintf("engine step timing: duration_ms=%s outcome=%s", tc.wantMS, tc.wantOutcome); !strings.Contains(text, want) {
				t.Errorf("missing %q: %s", want, text)
			}
			for _, a := range []struct {
				mark string
				want bool
			}{{"::warning::", tc.wantWarn}, {"::error::", tc.wantErr}} {
				got := strings.Count(text, a.mark)
				if a.want && got != 1 || !a.want && got != 0 {
					t.Errorf("%s printed %d times, want present=%v: %s", a.mark, got, a.want, text)
				}
			}
			if tc.wantWarn && !strings.Contains(text, "Engine shard egress took "+tc.wantMS+" ms") {
				t.Errorf("warning does not name the shard and time: %s", text)
			}
			if tc.wantErr && !strings.Contains(text, "hang guard") {
				t.Errorf("error does not name the hang guard: %s", text)
			}
		})
	}

	t.Run("limits", func(t *testing.T) {
		if job.Timeout != 15 {
			t.Errorf("engine-shard job budget = %d minutes, want 15", job.Timeout)
		}
		if step.Timeout != 11 {
			t.Errorf("Engine scenarios step budget = %d minutes, want 11", step.Timeout)
		}
		if !strings.Contains(step.Run, "-test.timeout=10m ") {
			t.Error("missing ten-minute Go test timeout")
		}
		if got := step.Env["ITOS_CONTAINER_ENGINE_SHARD"]; got != "${{ matrix.shard }}" {
			t.Errorf("shard env = %q, want the matrix shard", got)
		}
	})
}
