package features

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Returning through testing.Main reads every counter in -test.gocoverdir.
// A traced probe must report only its own invocation, then retain its measured
// data alongside the other processes belonging to the scenario.
func testRecoveryProbeCoverage(t *testing.T, root string) {
	t.Helper()
	cover := t.TempDir()
	t.Setenv("ITOS_CC_TEST_COVERDIR", cover)
	bin, err := build(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := &world{root: root, bin: bin, dir: t.TempDir(), coverdir: cover}
	closed := false
	closeFixture := func() {
		if !closed {
			closed = true
			if err := w.closeOverlap(); err != nil {
				t.Error(err)
			}
		}
	}
	t.Cleanup(closeFixture)
	if err := w.overlapSetup("child", "shared daemon", false); err != nil {
		t.Fatal(err)
	}
	var runs []*overlapRun
	var reports []string
	var pids []int
	for _, name := range []string{"ancestor", "prefix"} {
		r := w.overlap.start(name, "before create", "")
		if w.overlap.err != nil {
			t.Fatal(w.overlap.err)
		}
		if !r.probe.await() {
			t.Fatal("probe did not reach its pre-create barrier")
		}
		report, destination := "", ""
		for _, arg := range r.process.cmd.Args {
			if value, ok := strings.CutPrefix(arg, "-test.gocoverdir="); ok {
				report = value
			}
		}
		for _, value := range r.process.cmd.Env {
			if value, ok := strings.CutPrefix(value, "GOCOVERDIR="); ok {
				destination = value
			}
		}
		if report == cover || report == "" || report != destination {
			t.Errorf("probe coverage report must read only its own counters: report=%q environment=%q shared=%q", report, destination, cover)
		}
		for _, previous := range reports {
			if report == previous {
				t.Error("concurrent probes share a coverage report directory")
			}
		}
		reports = append(reports, report)
		pids = append(pids, r.pid())
		if err := r.probe.resume(); err != nil {
			t.Fatal(err)
		}
		w.overlap.observe(r, false)
		if w.overlap.err != nil || !r.active() {
			t.Fatalf("probe did not start its workload: %v", w.overlap.err)
		}
		runs = append(runs, r)
	}
	closeFixture()
	for i, r := range runs {
		if r.process.err != nil || r.process.exit != 0 {
			t.Errorf("probe did not return through the test runner: exit=%d error=%v", r.process.exit, r.process.err)
		}
		files, err := filepath.Glob(filepath.Join(cover, "covcounters.*."+strconv.Itoa(pids[i])+".*"))
		if err != nil || len(files) == 0 {
			t.Errorf("joined probe %d did not retain its own counters in the scenario: %v", pids[i], err)
		}
		if reports[i] != cover {
			if _, err := os.Stat(reports[i]); !os.IsNotExist(err) {
				t.Errorf("joined probe left its private report directory: %v", err)
			}
		}
	}
	if !executedCoverage(readProductCoverage(t, cover), "/internal/engine/session/domain/") {
		t.Error("joined probes lost executed session coverage")
	}
}
