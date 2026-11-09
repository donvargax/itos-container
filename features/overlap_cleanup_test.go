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
			exitAfter := overlapNeverExits
			if normalExit {
				exitAfter = 0
			}
			coverdir := ""
			if mode == "aggregate" {
				t.Setenv("GOCOVERDIR", t.TempDir())
			} else if mode != "plain" {
				coverdir = t.TempDir()
			}
			s := newOverlapDrainSim(coverdir, exitAfter, 50*time.Millisecond)
			defer s.join()
			w, worker, process := s.w, s.worker, s.process
			w.overlap.native = mode == "native"
			if mode == "covered timeout" {
				w.overlap.drainBound = 50 * time.Millisecond
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
			if s.cancelledBeforeExit == normalExit {
				t.Fatalf("%s cancelled before normal exit: %t", mode, s.cancelledBeforeExit)
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

// overlapNeverExits is the exitAfter of a controller which only cancellation
// joins. Zero exits the moment its workload ends.
const overlapNeverExits time.Duration = -1

// overlapDrainSim is a covered in-memory overlap fixture with one simulated
// controller. Once its workload ends the controller runs on for exitAfter, then
// exits by itself. Cancelling only signals it: it joins joinDelay later, as a
// real controller does, so a cleanup which does not wait for the join returns
// with the process still running. Build it inside the bubble when the delays
// are on a fake clock: its channels must belong to it.
type overlapDrainSim struct {
	w                   *world
	worker              *overlapWorker
	process             *recoveryProcess
	cancelledBeforeExit bool
	cancelledAt         time.Time
	cancelled           chan struct{}
	stop                chan struct{}
	joined              chan struct{}
}

func newOverlapDrainSim(coverdir string, exitAfter, joinDelay time.Duration) *overlapDrainSim {
	worker := &overlapWorker{
		value:  map[string]any{"State": map[string]any{"Running": true}},
		exited: make(chan struct{}),
	}
	engine := &overlapEngine{workers: map[string]*overlapWorker{"worker": worker}}
	s := &overlapDrainSim{
		worker:    worker,
		process:   &recoveryProcess{done: make(chan struct{})},
		cancelled: make(chan struct{}),
		stop:      make(chan struct{}),
		joined:    make(chan struct{}),
	}
	var finished, signalled sync.Once
	finish := func() { finished.Do(func() { close(s.process.done) }) }
	s.process.cancel = func() {
		select {
		case <-s.process.done:
		default:
			s.cancelledBeforeExit = true
		}
		signalled.Do(func() {
			s.cancelledAt = time.Now()
			close(s.cancelled)
		})
	}
	go func() {
		defer close(s.joined)
		exited := worker.exited
		var natural <-chan time.Time
		for {
			select {
			case <-exited:
				exited = nil
				if exitAfter == 0 {
					finish()
					return
				}
				if exitAfter > 0 {
					natural = time.After(exitAfter)
				}
			case <-natural:
				finish()
				return
			case <-s.cancelled:
				time.Sleep(joinDelay)
				finish()
				return
			case <-s.stop:
				return
			}
		}
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
func (s *overlapDrainSim) join() {
	close(s.stop)
	<-s.joined
}

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
			s := newOverlapDrainSim(dir, 34*time.Second, time.Second)
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
			const joinDelay = time.Second
			s := newOverlapDrainSim(dir, overlapNeverExits, joinDelay)
			defer s.join()
			start := time.Now()
			err := s.w.closeOverlap()
			elapsed := time.Since(start)
			if err == nil || !strings.Contains(err.Error(), "local controllers drain after assertions") {
				t.Errorf("a controller which never exits must fail cleanup naming the drain: %v", err)
			}
			if !s.cancelledBeforeExit {
				t.Error("forced cleanup did not cancel the controller")
			} else {
				// Forced cleanup cancels the instant the drain gives up and nothing
				// else runs then, so this one is exact.
				if gaveUp := s.cancelledAt.Sub(start); gaveUp != 35*time.Second {
					t.Errorf("drain gave up after %v, want exactly 35s", gaveUp)
				}
				// The controller joins joinDelay after it is cancelled, so a cleanup
				// which returned sooner never waited for it.
				if want := s.cancelledAt.Sub(start) + joinDelay; elapsed < want {
					t.Errorf("forced cleanup returned after %v, before the controller joined at %v", elapsed, want)
				}
			}
			select {
			case <-s.process.done:
			default:
				t.Error("forced cleanup left the controller unjoined")
			}
		})
	})
}
