package features

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

func TestMain(m *testing.M) {
	if os.Getenv("ITOS_CONTAINER_ENGINE_TESTS") == "1" || os.Getenv("ITOS_CONTAINER_SIGNAL_DIAGNOSTIC_CHILD") == "1" {
		logFeatureSignals(runtime.GOOS)
	}
	os.Exit(m.Run())
}

func logFeatureSignals(hostOS string) {
	if hostOS != "linux" {
		return
	}
	received := make(chan os.Signal, 64)
	// This subscription lasts until TestMain exits, including m.Run's cleanup.
	signal.Notify(received)
	fmt.Fprintf(os.Stderr, "features TestMain signal logger ready, pid %d\n", os.Getpid())
	go func() {
		for sig := range received {
			number := sig.(syscall.Signal)
			fmt.Fprintf(os.Stderr, "features TestMain received signal %d (%s), pid %d\n", number, sig, os.Getpid())
			switch number {
			// Linux terminating signals must not turn a killed suite green.
			// SIGKILL/SIGSTOP cannot be caught; the runner/kernel diagnostics
			// provide evidence for those. Runtime panic signals still panic.
			// SIGPIPE stays nonfatal for ordinary network EPIPE handling;
			// the supervising shell separately fails on output-capture errors.
			case syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGILL,
				syscall.SIGTRAP, syscall.SIGABRT, syscall.SIGBUS, syscall.SIGFPE,
				syscall.SIGSEGV, syscall.SIGTERM,
				syscall.Signal(16), syscall.Signal(31): // Linux SIGSTKFLT and SIGSYS.
				os.Exit(128 + int(number))
			case syscall.Signal(20), syscall.Signal(21), syscall.Signal(22): // Linux SIGTSTP, SIGTTIN, SIGTTOU.
				// Retain job-control semantics after recording the notification.
				signal.Reset(sig)
				self, err := os.FindProcess(os.Getpid())
				if err == nil {
					err = self.Signal(sig)
				}
				if err != nil {
					fmt.Fprintf(os.Stderr, "features TestMain could not restore signal: %v\n", err)
					os.Exit(1)
				}
			case syscall.Signal(18): // Linux SIGCONT.
				signal.Notify(received, syscall.Signal(20), syscall.Signal(21), syscall.Signal(22))
			}
		}
	}()
}

func TestEngineDiagnostics(t *testing.T) {
	testEngineDiagnostics(t, runtime.GOOS)
}

func testEngineDiagnostics(t *testing.T, hostOS string) {
	t.Helper()
	if hostOS != "linux" {
		t.Skip("the engine diagnostic runner uses Linux setsid")
	}
	data, err := os.ReadFile("../.github/workflows/ci.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow engineWorkflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	var script string
	for _, step := range workflow.Jobs["engine"].Steps {
		if step.Name == "Engine scenarios" {
			script = step.Run
		}
	}
	if script == "" {
		t.Fatal("missing engine invocation")
	}
	// Execute the actual workflow shell with a disposable compiler/test program.
	// No engine switch, CI spoof, product command or Docker access is involved.
	for _, tc := range []struct {
		name, finish, signal string
		buildExit, wantExit  int
		captureFailure       bool
	}{
		{name: "success", finish: "exit 0"},
		{name: "assertion failure", finish: "exit 7", wantExit: 7},
		{name: "termination", finish: `kill -TERM "$$"`, wantExit: 143, signal: "TERM"},
		{name: "uncatchable kill", finish: `kill -KILL "$$"`, wantExit: 137, signal: "KILL"},
		{name: "compilation failure", buildExit: 31, wantExit: 31},
		{name: "capture failure", finish: "exit 0", captureFailure: true, wantExit: 29},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range []string{"bin", "features"} {
				if err := os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
					t.Fatal(err)
				}
			}
			compiler := fmt.Sprintf(`#!/bin/bash
set -euo pipefail
[[ $# = 5 && "$1" = test && "$2" = -c && "$3" = -o && "$4" = "$RUNNER_TEMP/slice-11-features.test" && "$5" = ./features ]]
exit_code=%d
if (( exit_code != 0 )); then exit "$exit_code"; fi
cp "$RUNNER_TEMP/program" "$4"
`, tc.buildExit)
			program := `#!/bin/bash
set -euo pipefail
[[ "$PWD" = "$RUNNER_TEMP/features" ]]
[[ "$*" = '-test.count=1 -test.v -test.timeout=7m -test.paniconexit0 -test.run ^TestFeatures$' ]]
if [[ $(ps -o sid= -p "$$") -ne $$ ]]; then
  printf 'feature test did not get its own session\n' >&2
  exit 32
fi
printf 'feature stdout marker\n'
printf 'feature stderr marker\n' >&2
` + tc.finish + "\n"
			files := map[string]string{"bin/go": compiler, "program": program}
			if tc.captureFailure {
				files["bin/tee"] = "#!/bin/bash\ncat > \"$1\"\ncat \"$1\"\nexit 29\n"
			}
			for name, contents := range files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0700); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "bash", "-euo", "pipefail", "-c", script)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "RUNNER_TEMP="+dir, "PATH="+filepath.Join(dir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			if code := diagnosticExitCode(err); code != tc.wantExit {
				t.Fatalf("runner exit = %d, want %d: %s", code, tc.wantExit, out)
			}
			if tc.buildExit != 0 {
				if _, err := os.Stat(filepath.Join(dir, "slice-11-features.log")); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("a failed build must not start the test program")
				}
				return
			}
			log, err := os.ReadFile(filepath.Join(dir, "slice-11-features.log"))
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{"feature stdout marker", "feature stderr marker"} {
				if !strings.Contains(string(log), marker) || !strings.Contains(string(out), marker) {
					t.Fatalf("missing captured/streamed %q: log %q, output %q", marker, log, out)
				}
			}
			testExit := tc.wantExit
			if tc.captureFailure {
				testExit = 0
			}
			if !strings.Contains(string(out), fmt.Sprintf("Feature test exit status: %d", testExit)) {
				t.Fatalf("missing exit diagnostic: %s", out)
			}
			if tc.signal != "" && !strings.Contains(string(out), "Feature test signal: "+tc.signal) {
				t.Fatalf("missing signal diagnostic: %s", out)
			}
		})
	}
}

func diagnosticExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exited *exec.ExitError
	if errors.As(err, &exited) {
		return exited.ExitCode()
	}
	return -1
}

func TestFeatureSignalDiagnostics(t *testing.T) {
	testFeatureSignalDiagnostics(t, runtime.GOOS)
}

func testFeatureSignalDiagnostics(t *testing.T, hostOS string) {
	t.Helper()
	if hostOS != "linux" {
		t.Skip("engine signal diagnostics run on Linux")
	}
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-test.run=^TestFeatureSignalDiagnosticChild$")
	cmd.Env = append(os.Environ(), "ITOS_CONTAINER_SIGNAL_DIAGNOSTIC_CHILD=1")
	out, err := cmd.CombinedOutput()
	if diagnosticExitCode(err) != 128+int(syscall.SIGTERM) || !strings.Contains(string(out), "features TestMain received signal 15 (terminated)") {
		t.Fatalf("SIGTERM must be logged and remain fatal: %v, %s", err, out)
	}
}

func TestFeatureSignalDiagnosticChild(t *testing.T) {
	if os.Getenv("ITOS_CONTAINER_SIGNAL_DIAGNOSTIC_CHILD") != "1" {
		return
	}
	self, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	// Signal only this disposable child, never a process group or another PID.
	if err := self.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	<-timer.C
	t.Fatal("TestMain swallowed SIGTERM")
}
