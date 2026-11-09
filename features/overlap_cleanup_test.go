package features

import (
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
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

// overlapDrainSim is TestOverlapCleanup's covered in-memory fixture with its
// simulated controller on the enclosing bubble's fake clock. The controller
// runs on for exitAfter once its workload ends; zero never exits by itself, so
// only cancellation joins it. Build it inside the bubble: its channels must
// belong to it.
type overlapDrainSim struct {
	w                   *world
	process             *recoveryProcess
	cancelledBeforeExit bool
	joined              chan struct{}
}

func newOverlapDrainSim(coverdir string, exitAfter time.Duration) *overlapDrainSim {
	worker := &overlapWorker{
		value:  map[string]any{"State": map[string]any{"Running": true}},
		exited: make(chan struct{}),
	}
	engine := &overlapEngine{workers: map[string]*overlapWorker{"worker": worker}}
	s := &overlapDrainSim{
		process: &recoveryProcess{done: make(chan struct{})},
		joined:  make(chan struct{}),
	}
	var finished sync.Once
	finish := func() { finished.Do(func() { close(s.process.done) }) }
	s.process.cancel = func() {
		select {
		case <-s.process.done:
		default:
			s.cancelledBeforeExit = true
		}
		finish()
	}
	go func() {
		defer close(s.joined)
		select {
		case <-worker.exited:
		case <-s.process.done:
			return
		}
		if exitAfter == 0 {
			<-s.process.done
			return
		}
		time.Sleep(exitAfter)
		finish()
	}()
	child := &world{recovery: &recoveryNative{processes: []*recoveryProcess{s.process}}}
	s.w = &world{coverdir: coverdir, overlap: &overlapFixture{
		worlds: map[string]*world{"controller": child},
		runs:   []*overlapRun{{w: child, engine: engine, worker: "worker", process: s.process}},
	}}
	return s
}

// A bubble whose test fails with the controller's goroutine still asleep
// panics and hides the failure, so every path joins it.
func (s *overlapDrainSim) join() { <-s.joined }

// TestOverlapCleanup shortens the bound for its own timeout case, so it says
// nothing about the default every scenario uses. This holds the default on a
// fake clock: a controller which drains within the product's teardown total
// must not be cancelled, and one which never drains fails cleanup at the bound.
func TestOverlapDrainBound(t *testing.T) {
	t.Setenv("GOCOVERDIR", "")
	t.Setenv("ITOS_CC_TEST_COVERDIR", "")
	t.Run("slow drain", func(t *testing.T) {
		dir := t.TempDir()
		synctest.Test(t, func(t *testing.T) {
			s := newOverlapDrainSim(dir, 34*time.Second)
			defer s.join()
			start := time.Now()
			err := s.w.closeOverlap()
			elapsed := time.Since(start)
			if err != nil {
				t.Errorf("a controller exiting 34 s after its workload must drain within the bound: %v", err)
			}
			// Never exactly 34 s: the exit and overlapAwait's 5 ms tick share that
			// fake instant, so the poll can run first and return at 34.005 s.
			if elapsed < 34*time.Second || elapsed >= 35*time.Second {
				t.Errorf("drain returned after %v, want at least 34s and less than 35s", elapsed)
			}
			if s.cancelledBeforeExit {
				t.Error("cleanup cancelled the controller before it exited")
			}
		})
	})
	t.Run("hung controller", func(t *testing.T) {
		dir := t.TempDir()
		synctest.Test(t, func(t *testing.T) {
			s := newOverlapDrainSim(dir, 0)
			defer s.join()
			start := time.Now()
			err := s.w.closeOverlap()
			elapsed := time.Since(start)
			if err == nil || !strings.Contains(err.Error(), "local controllers drain after assertions") {
				t.Errorf("a controller which never exits must fail cleanup naming the drain: %v", err)
			}
			// Nothing else runs at the bound's instant, so this one is exact.
			if elapsed != 35*time.Second {
				t.Errorf("drain gave up after %v, want exactly 35s", elapsed)
			}
			if !s.cancelledBeforeExit {
				t.Error("forced cleanup did not cancel the controller")
			}
			select {
			case <-s.process.done:
			default:
				t.Error("forced cleanup left the controller unjoined")
			}
		})
	})
}
