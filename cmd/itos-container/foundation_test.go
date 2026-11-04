package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func TestRunReportsAnInvalidCommandModel(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--json"}, strings.NewReader(""), &stdout, &stderr,
		kong.DynamicCommand("broken", "invalid command", "", &struct{ Value chan int }{}))
	if code != 70 || !strings.Contains(stderr.String(), "chan int") {
		t.Fatalf("invalid model returned %d, stderr %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"rule":"internal"`) {
		t.Fatalf("invalid model JSON = %q", stdout.String())
	}
}

func TestRunReportsACommandWithoutAHandler(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"orphan"}, strings.NewReader(""), &stdout, &stderr,
		kong.DynamicCommand("orphan", "command without a handler", "", &struct{}{}))
	if code != 70 || !strings.Contains(stderr.String(), `no case runs the command "orphan"`) {
		t.Fatalf("unhandled command returned %d, stderr %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("unhandled command wrote %q to stdout", stdout.String())
	}
}
