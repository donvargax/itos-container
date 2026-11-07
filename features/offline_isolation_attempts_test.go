package features

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestIsolationRouteAttempts(t *testing.T) {
	// Compile only the scheduler and its in-memory clients, on every platform.
	// The native probe and its real network clients never execute on the host.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "-count=1", "testdata/slice-14/route_attempts.go", "testdata/slice-14/route_attempts_test.go")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("route attempt scheduling regressions: %v\n%s", err, out)
	}
}
