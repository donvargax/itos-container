package main

import (
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/cli"
	"github.com/donvargax/itos-container/internal/slices/count/command"
	"github.com/donvargax/itos-container/internal/slices/count/input"
	"github.com/donvargax/itos-container/internal/slices/count/port"
	"github.com/donvargax/itos-container/internal/slices/count/port/porttest"
)

// A run's ends, so a test reads what the person or the script would.
type ends struct {
	stdout, stderr strings.Builder
}

// ui is a UI writing to ends, with stdin held as the run's input.
func ui(e *ends, stdin string) *cli.UI {
	return &cli.UI{Classify: command.Classify, In: strings.NewReader(stdin), Stdout: &e.stdout, Stderr: &e.stderr}
}

// count runs the command over files named in data, reading stdin for -.
func count(e *ends, stdin string, file string, data map[string][]byte, asJSON bool) int {
	files := input.Files{Disk: porttest.Files{Data: data}, In: strings.NewReader(stdin)}
	return command.CLI{Files: files, File: file, JSON: asJSON}.Run(ui(e, stdin))
}

// A person reads the counts and the path as given on stdout, and is told to
// use --json on stderr: the two are the plain output's whole contract here.
func TestPlainOutputIsTheCountsAndTheHint(t *testing.T) {
	var e ends
	if code := count(&e, "", "notes.txt", map[string][]byte{"notes.txt": []byte("one two three\nfour five")}, false); code != 0 {
		t.Fatalf("Run exit = %d, want 0", code)
	}
	if got, want := e.stdout.String(), "notes.txt: lines 2, words 5\n"; got != want {
		t.Errorf("standard output = %q, want %q", got, want)
	}
	if got, want := e.stderr.String(), "itos-container: for scripts, use --json\n"; got != want {
		t.Errorf("error output = %q, want %q", got, want)
	}
}

// The counts are never the input's own text: a file holding a secret must
// not reach the output.
func TestPlainOutputNeverPrintsWhatItRead(t *testing.T) {
	var e ends
	if code := count(&e, "", "secret.txt", map[string][]byte{"secret.txt": []byte("hunter2")}, false); code != 0 {
		t.Fatalf("Run exit = %d, want 0", code)
	}
	if strings.Contains(e.stdout.String(), "hunter2") {
		t.Errorf("standard output prints the input: %q", e.stdout.String())
	}
}

// One object on stdout, one line, and nothing on stderr: --json prints the
// hint nowhere, so a script's output is only what it reads.
func TestJSONOutputIsOneObjectAndNoHint(t *testing.T) {
	var e ends
	if code := count(&e, "", "notes.txt", map[string][]byte{"notes.txt": []byte("one two three\nfour five")}, true); code != 0 {
		t.Fatalf("Run exit = %d, want 0", code)
	}
	want := `{"schema":1,"ok":true,"file":"notes.txt","lines":2,"words":5}` + "\n"
	if got := e.stdout.String(); got != want {
		t.Errorf("standard output = %q, want %q", got, want)
	}
	if e.stderr.Len() != 0 {
		t.Errorf("error output = %q, want none", e.stderr.String())
	}
}

// Standard input is named - in the object too, so a script needs no second
// rule to tell where the counts came from.
func TestJSONOutputNamesStandardInputAsTheArgument(t *testing.T) {
	var e ends
	if code := count(&e, "one two three", "-", nil, true); code != 0 {
		t.Fatalf("Run exit = %d, want 0", code)
	}
	want := `{"schema":1,"ok":true,"file":"-","lines":1,"words":3}` + "\n"
	if got := e.stdout.String(); got != want {
		t.Errorf("standard output = %q, want %q", got, want)
	}
}

// A failure is the domain's, given its code by the UI: a missing file is
// the environment's, exit 3, and the object says so with the rule and what
// to do about it.
func TestAMissingFileIsTheEnvironments(t *testing.T) {
	var e ends
	if code := count(&e, "", "absent.txt", nil, true); code != 3 {
		t.Fatalf("Run exit = %d, want 3", code)
	}
	want := `{"schema":1,"ok":false,"problems":[{"rule":"COUNT_INPUT_MISSING",` +
		`"message":"the file absent.txt is not there",` +
		`"fix":"check the path, or give - to count standard input"}]}` + "\n"
	if got := e.stdout.String(); got != want {
		t.Errorf("standard output = %q, want %q", got, want)
	}
	if got, want := e.stderr.String(), "itos-container: the file absent.txt is not there\n"; got != want {
		t.Errorf("error output = %q, want %q", got, want)
	}
}

// Without --json a failure is a line for a person, and the code is still the
// failure's.
func TestAFailureWithoutJSONIsALineForAPerson(t *testing.T) {
	var e ends
	if code := count(&e, "", "absent.txt", nil, false); code != 3 {
		t.Fatalf("Run exit = %d, want 3", code)
	}
	if e.stdout.Len() != 0 {
		t.Errorf("standard output = %q, want none", e.stdout.String())
	}
	if got, want := e.stderr.String(), "itos-container: the file absent.txt is not there\n"; got != want {
		t.Errorf("error output = %q, want %q", got, want)
	}
}

// An unreadable input, a directory among them, is the environment's too, and
// names its own rule rather than the missing one's.
func TestAnUnreadableInputIsTheEnvironments(t *testing.T) {
	var e ends
	files := input.Files{
		Disk: porttest.Files{Failures: map[string]port.ReadFailure{"adir": porttest.Unreadable("adir")}},
		In:   strings.NewReader(""),
	}
	if code := (command.CLI{Files: files, File: "adir", JSON: true}).Run(ui(&e, "")); code != 3 {
		t.Fatalf("Run exit = %d, want 3", code)
	}
	if !strings.Contains(e.stdout.String(), `"rule":"COUNT_INPUT_UNREADABLE"`) {
		t.Errorf("standard output = %q, want the unreadable rule", e.stdout.String())
	}
}
