package features

import (
	"strings"
	"sync"
	"testing"
)

// Plain controllers have no counters to export. A controller which cannot
// drain must still be cancelled and joined by ordinary fixture cleanup.
func TestOverlapCleanup(t *testing.T) {
	t.Setenv("GOCOVERDIR", "")
	t.Setenv("ITOS_CC_TEST_COVERDIR", "")
	for _, mode := range []string{"plain", "aggregate", "covered", "covered timeout", "native"} {
		t.Run(mode, func(t *testing.T) {
			normalExit := mode == "covered" || mode == "aggregate"
			worker := &overlapWorker{
				value:  map[string]any{"State": map[string]any{"Running": true}},
				exited: make(chan struct{}),
			}
			engine := &overlapEngine{workers: map[string]*overlapWorker{"worker": worker}}
			process := &recoveryProcess{done: make(chan struct{})}
			var joined sync.Once
			finish := func() { joined.Do(func() { close(process.done) }) }
			cancelledBeforeExit := false
			process.cancel = func() {
				select {
				case <-process.done:
				default:
					cancelledBeforeExit = true
				}
				finish()
			}
			child := &world{recovery: &recoveryNative{processes: []*recoveryProcess{process}}}
			w := &world{overlap: &overlapFixture{
				worlds: map[string]*world{"controller": child},
				runs:   []*overlapRun{{w: child, engine: engine, worker: "worker", process: process}},
				native: mode == "native",
			}}
			if mode == "aggregate" {
				t.Setenv("GOCOVERDIR", t.TempDir())
			} else if mode != "plain" {
				w.coverdir = t.TempDir()
			}
			if normalExit {
				go func() {
					<-worker.exited
					finish()
				}()
			}
			err := w.closeOverlap()
			if mode == "covered timeout" {
				if err == nil || !strings.Contains(err.Error(), "local controllers drain after assertions") {
					t.Fatalf("coverage drain timeout must fail cleanup: %v", err)
				}
			} else if err != nil {
				t.Fatalf("%s cleanup: %v", mode, err)
			}
			select {
			case <-process.done:
			default:
				t.Fatal("cleanup left its controller unjoined")
			}
			if cancelledBeforeExit == normalExit {
				t.Fatalf("%s cancelled before normal exit: %t", mode, cancelledBeforeExit)
			}
			select {
			case <-worker.exited:
				if mode == "plain" || mode == "native" {
					t.Fatal("cleanup requested an unnecessary coverage drain")
				}
			default:
				if mode != "plain" && mode != "native" {
					t.Fatal("coverage cleanup did not complete its simulated workload")
				}
			}
		})
	}
}
