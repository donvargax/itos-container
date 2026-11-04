package features

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

func (w *world) policyMemorySteps(sc *godog.ScenarioContext) {
	sc.Step(`^the policy file contains (\d+) nested sequences below an unknown field$`, func(depth int) error {
		data := "version: 1\nmanifest: a.json\nextra: " + strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth) + "\n"
		return os.WriteFile(filepath.Join(w.dir, "itos-container.yaml"), []byte(data), 0o600)
	})
	sc.Step(`^its GC trace reports less than (\d+) MiB of heap$`, func(limit int) error {
		peak, err := tracedHeapMiB(w.stderr)
		if err != nil {
			return err
		}
		if peak >= limit {
			return fmt.Errorf("policy inspection GC trace reached %d MiB of heap; want less than %d MiB", peak, limit)
		}
		return nil
	})
}

// Go's runtime/extern.go documents start->end->live heap sizes in gctrace.
// Refuse absent or changed trace formats rather than silently losing the check.
var gcHeapSizes = regexp.MustCompile(`, ([0-9]+)->([0-9]+)->([0-9]+) MB,`)

func tracedHeapMiB(trace string) (int, error) {
	peak, found := 0, false
	for line := range strings.SplitSeq(trace, "\n") {
		if !strings.HasPrefix(line, "gc ") {
			continue
		}
		sizes := gcHeapSizes.FindStringSubmatch(line)
		if sizes == nil {
			return 0, fmt.Errorf("unrecognized GC heap trace format")
		}
		found = true
		for _, size := range sizes[1:] {
			value, err := strconv.Atoi(size)
			if err != nil {
				return 0, fmt.Errorf("invalid GC heap size: %w", err)
			}
			peak = max(peak, value)
		}
	}
	if !found {
		return 0, fmt.Errorf("no GC heap trace was captured")
	}
	return peak, nil
}

func TestTracedHeapMiB(t *testing.T) {
	for _, tc := range []struct {
		name, trace string
		peak        int
		invalid     bool
	}{
		{"start", "gc 1 @0s: timings, 80->20->10 MB, 100 MB goal", 80, false},
		{"end", "gc 1 @0s: timings, 20->90->10 MB, 100 MB goal", 90, false},
		{"live", "gc 1 @0s: timings, 20->30->95 MB, 100 MB goal", 95, false},
		{"multiple", "gc 1 @0s: timings, 80->20->10 MB, 100 MB goal\ngc 2 @1s: timings, 30->40->20 MB, 50 MB goal\n", 80, false},
		{"zero", "gc 1 @0s: timings, 0->0->0 MB, 1 MB goal", 0, false},
		{"absent", "unrelated output\n", 0, true},
		{"changed format", "gc 1: no heap triplet", 0, true},
		{"overflow", "gc 1 @0s: timings, 999999999999999999999->0->0 MB, 1 MB goal", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tracedHeapMiB(tc.trace)
			if (err != nil) != tc.invalid || got != tc.peak {
				t.Fatalf("tracedHeapMiB() = %d, %v; want %d, invalid %v", got, err, tc.peak, tc.invalid)
			}
		})
	}
}
