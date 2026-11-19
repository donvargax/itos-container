package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// Exercise the same literal argv entry point as main. The feature harness
// supplies a private checkout, state and recording endpoint in the environment.
func TestPublicExecCommand(t *testing.T) {
	path := os.Getenv("SLICE_38_EXEC_INPUT")
	if path == "" {
		t.Skip("exec feature subprocess only")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		SessionID, Stdin, Output string
		Argv                     []string
	}
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	started := time.Now()
	exit := run(append([]string{"exec", input.SessionID, "--"}, input.Argv...), strings.NewReader(input.Stdin), &stdout, &stderr)
	report := struct {
		Exit           int
		Stdout, Stderr string
		ElapsedMS      int64
	}{exit, stdout.String(), stderr.String(), time.Since(started).Milliseconds()}
	data, err = json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(input.Output, data, 0600); err != nil {
		t.Fatal(err)
	}
}
