package features

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestLifecyclePreludePreservesBytesAndAllowsFullPipe(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native pipe-capacity observation requires Linux")
	}
	for _, stdout := range []bool{true, false} {
		t.Run(map[bool]string{true: "stdout", false: "stderr"}[stdout], func(t *testing.T) {
			read, write, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = read.Close(); _ = write.Close() }()
			prelude := []byte("session fixture diagnostic\n")
			if stdout {
				prelude = []byte("slice-12:ready\n")
			}
			if _, err := write.Write(prelude); err != nil {
				t.Fatal(err)
			}
			got, err := lifecyclePipePrelude(read, stdout)
			if err != nil || !bytes.Equal(got, prelude) {
				t.Fatalf("prelude was lost or changed: %q, %v", got, err)
			}
			done := make(chan struct{})
			go func() { defer close(done); _, _ = write.Write(make([]byte, 4<<20)) }()
			w := &world{lifecycle: &lifecycleNative{run: &recoveryProcess{done: done}}}
			full := w.lifecycleAwait(time.Second, func() bool { return lifecycleFullPipe(read) })
			_ = read.Close()
			<-done
			if !full {
				t.Fatal("short prelude prevented observation of actual backpressure")
			}
		})
	}
}

func TestLifecycleBarrierFailsWithinItsOwnBound(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := &world{lifecycle: &lifecycleNative{run: &recoveryProcess{done: make(chan struct{})}}}
		start := time.Now()
		if w.lifecycleAwait(5*time.Second, func() bool { return false }) || time.Since(start) != 5*time.Second {
			t.Fatal("missing stream barrier did not fail at its own deadline")
		}
		close(w.lifecycle.run.done)
		start = time.Now()
		if w.lifecycleAwait(5*time.Second, func() bool { return false }) || !time.Now().Equal(start) {
			t.Fatal("early controller exit waited for the barrier timeout")
		}
	})
}

func TestLifecycleCollectionGuardUsesRecordedOrigin(t *testing.T) {
	for _, phase := range []string{"startup", "cancellation", "exit"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p := &recoveryProcess{done: make(chan struct{}), started: time.Now()}
				l := &lifecycleNative{run: p}
				origin, limit := p.started, 40*time.Second
				if phase != "startup" {
					time.Sleep(7 * time.Second)
					origin, limit = time.Now(), 32*time.Second
					if phase == "cancellation" {
						l.signalled = origin
					} else {
						l.exitObserved = origin
					}
				}
				p.cancel = func() { close(p.done) }
				time.Sleep(3 * time.Second) // collection must not reset the origin
				l.wait()
				if time.Since(origin) != limit || l.err == nil || !strings.Contains(l.err.Error(), "guard") {
					t.Fatalf("collection did not bound and classify harness cancellation: %s, %v", time.Since(origin), l.err)
				}
			})
		})
	}
}

func TestLifecycleProcessGuardRecordsStartAndReturn(t *testing.T) {
	// This child is a harness-only sleeper, never a worker probe or the product.
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	p := &recoveryProcess{done: make(chan struct{}), cancel: cancel}
	p.cmd = exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLifecycleGuardSleeper$")
	p.cmd.Env = setEnv(os.Environ(), "SLICE_12_GUARD_SLEEPER", "1")
	p.cmd.Stdout, p.cmd.Stderr = &p.out, &p.errout
	lifecycleStartProcess(ctx, p)
	select {
	case <-p.done:
	case <-time.After(2 * time.Second):
		t.Fatal("guarded process stranded the harness")
	}
	if p.started.IsZero() || p.finished.Before(p.started) || p.finished.Sub(p.started) > time.Second ||
		p.err == nil || !strings.Contains(p.err.Error(), "harness guard after") {
		t.Fatalf("guard lost its recorded elapsed time or error: %v", p.err)
	}
}

func TestLifecycleGuardSleeper(t *testing.T) {
	if os.Getenv("SLICE_12_GUARD_SLEEPER") != "1" {
		return
	}
	time.Sleep(10 * time.Second)
}
