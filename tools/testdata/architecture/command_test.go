package architecture_test

import (
	"strings"
	"testing"
)

func TestSuccessfulCommandKeepsStderrOutOfValue(t *testing.T) {
	// A recovered curl failure belongs to diagnostics, not the --path value
	// that TestPolicy subsequently uses as its executable filename.
	got := must(t, t.TempDir(), "sh", "-c", "printf 'curl: recovered gateway timeout\\n' >&2; printf '/fixture/tool with spaces\\n'")
	if got != "/fixture/tool with spaces\n" {
		t.Fatalf("successful command value includes diagnostics: %q", got)
	}
}

func TestCommandFailureRetainsBothStreams(t *testing.T) {
	out, code := command(t, t.TempDir(), "sh", "-c", "printf 'stdout evidence\\n'; printf 'stderr evidence\\n' >&2; exit 7")
	if code != 7 || !strings.Contains(out, "stdout evidence\n") || !strings.Contains(out, "stderr evidence\n") {
		t.Fatalf("failed command lost its exit or diagnostics: code %d, output %q", code, out)
	}
}
