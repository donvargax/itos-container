package features

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cucumber/godog"
)

// Scheduling changes must preserve every selected row and its private world,
// including failures. Engine and red evidence still require serial execution.
func TestFeatureConcurrency(t *testing.T) {
	for _, mode := range []string{"local", "engine", "red"} {
		t.Run(mode, func(t *testing.T) {
			text := "Feature: scheduling\n"
			for n := range 8 {
				text += fmt.Sprintf("  @ID-MEMORY-%02d\n  Scenario: row %d\n    Given remember %d\n    Then check %d\n", n+1, n, n, n)
			}
			dir := writeFeatureFixture(t, text)
			selection, err := selectScenarios("", dir, featureEnvironment{})
			if err != nil {
				t.Fatal(err)
			}
			// These flags exercise scheduling only, with memory steps and no engine.
			selection.engine, selection.red = mode == "engine", mode == "red"
			for _, fail := range []bool{false, true} {
				synctest.Test(t, func(t *testing.T) {
					var mu sync.Mutex
					active, peak, completed := 0, 0, 0
					seen := map[int]int{}
					code := runScenarios(selection, godog.Options{
						Paths: []string{dir}, Format: "progress", Output: io.Discard, Concurrency: 4,
					}, func(sc *godog.ScenarioContext) {
						value := -1
						sc.Step(`^remember (\d+)$`, func(n int) {
							value = n
							mu.Lock()
							active++
							peak = max(peak, active)
							seen[n]++
							mu.Unlock()
							time.Sleep(time.Second)
						})
						sc.Step(`^check (\d+)$`, func(n int) error {
							if value != n || fail && n == 3 {
								return fmt.Errorf("row %d has value %d (failure control %v)", n, value, fail)
							}
							return nil
						})
						sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
							mu.Lock()
							active--
							completed++
							mu.Unlock()
							return ctx, nil
						})
					})
					want := 1
					if mode == "local" {
						want = 4
					}
					if peak != want || active != 0 || completed != 8 || len(seen) != 8 {
						t.Errorf("peak=%d want=%d active=%d completed=%d seen=%v", peak, want, active, completed, seen)
					}
					for n, count := range seen {
						if count != 1 {
							t.Errorf("row %d executed %d times", n, count)
						}
					}
					if (code != 0) != fail {
						t.Errorf("exit=%d failure control=%v", code, fail)
					}
				})
			}
		})
	}
}
