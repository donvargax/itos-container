package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The logs are held here: a run is quiet by default, and nothing count
// logs below a warning, so no scenario sees a log line.

// A warning to a writer that is not a terminal is one JSON object, with its
// level, its message and its attributes.
func TestLoggerWritesAWarningAsOneJSONObject(t *testing.T) {
	var buf bytes.Buffer
	logger(&buf).Warn("could not remove a folder", "path", "/tmp/x", "tries", 3)

	var line map[string]any
	dec := json.NewDecoder(&buf)
	if err := dec.Decode(&line); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if dec.More() {
		t.Fatalf("more than one object: %q", buf.String())
	}
	want := map[string]any{"level": "WARN", "msg": "could not remove a folder", "path": "/tmp/x", "tries": 3.0}
	for k, v := range want {
		if line[k] != v {
			t.Errorf("%s = %v, want %v", k, line[k], v)
		}
	}
}

// An error is logged too.
func TestLoggerWritesAnError(t *testing.T) {
	var buf bytes.Buffer
	logger(&buf).Error("broken")
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil || line["level"] != "ERROR" {
		t.Errorf("logged %q", buf.String())
	}
}

// Below a warning, nothing: a run is quiet by default.
func TestLoggerWritesNothingBelowAWarning(t *testing.T) {
	var buf bytes.Buffer
	l := logger(&buf)
	l.Info("starting")
	l.Debug("details")
	if buf.Len() != 0 {
		t.Errorf("logged %q", buf.String())
	}
}

// A terminal gets text, even when the test runner itself has redirected I/O.
func TestLoggerWritesTextToATerminal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terminal-log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	l := loggerFor(f, func(fd int) bool {
		if fd != int(f.Fd()) {
			t.Fatalf("terminal detector received fd %d, want %d", fd, f.Fd())
		}
		return true
	})
	l.Info("quiet")
	l.Warn("disk full", "path", "report")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"level=WARN", `msg="disk full"`, "path=report"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("terminal log %q lacks %q", data, want)
		}
	}
	if strings.Contains(string(data), "quiet") {
		t.Errorf("terminal log includes an info message: %q", data)
	}
}

// A file that is no terminal, a run's output redirected, gets JSON: a
// script's logs are machine-readable wherever a person is not reading.
func TestLoggerWritesJSONToARedirectedRun(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "log"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if h := logger(f).Handler(); isText(h) {
		t.Errorf("handler %T, want *slog.JSONHandler", h)
	}
}

// A writer that is not a file at all, as a test's buffer, is no terminal
// either.
func TestLoggerWritesJSONToAWriterThatIsNotAFile(t *testing.T) {
	var buf bytes.Buffer
	if h := logger(&buf).Handler(); isText(h) {
		t.Errorf("handler %T, want *slog.JSONHandler", h)
	}
}

// isText is whether h is the text handler, and not the JSON one.
func isText(h slog.Handler) bool {
	_, ok := h.(*slog.TextHandler)
	return ok
}

// A command line kong could not parse still asks for --json, or not: the
// last of --json and --no-json before any -- decides, and none means no. No
// scenario gives a usage error every one of these ways.
func TestWantsJSONReadsTheLastJSONFlagBeforeDashDash(t *testing.T) {
	for _, c := range []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"count", "--bogus"}, false},
		{[]string{"count", "--json"}, true},
		{[]string{"--json=true", "count"}, true},
		{[]string{"--json", "--no-json"}, false},
		{[]string{"--json", "--json=false"}, false},
		{[]string{"--no-json", "--json"}, true},
		{[]string{"--", "--json"}, false},
		{[]string{"--json", "--", "--no-json"}, true},
	} {
		if got := wantsJSON(c.args); got != c.want {
			t.Errorf("wantsJSON(%q) = %v, want %v", c.args, got, c.want)
		}
	}
}

// No arguments at all: no implicit default action, so the program says what
// it can do on stdout and succeeds.
func TestRunWithNoArgumentsPrintsTheHelpAndSucceeds(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("run with no arguments returned %d, want 0", code)
	}
	for _, want := range []string{"count", "version", "completion"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("the help does not name %q\n%s", want, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Errorf("the help wrote to stderr: %q", stderr.String())
	}
}

// A usage error kong finds before any command runs is the one failure main
// reports itself, as a line on stderr and exit 2.
func TestRunReportsAUsageErrorAsAUsageExit(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"count"}, strings.NewReader(""), &stdout, &stderr); code != 2 {
		t.Fatalf("run of count with no file returned %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "itos-container: ") {
		t.Errorf("stderr = %q, want a line prefixed for a person", stderr.String())
	}
}

// The completion protocol answers from kong's model and never runs a
// command: __complete is the shell's, and it lists what a new word fits.
func TestRunOfCompleteAnswersFromTheModelWithoutRunningACommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"__complete", ""}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("run of __complete returned %d, want 0", code)
	}
	for _, want := range []string{"count", "version", "completion"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("__complete does not offer %q\n%s", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "__complete") {
		t.Errorf("__complete offers itself\n%s", stdout.String())
	}
}

// The stamped version and commit the scenario harness builds with: run
// stamps what a release's build stamps, and this is where that is read.
func TestRunPrintsTheStampVersionAndCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"version"}, strings.NewReader(""), &stdout, &stderr); code != 0 {
		t.Fatalf("run of version returned %d, want 0", code)
	}
	if !strings.HasPrefix(stdout.String(), "itos-container ") {
		t.Errorf("version printed %q", stdout.String())
	}
}

// A line longer than any reader's token limit is still one line and is
// counted whole, so the answer does not depend on the length of a line.
func TestCountReadsALineWholePastAnyTokenLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "long.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 100000)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"count", path, "--json"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run of count returned %d: %s", code, stderr.String())
	}
	// The file is written as JSON writes it, whose escaping is the encoder's
	// and not this test's: a Windows path holds backslashes, and the answer a
	// script reads is the one with them escaped.
	name, err := json.Marshal(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema":1,"ok":true,"file":` + string(name) + `,"lines":1,"words":1}` + "\n"
	if got := stdout.String(); got != want {
		t.Errorf("standard output = %q, want %q", got, want)
	}
}

// The command a line names, as the usage failure about it is written: the
// first word that is neither a flag nor a flag's value, and "" where the line
// names none. No scenario begins a line with a flag, so nothing else reads
// this.
func TestCommandNameSkipsFlagsAndTheirValues(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, ""},
		{[]string{"count", "notes.txt"}, "count"},
		{[]string{"--", "count"}, ""},
		// A flag's value is the next word unless the flag took it as
		// --flag=value, which is one word: a word after a bare flag is that
		// flag's value, and never names the command.
		{[]string{"--json", "count", "notes.txt"}, "notes.txt"},
		{[]string{"--json=yes", "count", "notes.txt"}, "count"},
		{[]string{"--json", "count"}, ""},
		{[]string{"--json"}, ""},
		// A lone - is a command's argument, the file read from standard
		// input, and not a flag.
		{[]string{"-", "notes.txt"}, "-"},
	} {
		if got := commandName(c.args); got != c.want {
			t.Errorf("commandName(%q) = %q, want %q", c.args, got, c.want)
		}
	}
}
