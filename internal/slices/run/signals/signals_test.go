package signals

import (
	"context"
	"syscall"
	"testing"
	"time"

	session "github.com/donvargax/itos-container/internal/engine/session/port"
)

// A terminal run must keep consuming hangup signals after its lease has
// reported the disconnect and stopped its own subscription, without turning
// one into a cancellation; interrupt forwarding is unchanged.
func TestCaptureConsumesHangupForTerminalRuns(t *testing.T) {
	ctx, release := Capture(context.Background(), true)
	defer release()
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

// A plain run keeps the inherited hangup behavior: nothing subscribes it, and
// no hangup is consumed on its behalf.
func TestCaptureLeavesPlainRunsAlone(t *testing.T) {
	ctx, release := Capture(context.Background(), false)
	defer release()
	select {
	case <-ctx.Done():
		t.Fatal("a plain capture ended before release")
	default:
	}
	if _, ok := context.Cause(ctx).(session.Interrupt); ok {
		t.Fatal("a plain capture invented a cancellation")
	}
}
