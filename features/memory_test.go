package features

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

func TestFeatureMemoryRetention(t *testing.T) {
	t.Run("completed worlds release payloads during the suite", testFeatureLiveHeap)
	for _, mode := range []string{"assertion", "cleanup", "assertion and cleanup"} {
		t.Run(mode, func(t *testing.T) { testFeatureFailedCleanup(t, mode) })
	}
}

func testFeatureLiveHeap(t *testing.T) {
	const cases = 64
	const payloadBytes = 4 << 20
	const sentinel = "T-35 output sentinel"
	var feature strings.Builder
	feature.WriteString("Feature: completed world lifetime\n")
	for i := range cases {
		fmt.Fprintf(&feature, "  Scenario: case %d\n    Given a fresh memory payload\n    Then its standard output says %q\n", i, sentinel)
	}
	dir := writeFeatureFixture(t, feature.String())
	selection, err := selectScenarios("", dir, featureEnvironment{})
	if err != nil {
		t.Fatal(err)
	}
	initialized, executed, cleaned, observed := 0, 0, 0, 0
	var warm, final uint64
	var paths []string
	var output bytes.Buffer
	code := runScenarios(selection, godog.Options{Paths: []string{dir}, Format: "progress", Output: &output, TestingT: t}, func(sc *godog.ScenarioContext) {
		initialized++
		w := initializeScenario(sc, "unused-root", "unused-binary")
		var scratch string
		sc.Step(`^a fresh memory payload$`, func() error {
			if w.stdout != "" || w.doctor != nil {
				return errors.New("scenario inherited another world's state")
			}
			executed++
			scratch = w.dir
			paths = append(paths, scratch)
			// Repeat allocates anew for every world; the observer never keeps it.
			w.stdout = sentinel + strings.Repeat("x", payloadBytes-len(sentinel))
			w.doctor = &doctorWorld{cleanup: []func(){func() { cleaned++ }}}
			return nil
		})
		// godog v0.16.0 runs After hooks in registration order. Sample after
		// real cleanup, while earlier matched handlers still live in its results.
		// This hook captures only paths/counters, never w or its payload.
		sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
			observed++
			if _, err := os.Stat(scratch); !os.IsNotExist(err) {
				t.Errorf("scratch directory remains after cleanup: %s (%v)", scratch, err)
			}
			if observed == 8 || observed == cases {
				runtime.GC()
				var stats runtime.MemStats
				runtime.ReadMemStats(&stats)
				if observed == 8 {
					warm = stats.HeapAlloc
				} else {
					final = stats.HeapAlloc
				}
			}
			return ctx, nil
		})
	})
	if code != 0 || selection.count != cases || initialized != cases || executed != cases || cleaned != cases || observed != cases || len(paths) != cases {
		t.Fatalf("incomplete memory suite: code=%d selected=%d initialized=%d executed=%d cleaned=%d observed=%d paths=%d\n%s",
			code, selection.count, initialized, executed, cleaned, observed, len(paths), output.String())
	}
	assertFeatureScratchRemoved(t, paths)
	if warm == 0 || final == 0 {
		t.Fatal("missing in-suite heap observation")
	}
	growth := int64(final) - int64(warm)
	t.Logf("live heap after cases 8 and 64: %d -> %d bytes (growth %d)", warm, final, growth)
	if growth > 32<<20 {
		t.Errorf("completed worlds retain %d bytes of additional live heap; limit %d", growth, 32<<20)
	}
}

func testFeatureFailedCleanup(t *testing.T, mode string) {
	wantAssertion := strings.Contains(mode, "assertion")
	wantCleanup := strings.Contains(mode, "cleanup")
	firstOutput := "original stdout diagnostic"
	if !wantAssertion {
		firstOutput = "expected output"
	}
	dir := writeFeatureFixture(t, `Feature: failed world lifetime
  Scenario: original failure
    Given a small memory fixture
    Then its standard output says "expected output"
  Scenario: independent successor
    Given a small memory fixture
    Then its standard output says "successor output"
`)
	selection, err := selectScenarios("", dir, featureEnvironment{})
	if err != nil {
		t.Fatal(err)
	}
	initialized, executed, cleaned, observed, canceled := 0, 0, 0, 0, 0
	var paths []string
	var output bytes.Buffer
	// Expected failures belong to the returned code and diagnostics, not t.
	code := runScenarios(selection, godog.Options{Paths: []string{dir}, Format: "pretty", Output: &output}, func(sc *godog.ScenarioContext) {
		initialized++
		w := initializeScenario(sc, "unused-root", "unused-binary")
		var scratch string
		sc.Step(`^a small memory fixture$`, func() error {
			if w.stdout != "" || w.stderr != "" || w.doctor != nil || w.sockets != nil {
				return errors.New("successor inherited a completed world's state")
			}
			executed++
			scratch = w.dir
			paths = append(paths, scratch)
			w.stdout = "successor output"
			var cleanupErr error
			if executed == 1 {
				w.stdout, w.stderr = firstOutput, "original stderr diagnostic"
				if wantCleanup {
					cleanupErr = errors.New("T-35 owned cleanup diagnostic")
				}
			}
			// Exercise the real closeSockets join/error path with an in-memory
			// pipe. No executable, native engine or network endpoint is involved.
			reader, writer := io.Pipe()
			p := &socketProcess{input: writer, done: make(chan struct{})}
			go func() {
				_, _ = io.Copy(io.Discard, reader)
				_ = reader.Close()
				p.err = cleanupErr
				close(p.done)
			}()
			w.sockets = &socketWorld{listener: p, listenerCancel: func() { canceled++ }}
			w.doctor = &doctorWorld{cleanup: []func(){func() {
				select {
				case <-p.done:
				default:
					t.Error("cleanup did not join the owned resource")
				}
				if w.stdout == "" {
					t.Error("world state released before owned cleanup finished")
				}
				cleaned++
			}}}
			return nil
		})
		sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
			observed++
			if _, err := os.Stat(scratch); !os.IsNotExist(err) {
				t.Errorf("failed scenario left scratch directory %s: %v", scratch, err)
			}
			if w.stdout != "" || w.stderr != "" || w.doctor != nil || w.sockets != nil {
				t.Error("completed scenario retains output or fixture state")
			}
			return ctx, nil
		})
	})
	if code == 0 || initialized != 2 || executed != 2 || cleaned != 2 || observed != 2 || canceled != 2 {
		t.Errorf("failure or successor lost: code=%d initialized=%d executed=%d cleaned=%d observed=%d canceled=%d\n%s",
			code, initialized, executed, cleaned, observed, canceled, output.String())
	}
	assertFeatureScratchRemoved(t, paths)
	for _, text := range []string{"original failure", "independent successor", "1 passed", "memory.feature:", "steps_test.go:"} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("lost scenario/step diagnostic %q:\n%s", text, output.String())
		}
	}
	if wantAssertion {
		for _, text := range []string{`standard output does not say "expected output"`, firstOutput, "original stderr diagnostic"} {
			if !strings.Contains(output.String(), text) {
				t.Errorf("lost original assertion %q:\n%s", text, output.String())
			}
		}
	}
	if wantCleanup && !strings.Contains(output.String(), "T-35 owned cleanup diagnostic") {
		t.Errorf("lost cleanup failure:\n%s", output.String())
	}
}

func assertFeatureScratchRemoved(t *testing.T, paths []string) {
	t.Helper()
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		if path == "" || seen[path] {
			t.Errorf("scenarios did not have independent scratch directories: %q", path)
		}
		seen[path] = true
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("scratch directory remains after suite: %s (%v)", path, err)
		}
	}
}
