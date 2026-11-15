package features

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"
)

func writeLifecycleHeartbeat(t *testing.T, w *world, value string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(w.doctor.checkout, "slice-12-heartbeat"), []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}

// Exercise only the observer, using private files and controlled time. No
// native probe, product, engine or socket runs on the host.
func TestLifecycleDisconnectWaitsForInitialProgress(t *testing.T) {
	for _, publishes := range []bool{true, false} {
		t.Run(map[bool]string{true: "delayed heartbeat", false: "no heartbeat"}[publishes], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				w := lifecycleReadinessWorld(t)
				w.lifecycle.fault = newLifecycleFaults("disconnect", "", 0)
				w.offline.api.requests = []offlineRequest{{Path: "/containers/owned/wait", Status: http.StatusOK}}
				if publishes {
					written := make(chan struct{})
					defer func() { <-written }()
					go func() {
						defer close(written)
						time.Sleep(50 * time.Millisecond)
						writeLifecycleHeartbeat(t, w, "1")
					}()
				}
				start := time.Now()
				w.lifecycleDisconnectReady()
				if publishes {
					if w.lifecycle.err != nil || w.lifecycle.heartbeat != 1 || w.lifecycle.signalled.IsZero() || time.Since(start) < 50*time.Millisecond {
						t.Error("transport disconnected before observing the probe's first heartbeat")
					}
				} else if w.lifecycle.err == nil || !w.lifecycle.signalled.IsZero() || time.Since(start) != 5*time.Second {
					t.Error("wait acknowledgement alone authorized transport loss without a progress baseline")
				}
			})
		})
	}
}

func TestLifecyclePostDisconnectProgress(t *testing.T) {
	for _, tc := range []struct {
		name, first, next string
		want              int
	}{
		{"already advanced", "8", "", 8},
		{"next tick after controller exit", "7", "8", 8},
		{"temporarily missing", "", "8", 8},
		{"temporarily malformed", "partial", "8", 8},
		{"regressed counter", "6", "8", 8},
		{"unchanged", "7", "", 0},
		{"missing", "", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				w := lifecycleReadinessWorld(t)
				w.lifecycle.heartbeat = 7
				close(w.lifecycle.run.done) // observer must outlive the controller
				if tc.first != "" {
					writeLifecycleHeartbeat(t, w, tc.first)
				}
				if tc.next != "" {
					written := make(chan struct{})
					defer func() { <-written }()
					go func() {
						defer close(written)
						time.Sleep(50 * time.Millisecond)
						writeLifecycleHeartbeat(t, w, tc.next)
					}()
				}
				start := time.Now()
				w.lifecycleLaterHeartbeat()
				elapsed := time.Since(start)
				if w.lifecycle.later != tc.want {
					t.Error("post-disconnection observation did not retain independently advanced progress")
				}
				if tc.want == 0 && elapsed != 3*time.Second {
					t.Error("missing worker progress did not end at the fixed observation allowance")
				}
				if tc.next != "" && (elapsed < 50*time.Millisecond || elapsed > 60*time.Millisecond) {
					t.Error("observer did not await the next heartbeat within one poll")
				}
			})
		})
	}
}
