package features

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// These controls use only a disposable host child and /proc, never an engine.
// Delay exec deterministically: the PID/start identity is already observable,
// but its nonce-bearing exec environment is not yet available to the parent.
func isolationDelayedHostProcess(t *testing.T, nonce string) (*isolationWorld, io.WriteCloser) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", `printf 'ready\n'; IFS= read -r barrier; export SLICE_14_SENTINEL="$1"; exec /bin/sleep 240`, "slice-11-sentinel", nonce)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	ready, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = input.Close() })
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = cmd.Wait() })
	if line, err := bufio.NewReader(ready).ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatalf("host child did not reach its exec barrier: %q, %v", line, err)
	}
	c := isolationConfig{Nonce: nonce, HostPID: cmd.Process.Pid, HostStart: isolationProcessStart(cmd.Process.Pid)}
	if c.HostStart == "" {
		t.Fatal("host child has no start identity")
	}
	return &isolationWorld{config: c}, input
}

func TestOfflineIsolationHostProcessReadiness(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("host process controls require Linux procfs")
	}
	t.Run("delayed exec environment", func(t *testing.T) {
		s, input := isolationDelayedHostProcess(t, "slice-11-delayed-nonce")
		if _, err := s.hostControls(); err == nil {
			t.Fatal("host control accepted a process without its nonce")
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- isolationAwaitHostProcess(ctx, s.config) }()
		select {
		case err := <-done:
			t.Fatalf("readiness returned before the nonce-bearing exec: %v", err)
		case <-time.After(20 * time.Millisecond):
		}
		if _, err := io.WriteString(input, "continue\n"); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		control, err := s.hostControls()
		if err != nil || control["sentinel"] != s.config.HostStart {
			t.Fatalf("ready process lost its nonce/start control: %v, %v", control, err)
		}
	})
	t.Run("missing nonce is bounded", func(t *testing.T) {
		s, _ := isolationDelayedHostProcess(t, "slice-11-missing-nonce")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()
		if err := isolationAwaitHostProcess(ctx, s.config); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("unavailable nonce did not exhaust the readiness budget: %v", err)
		}
		if _, err := s.hostControls(); err == nil {
			t.Fatal("missing nonce passed the host control after timeout")
		}
	})
	t.Run("nonce must match a complete environment entry", func(t *testing.T) {
		s, input := isolationDelayedHostProcess(t, "slice-11-nonce-suffix")
		if _, err := io.WriteString(input, "continue\n"); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := isolationAwaitHostProcess(ctx, s.config); err != nil {
			t.Fatal(err)
		}
		s.config.Nonce = "slice-11-nonce"
		if _, err := s.hostControls(); err == nil {
			t.Fatal("nonce prefix passed as the host process identity")
		}
		ctx, cancelWrong := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancelWrong()
		if err := isolationAwaitHostProcess(ctx, s.config); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("wrong nonce passed readiness or failed before its deadline: %v", err)
		}
	})
	t.Run("changed start identity", func(t *testing.T) {
		s, _ := isolationDelayedHostProcess(t, "slice-11-changed-nonce")
		s.config.HostStart += "0"
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := isolationAwaitHostProcess(ctx, s.config); err == nil || errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("changed process identity was not rejected immediately: %v", err)
		}
		if _, err := s.hostControls(); err == nil {
			t.Fatal("changed process identity passed the host control")
		}
	})
}
