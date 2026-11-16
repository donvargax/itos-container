package features

import (
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"
)

// Only fixture files and the in-memory disconnect barrier are used here. The
// worker probe and real-engine scenario remain CI-only.
func lifecycleHeartbeatWorld(t *testing.T) *world {
	t.Helper()
	w := lifecycleReadinessWorld(t)
	w.lifecycle.mode = "disconnect"
	w.lifecycle.fault = newLifecycleFaults("disconnect", "", 0)
	w.offline.api.requests = []offlineRequest{{Path: "/containers/worker/wait", Status: 200}}
	return w
}

func publishLifecycleHeartbeat(t *testing.T, w *world, value string) {
	t.Helper()
	path := filepath.Join(w.doctor.checkout, "slice-12-heartbeat")
	if err := os.WriteFile(path+".next", []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".next", path); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleDisconnectWaitsForFirstHeartbeat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := lifecycleHeartbeatWorld(t)
		started := time.Now()
		published := make(chan struct{})
		go func() {
			defer close(published)
			time.Sleep(50 * time.Millisecond) // the probe publishes after readiness
			w.lifecycle.fault.mu.Lock()
			lost := w.lifecycle.fault.lost
			w.lifecycle.fault.mu.Unlock()
			if lost {
				t.Error("transport disconnected before a positive heartbeat baseline")
			}
			publishLifecycleHeartbeat(t, w, "1")
		}()
		w.lifecycleDisconnectReady()
		<-published
		l := w.lifecycle
		if l.err != nil || l.heartbeat != 1 || l.signalled.Sub(started) < 50*time.Millisecond || !l.fault.lost {
			t.Fatalf("missing positive pre-disconnection baseline: heartbeat=%d, origin=%s, lost=%v, err=%v",
				l.heartbeat, l.signalled.Sub(started), l.fault.lost, l.err)
		}
	})
}

func TestLifecycleDisconnectRefusesMissingHeartbeat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := lifecycleHeartbeatWorld(t)
		started := time.Now()
		w.lifecycleDisconnectReady()
		l := w.lifecycle
		if l.err == nil || l.fault.lost || !l.signalled.IsZero() || time.Since(started) != 5*time.Second {
			t.Fatalf("missing heartbeat authorized disconnection: lost=%v, elapsed=%s, err=%v",
				l.fault.lost, time.Since(started), l.err)
		}
	})
}

func TestLifecycleProgressWaitsAfterControllerExit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := lifecycleHeartbeatWorld(t)
		l := w.lifecycle
		l.heartbeat, l.returned = 1, time.Now()
		l.fault.disconnect()
		close(l.run.done)                    // a fast controller return must not end worker observation
		publishLifecycleHeartbeat(t, w, "2") // already present before observation
		started := time.Now()
		published := make(chan struct{})
		go func() {
			defer close(published)
			time.Sleep(50 * time.Millisecond)
			publishLifecycleHeartbeat(t, w, "3")
		}()
		w.lifecycleDisconnectProgress()
		elapsed := time.Since(started)
		<-published
		if l.err != nil || l.later != 3 || elapsed < 50*time.Millisecond || !l.fault.lost {
			t.Fatalf("no fresh progress while disconnected: later=%d, elapsed=%s, err=%v", l.later, elapsed, l.err)
		}
	})
}

func TestLifecycleProgressRefusesStalledWorker(t *testing.T) {
	for _, value := range []string{"", "invalid", "0", "1", "2"} {
		t.Run("heartbeat="+value, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				w := lifecycleHeartbeatWorld(t)
				l := w.lifecycle
				l.heartbeat, l.returned = 1, time.Now()
				l.fault.disconnect()
				close(l.run.done)
				if value != "" {
					publishLifecycleHeartbeat(t, w, value)
				}
				started := time.Now()
				w.lifecycleDisconnectProgress()
				if l.err == nil || time.Since(started) != 5*time.Second || !l.fault.lost {
					t.Fatalf("stalled worker accepted or wait unbounded: later=%d, elapsed=%s, err=%v", l.later, time.Since(started), l.err)
				}
			})
		})
	}
}
