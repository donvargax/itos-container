package features

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

// The names the report cases hang on. diagnosticChild is the test the child
// process runs, so its scenarios are subtests of it as TestFeatures' are.
const (
	diagnosticLogChildEnv = "ITOS_CONTAINER_DIAGNOSTIC_LOG_CHILD"
	diagnosticChild       = "TestEngineDiagnosticsLogChild"
)

// diagnosticQuoted is the error the last scenario of the completed run fails
// with: every line a report must keep inside that scenario's text, at the
// indent go test and godog print it with.
var diagnosticQuoted = []string{
	"quoted 100%2F done\rtail",
	"panic: quoted",
	"",
	"::error title=planted::x",
	"=== RUN   TestNested",
	"=== NAME  TestNested/sub",
	"    --- FAIL: TestNested/sub (0.00s)",
	"--- FAIL: TestNested (0.00s)",
	"    nested_test.go:3: nested block",
}

// diagnosticFirstLineQuote is the error of a scenario whose first line quotes
// go test output, and whose later line quotes a failure line naming a scenario
// that passed: a report that takes the first line for an own failure line drops
// the block, then reads the later line as an own one.
var diagnosticFirstLineQuote = []string{
	"nested go test said --- FAIL: TestX (0.00s)",
	"    --- FAIL: " + diagnosticChild + "/passing_case (0.00s)",
	"more detail",
}

// diagnosticInvalidBytes is the error of a scenario that quotes raw stderr with
// bytes that are no UTF-8, on its first line and on a later one, as the steps
// that print a workload's stderr with %s can: a report that matches in a UTF-8
// locale finds no block start in the first line.
var diagnosticInvalidBytes = []string{
	"completed anchored workload returned 75: \xff\xfe stderr",
	"more \xc3 detail",
}

// outlineRowError is what the nth outline row fails with: a long error of
// short lines, so that go test blocks of failures that overlapped would cut
// one another short, as they do when godog runs scenarios concurrently.
func outlineRowError(row int) string {
	lines := []string{fmt.Sprintf("row %d failed", row)}
	for line := 2; line <= 60; line++ {
		lines = append(lines, fmt.Sprintf("r%d l%02d", row, line))
	}
	return strings.Join(lines, "\n")
}

func diagnosticOutline(rows int) string {
	var feature strings.Builder
	feature.WriteString("@engine\nFeature: rows\n  @ID-MEMORY-01\n  Scenario Outline: failing row\n    Then row <row> fails\n    Examples:\n      | row |\n")
	for row := 1; row <= rows; row++ {
		fmt.Fprintf(&feature, "      | %02d |\n", row)
	}
	return feature.String()
}

const diagnosticCompletedFixture = `@engine
Feature: failures
  @ID-LIFECYCLE-09
  Scenario Outline: escalation, round: one
    Then escalation row <row> is observed
    Examples:
      | row |
      | 1   |
      | 2   |
      | 3   |

  @ID-LIFECYCLE-13
  Scenario: quiet interval
    When the quiet interval is observed

  @ID-MEMORY-01
  Scenario: passing case
    Then it passes

  @ID-MEMORY-02
  Scenario: cleanup failure
    Then it passes

  @ID-MEMORY-03
  Scenario: undefined case
    Then nothing defines this step

  @ID-MEMORY-05
  Scenario: first line quoting case
    Then it fails quoting a failure line first

  @ID-MEMORY-06
  Scenario: invalid bytes case
    Then it fails with invalid bytes

  @ID-MEMORY-04
  Scenario: quoted 100% case
    Then it fails with quoted output
`

const diagnosticTimeoutFixture = `@engine
Feature: timeout
  @ID-MEMORY-01
  Scenario: failing case
    Then it fails with a quoted panic

  @ID-MEMORY-02
  Scenario: sleeping case
    Then it sleeps
`

// TestEngineDiagnosticsLogChild is run only by TestEngineDiagnostics, as a
// separate process, to leave the log a real failing run leaves. It runs
// memory-only steps through the harness as TestFeatures does, and reaches no
// engine, as engine_schedule_regression_test.go does.
func TestEngineDiagnosticsLogChild(t *testing.T) {
	name := os.Getenv(diagnosticLogChildEnv)
	if name == "" {
		return
	}
	var feature string
	switch name {
	case "completed":
		feature = diagnosticCompletedFixture
	case "timeout":
		feature = diagnosticTimeoutFixture
	case "ten":
		feature = diagnosticOutline(10)
	case "eleven":
		feature = diagnosticOutline(11)
	case "test failure":
		t.Logf("excluded 1 scenario(s): logged before the failure")
		t.Fatal("failed with no scenario to blame")
	default:
		t.Fatalf("unknown fixture %q", name)
	}
	dir := writeFeatureFixture(t, feature)
	selection, err := selectScenarios("", dir, featureEnvironment{engine: "1", ci: "true", actions: "true", os: "linux", arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	// Concurrency 4 is TestFeatures's request, which an engine selection runs
	// at 1 (TestFeatureConcurrency): a log with the blocks of overlapping
	// scenarios in it is no log of the engine job, which this report reads.
	code := runFeatureScenarios(selection, godog.Options{
		Format:      "pretty",
		Output:      os.Stdout,
		Paths:       []string{dir},
		Strict:      true,
		TestingT:    t,
		Concurrency: 4,
	}, func(sc *godog.ScenarioContext, duringQuiet func()) {
		sc.Step(`^it passes$`, func() {})
		sc.Step(`^row (\d+) fails$`, func(row int) error { return errors.New(outlineRowError(row)) })
		sc.Step(`^escalation row (\d+) is observed$`, func(row int) error {
			if row == 2 {
				return errors.New("row 2 failed")
			}
			return nil
		})
		sc.Step(`^the quiet interval is observed$`, func() error {
			if duringQuiet != nil {
				duringQuiet()
			}
			return errors.New("quiet interval failed after its rows")
		})
		sc.Step(`^it fails with quoted output$`, func() error { return errors.New(strings.Join(diagnosticQuoted, "\n")) })
		sc.Step(`^it fails quoting a failure line first$`, func() error { return errors.New(strings.Join(diagnosticFirstLineQuote, "\n")) })
		sc.Step(`^it fails with invalid bytes$`, func() error { return errors.New(strings.Join(diagnosticInvalidBytes, "\n")) })
		sc.Step(`^it fails with a quoted panic$`, func() error { return errors.New("failed first\npanic: quoted") })
		sc.Step(`^it sleeps$`, func() { time.Sleep(time.Minute) })
		sc.After(func(ctx context.Context, scenario *godog.Scenario, _ error) (context.Context, error) {
			if scenario.Name == "cleanup failure" {
				return ctx, errors.New("remove anchor: 409 conflict")
			}
			return ctx, nil
		})
	})
	if code != 0 {
		t.Fatal("a scenario failed")
	}
}

// A failure is what one annotation and one printed block say, unescaped.
type diagnosticFailure struct{ title, text string }

var (
	reportDurations = regexp.MustCompile(`\(\d+s\)`)
	dataEscaper     = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
	propertyEscaper = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C")
)

// reportOf is what the report prints for up to ten failures: the runner's
// escapes for the annotations, four spaces before each line of a printed text.
func reportOf(failures ...diagnosticFailure) (annotations []string, printed string) {
	var block strings.Builder
	for _, f := range failures {
		annotations = append(annotations, "::error title="+propertyEscaper.Replace(f.title)+"::"+dataEscaper.Replace(f.text))
		block.WriteString("--- FAIL: " + f.title + "\n")
		for _, line := range strings.Split(f.text, "\n") {
			block.WriteString("    " + line + "\n")
		}
	}
	return annotations, block.String()
}

// lastNonBlankFrom is the log from its last line holding more than spaces and
// tabs on: what the step's tail must end the output with, newline or none.
func lastNonBlankFrom(log string) string {
	lines := strings.SplitAfter(log, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Trim(lines[i], " \t\r\n") != "" {
			return strings.Join(lines[i:], "")
		}
	}
	return ""
}

// runDiagnosticsStep runs the real Engine failure diagnostics step as CI does,
// from the checkout's root, with sudo and free printing markers so that no real
// sudo runs. It runs in a UTF-8 locale, as CI's runner may, which is where a
// byte that is no UTF-8 cuts a pattern short. A nil log leaves no file, as a
// failed compilation does.
func runDiagnosticsStep(t *testing.T, script string, log *string, limit time.Duration) string {
	t.Helper()
	runner, stubs := t.TempDir(), t.TempDir()
	for name, marker := range map[string]string{"sudo": "sudo marker", "free": "free marker"} {
		if err := os.WriteFile(filepath.Join(stubs, name), []byte("#!/bin/bash\nprintf '%s\\n' '"+marker+"'\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if log != nil {
		if err := os.WriteFile(filepath.Join(runner, "slice-11-features.log"), []byte(*log), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-euo", "pipefail", "-c", script)
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "LC_ALL=C.UTF-8", "RUNNER_TEMP="+runner, "PATH="+stubs+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if code := diagnosticExitCode(err); code != 0 {
		t.Fatalf("diagnostics exit = %d, want 0: %.2000s", code, out)
	}
	return string(out)
}

// runDiagnosticChild re-executes this test binary as the child that leaves a
// real log, with the step's flags and its stdout and stderr together. A nonzero
// exit is required. The child's TMPDIR is its own: one timed out cannot remove
// its temporary directory.
func runDiagnosticChild(t *testing.T, fixture, timeout string) string {
	t.Helper()
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-test.count=1", "-test.v", "-test.timeout="+timeout, "-test.paniconexit0", "-test.run", "^"+diagnosticChild+"$")
	cmd.Env = append(os.Environ(), diagnosticLogChildEnv+"="+fixture, "TMPDIR="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if code := diagnosticExitCode(err); code <= 0 {
		t.Fatalf("the %s child exit = %d, want a failure: %.2000s", fixture, code, out)
	}
	return string(out)
}

// checkDiagnostics holds one run of the step to: exactly these annotations as
// its leading run of ::error lines, then these printed blocks and both markers,
// and, given a log, the log's last non-blank line ending the output. A
// durations normalization leaves out running times, which go rounds to the second.
func checkDiagnostics(t *testing.T, out string, log *string, annotations []string, printed string, durationsLeftOut bool) {
	t.Helper()
	head := out
	if durationsLeftOut {
		head = reportDurations.ReplaceAllString(out, "(Ns)")
	}
	var got []string
	rest := head
	for strings.HasPrefix(rest, "::error ") {
		line, after, _ := strings.Cut(rest, "\n")
		got = append(got, line)
		rest = after
	}
	if !slices.Equal(got, annotations) {
		t.Errorf("annotations:\n got %q\nwant %q", got, annotations)
	}
	const markers = "sudo marker\nfree marker\n"
	if !strings.HasPrefix(rest, printed+markers) {
		t.Errorf("after the annotations, want the printed blocks and both markers:\nwant %q\n got %.3000q", printed+markers, rest)
	}
	if log == nil || *log == "" {
		if !strings.HasSuffix(out, markers) {
			t.Errorf("the tail of nothing must leave the output ending with the markers: %.500q", out)
		}
		return
	}
	if want := lastNonBlankFrom(*log); !strings.HasSuffix(out, want) {
		t.Errorf("the output must end with the log's last non-blank line %q: %.500q", want, out[max(0, len(out)-500):])
	}
}

func testEngineFailureReport(t *testing.T, script string) {
	t.Helper()
	prefix := diagnosticChild + "/"
	check := func(t *testing.T, log *string, durationsLeftOut bool, limit time.Duration, failures ...diagnosticFailure) {
		t.Helper()
		out := runDiagnosticsStep(t, script, log, limit)
		annotations, printed := reportOf(failures...)
		checkDiagnostics(t, out, log, annotations, printed, durationsLeftOut)
	}
	handWritten := func(t *testing.T, lines string, failures ...diagnosticFailure) {
		t.Helper()
		check(t, &lines, false, 5*time.Second, failures...)
	}

	t.Run("completed run", func(t *testing.T) {
		log := runDiagnosticChild(t, "completed", "4m")
		quoted := strings.Join(diagnosticQuoted, "\n")
		failures := []diagnosticFailure{
			{prefix + "escalation,_round:_one#01", "row 2 failed"},
			{prefix + "quiet_interval", "quiet interval failed after its rows"},
			{prefix + "cleanup_failure", "after scenario hook failed: remove anchor: 409 conflict"},
			{prefix + "undefined_case", "step is undefined: nothing defines this step"},
			{prefix + "first_line_quoting_case", strings.Join(diagnosticFirstLineQuote, "\n")},
			{prefix + "invalid_bytes_case", strings.Join(diagnosticInvalidBytes, "\n")},
			{prefix + "quoted_100%_case", quoted},
		}
		check(t, &log, false, 5*time.Second, failures...)
		// The quoted error is whole under its own scenario, its empty line kept.
		annotations, printed := reportOf(failures...)
		if !strings.Contains(annotations[6], "100%25_case::quoted 100%252F done%0Dtail%0Apanic: quoted%0A%0A::error title=planted::x%0A") ||
			!strings.Contains(printed, "    panic: quoted\n    \n    ::error title=planted::x\n") {
			t.Errorf("the quoted error lost its empty line or its place: %q", annotations[6])
		}
		for _, name := range []string{"TestNested", "go test"} {
			if strings.Contains(strings.Join(annotations, "\n"), "title="+name) {
				t.Errorf("a quoted line named %s", name)
			}
		}
	})

	t.Run("timeout", func(t *testing.T) {
		log := runDiagnosticChild(t, "timeout", "5s")
		check(t, &log, true, 5*time.Second,
			diagnosticFailure{"go test", "panic: test timed out after 5s\nrunning tests:\n\t" + diagnosticChild + " (Ns)\n\t" + prefix + "sleeping_case (Ns)"},
			diagnosticFailure{prefix + "failing_case", "failed first\npanic: quoted"})
		// Branches no cheap real run reaches.
		t.Run("fatal error with a failed scenario", func(t *testing.T) {
			handWritten(t, "=== RUN   TestFeatures\n=== RUN   TestFeatures/a\n--- FAIL: TestFeatures (0.00s)\n    --- FAIL: TestFeatures/a (0.00s)\nfatal error: all goroutines are asleep - deadlock!\n",
				diagnosticFailure{"go test", "fatal error: all goroutines are asleep - deadlock!"},
				diagnosticFailure{"TestFeatures/a", "no error text in the log"})
		})
		// With no --- FAIL: line in a timeout's log, only a scenario's block says
		// it failed, so a block a report cannot read is a scenario it cannot name.
		timedOut := "=== RUN   TestFeatures/b\npanic: test timed out after 4m0s\n\trunning tests:\n\t\tTestFeatures/b (4m0s)\n"
		timeoutList := diagnosticFailure{"go test", "panic: test timed out after 4m0s\nrunning tests:\n\tTestFeatures/b (4m0s)"}
		t.Run("a block whose first line quotes a failure line", func(t *testing.T) {
			handWritten(t, "=== RUN   TestFeatures\n=== RUN   TestFeatures/a\n    suite.go:640: nested go test said --- FAIL: TestX (0.00s)\n        more detail\n"+timedOut,
				timeoutList,
				diagnosticFailure{"TestFeatures/a", "nested go test said --- FAIL: TestX (0.00s)\nmore detail"})
		})
		t.Run("a block whose first line holds invalid bytes", func(t *testing.T) {
			handWritten(t, "=== RUN   TestFeatures\n=== RUN   TestFeatures/a\n    suite.go:640: returned 75: \xff\xfe stderr\n        more\n"+timedOut,
				timeoutList,
				diagnosticFailure{"TestFeatures/a", "returned 75: \xff\xfe stderr\nmore"})
		})
		t.Run("two panics", func(t *testing.T) {
			handWritten(t, "=== RUN   TestFeatures\npanic: first\n\tdetail\n\ngoroutine 1 [running]:\nfatal error: second\n",
				diagnosticFailure{"go test", "panic: first\ndetail"},
				diagnosticFailure{"go test", "fatal error: second"})
		})
		for _, tc := range []struct {
			name, own string
			failures  []diagnosticFailure
		}{
			{"run line", "=== RUN   TestFeatures/b", []diagnosticFailure{{"Engine scenarios", "the log holds no go-test failure line; its last line: === RUN   TestFeatures/b"}}},
			{"name line", "=== NAME  TestFeatures", []diagnosticFailure{{"Engine scenarios", "the log holds no go-test failure line; its last line: === NAME  TestFeatures"}}},
			{"block start", "    x.go:1: y", []diagnosticFailure{{"Engine scenarios", "the log holds no go-test failure line; its last line:     x.go:1: y"}}},
			{"run failure", "--- FAIL: TestFeatures (0.00s)", []diagnosticFailure{{"TestFeatures", "no error text in the log"}}},
			{"scenario failure", "    --- FAIL: TestFeatures/a (0.00s)", []diagnosticFailure{
				{"TestFeatures/a", "no error text in the log"},
				{"Engine scenarios", "the log holds no go-test failure line; its last line:     --- FAIL: TestFeatures/a (0.00s)"},
			}},
		} {
			t.Run("a quoted panic before a "+tc.name, func(t *testing.T) {
				handWritten(t, "=== RUN   TestFeatures\npanic: quoted\n"+tc.own+"\n", tc.failures...)
			})
		}
	})

	t.Run("ten failures", func(t *testing.T) {
		log := runDiagnosticChild(t, "ten", "4m")
		var failures []diagnosticFailure
		for row := 1; row <= 10; row++ {
			failures = append(failures, diagnosticFailure{prefix + outlineRowName(row), outlineRowError(row)})
		}
		check(t, &log, false, 5*time.Second, failures...)
	})

	t.Run("more than ten", func(t *testing.T) {
		log := runDiagnosticChild(t, "eleven", "4m")
		var failures []diagnosticFailure
		for row := 1; row <= 11; row++ {
			failures = append(failures, diagnosticFailure{prefix + outlineRowName(row), outlineRowError(row)})
		}
		annotations, printed := reportOf(failures...)
		// Nine of their own, then the other two named, one per line.
		annotations = append(annotations[:9], "::error title=2 more failed::"+prefix+"failing_row#09%0A"+prefix+"failing_row#10")
		out := runDiagnosticsStep(t, script, &log, 5*time.Second)
		checkDiagnostics(t, out, &log, annotations, printed, false)
	})

	t.Run("test failure without scenario", func(t *testing.T) {
		log := runDiagnosticChild(t, "test failure", "4m")
		check(t, &log, false, 5*time.Second, diagnosticFailure{diagnosticChild, "failed with no scenario to blame"})
		handWritten(t, "=== RUN   TestFeatures\n--- FAIL: TestFeatures (0.00s)\nFAIL\n",
			diagnosticFailure{"TestFeatures", "no error text in the log"})
		// A scenario's text is its last suite.go block; another file's block
		// beneath a scenario is a quote, and fails nothing.
		handWritten(t, "=== RUN   TestFeatures\n=== RUN   TestFeatures/a\n    suite.go:640: first\n    suite.go:640: second\n    other.go:1: quoted\n=== RUN   TestFeatures/b\n    other.go:1: quoted\n--- FAIL: TestFeatures (0.00s)\nFAIL\n",
			diagnosticFailure{"TestFeatures/a", "second"})
		// Names and errors come from the checkout: data, never evaluated.
		t.Run("names are data", func(t *testing.T) {
			canary := filepath.Join(t.TempDir(), "canary")
			plain, keyed := "TestFeatures/a$(touch "+canary+")", "TestFeatures/b[$(touch "+canary+")]"
			text := "x`touch " + canary + "` $(touch " + canary + ")"
			handWritten(t, "=== RUN   TestFeatures\n=== RUN   "+plain+"\n    suite.go:640: "+text+"\n=== RUN   "+keyed+"\n    suite.go:640: y\n--- FAIL: "+keyed+" (0.00s)\n--- FAIL: TestFeatures (0.00s)\n",
				diagnosticFailure{plain, text}, diagnosticFailure{keyed, "y"})
			if _, err := os.Stat(canary); err == nil {
				t.Error("a name or an error was evaluated")
			}
		})
	})

	t.Run("no go test failure line", func(t *testing.T) {
		// A kill leaves a last line with no newline: it is the log's last line, not nothing.
		handWritten(t, "=== RUN   TestFeatures\n=== RUN   TestFeatures/a\npanic: quoted\n--- FAIL: TestNested (0.00s)\n    suite.go:640: boom\nfeatures TestMain received signal 15 (terminated), pid 7",
			diagnosticFailure{"TestFeatures/a", "boom"},
			diagnosticFailure{"Engine scenarios", "the log holds no go-test failure line; its last line: features TestMain received signal 15 (terminated), pid 7"})
		handWritten(t, "=== RUN   TestFeatures\n--- PASS: TestFeatures (0.00s)\nPASS\n \t\n\n",
			diagnosticFailure{"Engine scenarios", "the log holds no go-test failure line; its last line: PASS"})
		handWritten(t, "",
			diagnosticFailure{"Engine scenarios", "the log holds no go-test failure line; it is blank"})
		// An error of 50,000 lines is cut to its first 4096 characters before
		// anything is built from it, so the report stays linear in the log.
		var giant strings.Builder
		giant.WriteString("=== RUN   TestFeatures\n=== RUN   TestFeatures/a\n    suite.go:640: first line\n")
		var errorText strings.Builder
		errorText.WriteString("first line")
		for line := 1; line <= 50000; line++ {
			text := fmt.Sprintf("line %05d of an error that goes on", line)
			giant.WriteString("        " + text + "\n")
			errorText.WriteString("\n" + text)
		}
		giant.WriteString("--- FAIL: TestFeatures (0.00s)\nFAIL\n")
		log := giant.String()
		check(t, &log, false, 30*time.Second, diagnosticFailure{"TestFeatures/a", errorText.String()[:4096]})
	})

	t.Run("no log", func(t *testing.T) {
		check(t, nil, false, 5*time.Second, diagnosticFailure{"Engine scenarios", "no test log: the test binary never ran"})
	})
}

// outlineRowName is how go test names the nth row of an outline whose rows
// share one name: the name, then the name with #01, #02 and on.
func outlineRowName(row int) string {
	if row == 1 {
		return "failing_row"
	}
	return fmt.Sprintf("failing_row#%02d", row-1)
}
