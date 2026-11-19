//go:build !windows

package signals

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	session "github.com/donvargax/itos-container/internal/engine/session/port"
	terminal "github.com/donvargax/itos-container/internal/engine/terminal/port"
	tf "github.com/donvargax/itos-container/internal/engine/terminal/port/porttest"
)

type signalCheck func(context.Context)

func (check signalCheck) Run(ctx context.Context, _ approval.Durable, _ []string, _ session.Streams) session.RunResult {
	check(ctx)
	return session.RunResult{}
}

// A terminal run must keep consuming hangup signals after its lease has
// reported the disconnect and stopped its own subscription, without turning
// one into a cancellation; interrupt forwarding is unchanged.
func TestCaptureConsumesHangupForTerminalRuns(t *testing.T) {
	provider := &tf.Fake{Size: terminal.Dimensions{Columns: 113, Rows: 37}}
	lease, err := provider.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	runner := Runner{Next: signalCheck(func(ctx context.Context) { checkTerminalSignals(t, ctx) })}
	runner.Run(t.Context(), approval.Durable{}, nil, session.Streams{Terminal: lease})
}

func checkTerminalSignals(t *testing.T, ctx context.Context) {
	t.Helper()
	if cause := context.Cause(ctx); cause != nil {
		t.Fatalf("a fresh capture reported a cause: %v", cause)
	}
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for context.Cause(ctx) != nil {
		if time.Now().After(deadline) {
			t.Fatalf("the consumed hangup still reported a cause: %v", context.Cause(ctx))
		}
		time.Sleep(time.Millisecond)
	}
	if _, ok := context.Cause(ctx).(session.Interrupt); ok {
		t.Fatal("a hangup cancelled the run")
	}
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	interrupted, ok := context.Cause(ctx).(session.Interrupt)
	for !ok && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		interrupted, ok = context.Cause(ctx).(session.Interrupt)
	}
	if !ok || interrupted.Signal != "SIGTERM" {
		t.Fatalf("interrupt forwarding changed: %v", context.Cause(ctx))
	}
}

// A plain run keeps the inherited hangup disposition: nothing subscribes SIGHUP
// on its behalf, so the default action must still terminate the process. The
// property is proven by a child that captures in plain mode, raises SIGHUP to
// itself and must die to it; a consumed hangup is reached only by sleeping out
// the child's failure marker.
func TestPlainCaptureLeavesHangupDeadly(t *testing.T) {
	if os.Getenv("ITOS_CONTAINER_TEST_PLAIN_HANGUP") == "1" {
		_, release := Capture(context.Background(), false)
		defer release()
		if err := syscall.Kill(syscall.Getpid(), syscall.SIGHUP); err != nil {
			os.Exit(3)
		}
		time.Sleep(20 * time.Second)
		os.Exit(1)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestPlainCaptureLeavesHangupDeadly$", "-test.count=1")
	cmd.Env = append(os.Environ(), "ITOS_CONTAINER_TEST_PLAIN_HANGUP=1")
	err := cmd.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("the plain-run child did not exit: %v", err)
	}
	status, ok := exit.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGHUP {
		t.Fatalf("a plain run did not die to its inherited hangup: exit=%d signaled=%t signal=%v", exit.ExitCode(), ok && status.Signaled(), status)
	}
}
