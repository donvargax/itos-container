package features

import (
	"bytes"
	"context"
	"encoding/json"
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

const engineFailureDiagnostics = `set +e
timeout 40 tools/bin/engine-failure-report "$RUNNER_TEMP/slice-11-features.log"
sudo dmesg | tail -50
free -m
if [[ -f "$RUNNER_TEMP/slice-11-features.log" ]]; then
  # Every timing record, slowest first, so a timeout's rows show where the time went.
  # grep -a reads a NUL or invalid UTF-8 in the log as text; the sequence number keeps
  # equal durations in log order; a record with no duration counts as zero.
  printf 'Engine scenario timing records, slowest first: %s\n' "$(grep -ac '^engine scenario timing: ' "$RUNNER_TEMP/slice-11-features.log")"
  n=0
  grep -a '^engine scenario timing: ' "$RUNNER_TEMP/slice-11-features.log" | while IFS= read -r record; do
    record=${record#engine scenario timing: }
    ns=0
    if [[ $record =~ \"duration_ns\":([0-9]+) ]]; then ns=${BASH_REMATCH[1]}; fi
    ns=${ns#"${ns%%[!0]*}"}
    ns=${ns:-0}
    padded=000000000$ns
    whole=${padded:0:${#padded}-9}
    whole=${whole#"${whole%%[!0]*}"}
    printf '%s\t%s\t%4s.%s s %s\n' "$ns" "$((++n))" "${whole:-0}" "${padded:${#padded}-9:3}" "$record"
  done | LC_ALL=C sort -t $'\t' -k1,1nr -k2,2n | cut -f3-
  tail -100 "$RUNNER_TEMP/slice-11-features.log"
fi
`

// What the cut row of LIFECYCLE-09 records (the "delayed acknowledgement cut a
// call limit after the daemon check" case of TestLifecycleSignalWindow): its
// signals as the record writes them, in nanoseconds after cancellation.
const lifecycleCutSignals = `[{"signal":"SIGTERM","at_ns":650000000,"status":204,"cancelled":true,"previous_ns":600000000,"next_ns":1620000000},` +
	`{"signal":"SIGKILL","at_ns":1620000000,"replied_ns":1620000000,"status":204,"cancelled":false,"previous_ns":1620000000}]`

// A log record the step reads: what engineTiming wrote, the text after its prefix,
// which is what the section prints, and the nanoseconds it sorts and shows.
type failureLogRecord struct {
	written, text string
	ns            int64
}

func TestEngineFailureTimings(t *testing.T) {
	testEngineFailureTimings(t, runtime.GOOS)
}

func testEngineFailureTimings(t *testing.T, hostOS string) {
	t.Helper()
	if hostOS != "linux" {
		t.Skip("the engine failure diagnostics run on Linux")
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
	for _, step := range workflow.Jobs["engine-shard"].Steps {
		if step.Name == "Engine failure diagnostics" {
			script = step.Run
		}
	}
	if script == "" {
		t.Fatal("missing engine failure diagnostics step")
	}

	// Records in the streamed form, taken from engineTiming itself so that a
	// change to that form fails here.
	var buffer bytes.Buffer
	timing := &engineTiming{output: &buffer}
	record := func(feature, id string, ns int64, signals string) failureLogRecord {
		buffer.Reset()
		value := scenarioTimingRecord{Feature: feature, ID: id, Line: 7, DurationNS: ns, Outcome: "passed"}
		if signals != "" {
			var offsets []signalOffset
			if err := json.Unmarshal([]byte(signals), &offsets); err != nil {
				t.Fatal(err)
			}
			value.Outcome, value.Signals = "failed", &offsets
		}
		timing.write("scenario", value)
		written := buffer.String()
		text, ok := strings.CutPrefix(strings.TrimPrefix(written, "\n"), "engine scenario timing: ")
		if !ok || strings.Count(written, "\n") != 2 || !strings.HasSuffix(text, "\n") {
			t.Fatalf("engineTiming no longer writes one prefixed line: %q", written)
		}
		return failureLogRecord{written: written, text: strings.TrimSuffix(text, "\n"), ns: ns}
	}
	second := int64(time.Second)
	run01 := record("run.feature", "@ID-RUN-01", 1999999999, "")
	overlap03 := record("overlap.feature", "@ID-OVERLAP-03", 35*second+200_000_000, "")
	lifecycle09 := record("lifecycle.feature", "@ID-LIFECYCLE-09", 12*second+300_000_000, lifecycleCutSignals)
	lifecycle13 := record("lifecycle.feature", "@ID-LIFECYCLE-13", 128*second+410_000_000, "")
	anchors09 := record("anchors.feature", "@ID-ANCHORS-09", 35*second+200_000_000, "")

	// A section line: the whole seconds right-aligned in four columns, the
	// milliseconds truncated, then the record's text.
	line := func(r failureLogRecord) string {
		return fmt.Sprintf("%4d.%03d s %s\n", r.ns/second, r.ns%second/int64(time.Millisecond), r.text)
	}
	log := func(parts ...string) string { return strings.Join(parts, "") }
	// Each record between godog's own lines, as the progress formatter leaves them.
	between := func(r failureLogRecord) string { return "....\n" + r.written + "...\n" }
	var goroutines strings.Builder
	for i := range 120 {
		fmt.Fprintf(&goroutines, "goroutine %d [chan receive, %d minutes]:\n", i+8, i%4)
	}
	// A kill ends the log inside a record: its text stops after the ID's prefix.
	cut := `{"feature":"lifecycle.feature","id":"@ID-LIFE`
	killed := lifecycle09.written[:strings.Index(lifecycle09.written, cut)+len(cut)]
	if killed != "\nengine scenario timing: "+cut {
		t.Fatalf("the cut record is not the start of a streamed record: %q", killed)
	}
	for _, tc := range []struct {
		name string
		log  *string // nil leaves no log
		want string  // the section
	}{
		{name: "timeout", log: ptr(log(
			"=== RUN   TestFeatures\n",
			"Feature: lifecycle\n",
			"output \x00 and \xff bytes of a workload\n",
			between(run01), between(overlap03), between(lifecycle09), between(lifecycle13), between(anchors09),
			"panic: test timed out after 4m0s\n\trunning tests:\n\t\tTestFeatures/lifecycle (4m0s)\n\n",
			goroutines.String(),
		)), want: "Engine scenario timing records, slowest first: 5\n" +
			line(lifecycle13) + line(overlap03) + line(anchors09) + line(lifecycle09) + line(run01)},
		{name: "record cut by a kill", log: ptr(log(
			"=== RUN   TestFeatures\n",
			between(run01), between(overlap03),
			killed,
		)), want: "Engine scenario timing records, slowest first: 3\n" +
			line(overlap03) + line(run01) + line(failureLogRecord{text: cut})},
		{name: "no records", log: ptr(log(
			"=== RUN TestFeatures\n", "--- FAIL: TestFeatures (0.00s)\n", "FAIL\n",
		)), want: "Engine scenario timing records, slowest first: 0\n"},
		{name: "no log"},
	} {
		t.Run(strings.ReplaceAll(tc.name, " ", "_"), func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "bin")
			if err := os.Mkdir(bin, 0700); err != nil {
				t.Fatal(err)
			}
			// No real sudo runs: the stubs check their arguments and mark the output.
			stubs := map[string]string{
				"sudo": "#!/bin/bash\n[[ $# = 1 && $1 = dmesg ]] || exit 64\nprintf 'kernel marker\\n'\n",
				"free": "#!/bin/bash\n[[ $# = 1 && $1 = -m ]] || exit 64\nprintf 'memory marker\\n'\n",
			}
			for name, contents := range stubs {
				if err := os.WriteFile(filepath.Join(bin, name), []byte(contents), 0700); err != nil {
					t.Fatal(err)
				}
			}
			tail := ""
			if tc.log != nil {
				if err := os.WriteFile(filepath.Join(dir, "slice-11-features.log"), []byte(*tc.log), 0600); err != nil {
					t.Fatal(err)
				}
				tail = string(lastLines([]byte(*tc.log), 100))
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "bash", "-euo", "pipefail", "-c", script)
			cmd.Dir = ".." // the checkout's root, as in CI
			cmd.Env = append(os.Environ(), "RUNNER_TEMP="+dir, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("the step failed: %v\nstdout %q\nstderr %q", err, stdout.String(), stderr.String())
			}
			out := stdout.String()
			kernel, memory := strings.Index(out, "kernel marker\n"), strings.Index(out, "memory marker\n")
			if kernel < 0 || memory < kernel {
				t.Fatalf("the kernel marker must precede the memory marker: %q", out)
			}
			got, want := out[memory+len("memory marker\n"):], tc.want+tail
			if got != want {
				t.Errorf("after the memory marker the step printed\n%q\nwant the section and the last 100 lines\n%q", got, want)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }

// The last n lines of data as tail prints them: a last line the log ends
// without a newline is printed without one.
func lastLines(data []byte, n int) []byte {
	lines := bytes.SplitAfter(data, []byte("\n"))
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return bytes.Join(lines, nil)
}
