package version

import (
	"runtime/debug"
	"testing"
)

func TestVersionWithoutBuildMetadata(t *testing.T) {
	previous := stamp
	stamp = ""
	t.Cleanup(func() { stamp = previous })
	if got := version(func() (*debug.BuildInfo, bool) { return nil, false }); got != Dev {
		t.Fatalf("version without build metadata = %q, want %q", got, Dev)
	}
}
