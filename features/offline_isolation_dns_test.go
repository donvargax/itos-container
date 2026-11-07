package features

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestIsolationDNSWire(t *testing.T) {
	// Test the pure packet helpers on every platform without compiling or
	// executing the native probe, opening sockets or selecting engine scenarios.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "-count=1", "testdata/slice-14/dns_wire.go", "testdata/slice-14/dns_wire_test.go")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("DNS wire regressions: %v\n%s", err, out)
	}
}
