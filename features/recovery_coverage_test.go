package features

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The child is already joined, so Go cannot still be writing these files.
// Renaming within the scenario destination preserves measured bytes, including
// concurrent outline counters. Metadata with the same hash is identical.
func (p *recoveryProcess) exportCoverage() error {
	if p.coverageSource == "" {
		return nil
	}
	entries, err := os.ReadDir(p.coverageSource)
	if err != nil {
		return err
	}
	var metadata, counters bool
	for _, entry := range entries {
		meta := strings.HasPrefix(entry.Name(), "covmeta.")
		counter := strings.HasPrefix(entry.Name(), "covcounters.")
		if !entry.Type().IsRegular() || (!meta && !counter) {
			return fmt.Errorf("unexpected recovery coverage entry %q", entry.Name())
		}
		if err := os.Rename(filepath.Join(p.coverageSource, entry.Name()), filepath.Join(p.coverageDestination, entry.Name())); err != nil {
			return err
		}
		metadata = metadata || meta
		counters = counters || counter
	}
	// Some recovery assertions deliberately kill the controller. Preserve any
	// files it emitted, but require complete output only on normal test exit.
	if p.err == nil && p.exit == 0 && (!metadata || !counters) {
		return fmt.Errorf("recovery probe did not export both coverage metadata and counters")
	}
	if err := os.Remove(p.coverageSource); err != nil {
		return err
	}
	p.coverageSource = ""
	return nil
}

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
	if err := w.overlap.finishLocalWorkloads(); err != nil {
		t.Fatal(err)
	}
	measured := map[string][]byte{}
	for _, report := range reports {
		entries, err := os.ReadDir(report)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			data, err := os.ReadFile(filepath.Join(report, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			measured[entry.Name()] = data
		}
	}
	ambient := t.TempDir()
	t.Setenv("GOCOVERDIR", ambient)
	closeFixture()
	assertCoverageEntries(t, ambient, nil)
	for name, want := range measured {
		got, err := os.ReadFile(filepath.Join(cover, name))
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("export changed measured coverage %s: %v", name, err)
		}
	}
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

func TestRecoveryCoverageCleanup(t *testing.T) {
	for _, mode := range []string{"killed", "missing counters", "missing directory"} {
		t.Run(mode, func(t *testing.T) {
			source, destination := t.TempDir(), t.TempDir()
			p := &recoveryProcess{done: make(chan struct{}), coverageSource: source, coverageDestination: destination}
			p.cancel = func() { close(p.done) }
			if mode == "killed" {
				p.exit = 137
			}
			if mode == "missing directory" {
				if err := os.Remove(source); err != nil {
					t.Fatal(err)
				}
			}
			w := &world{recovery: &recoveryNative{processes: []*recoveryProcess{p}}}
			err := w.closeRecoveryNative()
			if (err == nil) != (mode == "killed") {
				t.Fatalf("%s cleanup error: %v", mode, err)
			}
			assertCoverageEntries(t, destination, nil)
		})
	}
}
