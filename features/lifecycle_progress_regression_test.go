package features

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"testing/synctest"
	"time"
)

func lifecyclePublishHeartbeat(t *testing.T, w *world, value int) {
	t.Helper()
	path := filepath.Join(w.doctor.checkout, "slice-12-heartbeat")
	if err := os.WriteFile(path+".next", []byte(strconv.Itoa(value)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".next", path); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleDisconnectWaitsForHealthyHeartbeat(t *testing.T) {
	for _, observation := range []string{"delayed heartbeat", "missing heartbeat", "missing wait", "controller exit"} {
		t.Run(observation, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				w := lifecycleReadinessWorld(t)
				l := w.lifecycle
				l.fault = newLifecycleFaults("disconnect", "", 0)
				// An acknowledged wait and a complete readiness frame may both
				// precede the probe's first heartbeat publication.
				w.offline.api.requests = []offlineRequest{{Path: "/containers/owned/wait", Status: 200}}
				if observation == "missing wait" {
					w.offline.api.requests = nil
					lifecyclePublishHeartbeat(t, w, 1)
				}
				_, _ = l.run.out.Write([]byte(lifecycleReadyFrame))
				if observation == "delayed heartbeat" {
					written := make(chan struct{})
					defer func() { <-written }()
					go func() {
						defer close(written)
						time.Sleep(100 * time.Millisecond)
						l.fault.mu.Lock()
						if l.fault.lost {
							t.Error("disconnect preceded the healthy heartbeat")
						}
						l.fault.mu.Unlock()
						lifecyclePublishHeartbeat(t, w, 1)
					}()
				}
				if observation == "controller exit" {
					close(l.run.done)
				}
				start := time.Now()
				w.lifecycleDisconnectReady()
				if observation == "delayed heartbeat" {
					if l.heartbeat != 1 || !l.fault.lost || l.err != nil || time.Since(start) < 100*time.Millisecond {
						t.Errorf("disconnect did not wait for a healthy heartbeat: baseline=%d lost=%t elapsed=%s err=%v", l.heartbeat, l.fault.lost, time.Since(start), l.err)
					}
				} else if l.fault.lost || !l.signalled.IsZero() || l.err == nil {
					t.Errorf("disconnect without healthy progress: lost=%t origin=%s err=%v", l.fault.lost, l.signalled, l.err)
				}
				if (observation == "missing heartbeat" || observation == "missing wait") && time.Since(start) != 5*time.Second {
					t.Errorf("missing observation escaped its bound: %s", time.Since(start))
				}
			})
		})
	}
}

func TestLifecycleDisconnectWaitsForIndependentProgress(t *testing.T) {
	for _, tc := range []struct {
		name, initial string
		publish       bool
		wantProgress  bool
	}{
		{"already advanced", "8", false, true},
		{"delayed progress", "7", true, true},
		{"temporarily missing", "", true, true},
		{"temporarily malformed", "partial", true, true},
		{"regressed then advanced", "6", true, true},
		{"missing", "", false, false},
		{"unchanged", "7", false, false},
		{"regressed", "6", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				w := lifecycleReadinessWorld(t)
				l := w.lifecycle
				l.heartbeat, l.returned = 7, time.Now()
				close(l.run.done) // Expected: the controller has already returned.
				if tc.initial != "" {
					if err := os.WriteFile(filepath.Join(w.doctor.checkout, "slice-12-heartbeat"), []byte(tc.initial), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if tc.publish {
					written := make(chan struct{})
					defer func() { <-written }()
					go func() {
						defer close(written)
						time.Sleep(100 * time.Millisecond)
						lifecyclePublishHeartbeat(t, w, 8)
					}()
				}
				w.lifecycleDisconnectProgress()
				if tc.wantProgress {
					if l.later != 8 || l.err != nil {
						t.Errorf("did not await independent progress after controller exit: later=%d elapsed=%s err=%v", l.later, time.Since(l.returned), l.err)
					}
					if elapsed := time.Since(l.returned); tc.publish && (elapsed < 100*time.Millisecond || elapsed > 110*time.Millisecond) {
						t.Errorf("observer did not await the next heartbeat within one poll: %s", elapsed)
					}
				} else if l.err == nil || time.Since(l.returned) != 5*time.Second {
					t.Errorf("missing progress did not fail within its bound: later=%d elapsed=%s err=%v", l.later, time.Since(l.returned), l.err)
				}
			})
		})
	}
}
