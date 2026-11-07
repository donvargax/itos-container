package features

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

func TestScenarioConcurrency(t *testing.T) {
	for _, mode := range []string{"local", "local failure", "engine", "red"} {
		t.Run(mode, func(t *testing.T) {
			dir := writeFeatureFixture(t, `Feature: independent scenarios
  Scenario Outline: each fixture executes once
    Then observe fixture "<name>"
    Examples:
      | name |
      | a    |
      | b    |
      | c    |
      | d    |
      | e    |
      | f    |
`)
			serial := mode == "engine" || mode == "red"
			selection := scenarioSelection{count: 6, engine: mode == "engine", red: mode == "red"}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			var mu sync.Mutex
			seen := map[string]int{}
			active, peak := 0, 0
			group := make(chan struct{})
			code := runScenarios(selection, godog.Options{Paths: []string{dir}, Format: "progress", Output: io.Discard, Concurrency: localScenarioWorkers}, func(sc *godog.ScenarioContext) {
				sc.Step(`^observe fixture "([^"]*)"$`, func(name string) error {
					mu.Lock()
					seen[name]++
					active++
					peak = max(peak, active)
					if len(seen) == 4 {
						close(group)
					}
					mu.Unlock()
					defer func() { mu.Lock(); active--; mu.Unlock() }()
					if serial {
						// Leave a scheduling window in which a wrongly parallel
						// runner would admit the next fixture.
						time.Sleep(10 * time.Millisecond)
					} else {
						select {
						case <-group:
						case <-ctx.Done():
							return errors.New("independent local fixtures never overlapped")
						}
					}
					if mode == "local failure" && name == "c" {
						return errors.New("fixture assertion failed")
					}
					return nil
				})
			})
			wantCode, wantPeak := 0, 4
			if mode == "local failure" {
				wantCode = 1
			}
			if serial {
				wantPeak = 1
			}
			if code != wantCode || peak != wantPeak || active != 0 || len(seen) != 6 {
				t.Fatalf("exit %d, peak %d, active %d, executions %v; want exit %d and peak %d", code, peak, active, seen, wantCode, wantPeak)
			}
			for name, count := range seen {
				if count != 1 {
					t.Errorf("fixture %s executed %d times", name, count)
				}
			}
		})
	}
}
