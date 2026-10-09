package cli

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/count/port"
)

// Every kind the counting port's sealed set has is read by a scenario by its
// exit code and its --json rule ID. Both are the contract a script relies
// on, and they are checked here whole: a kind whose code or rule changed
// would break every script that reads it, and no other check here would say
// so.
func TestEachReadFailureKindHasItsCodeRuleAndSentence(t *testing.T) {
	for _, c := range []struct {
		failure    port.ReadFailure
		code       int
		rule       string
		message    string
		fix        string
		wantStderr string
	}{
		{
			failure:    &port.Missing{Path: "absent.txt"},
			code:       CodeEnvironment,
			rule:       "COUNT_INPUT_MISSING",
			message:    "the file absent.txt is not there",
			fix:        "check the path, or give - to count standard input",
			wantStderr: "itos-container: the file absent.txt is not there\n",
		},
		{
			failure:    &port.Unreadable{Path: "adirectory"},
			code:       CodeEnvironment,
			rule:       "COUNT_INPUT_UNREADABLE",
			message:    "the file adirectory cannot be read as text",
			fix:        "give a file this program can read, not a folder",
			wantStderr: "itos-container: the file adirectory cannot be read as text\n",
		},
	} {
		var stdout, stderr strings.Builder
		ui := &UI{Stdout: &stdout, Stderr: &stderr}
		if code := ui.Fail(c.failure, true); code != c.code || Code(c.failure) != c.code {
			t.Errorf("%T: exit %d, not %d", c.failure, code, c.code)
		}
		if got := Message(c.failure); got != c.message {
			t.Errorf("%T says\n%s, not\n%s", c.failure, got, c.message)
		}
		if stderr.String() != c.wantStderr {
			t.Errorf("%T: stderr is %q, want %q", c.failure, stderr.String(), c.wantStderr)
		}
		want := `{"schema":1,"ok":false,"problems":[{"rule":"` + c.rule +
			`","message":"` + c.message + `","fix":"` + c.fix + `"}]}` + "\n"
		if got := stdout.String(); got != want {
			t.Errorf("%T: --json is %q, want %q", c.failure, got, want)
		}
	}
}

// An error of no kind in the sealed set is a defect of ours: 70, and never
// some other code a child process might have returned.
func TestAnErrorOfNoKnownKindIsInternal(t *testing.T) {
	cause := fmt.Errorf("cannot write the report: %w", errors.New("no space"))
	var stdout, stderr strings.Builder
	ui := &UI{Stdout: &stdout, Stderr: &stderr}
	if code := ui.Fail(cause, false); code != CodeInternal || Code(cause) != CodeInternal {
		t.Fatalf("Fail = %d and Code = %d, want %d", code, Code(cause), CodeInternal)
	}
	if got, want := stdout.String(), ""; got != want {
		t.Errorf("standard output = %q, want none without --json", got)
	}
	if got := stderr.String(); !strings.HasPrefix(got, "itos-container: cannot write the report:") {
		t.Errorf("stderr = %q", got)
	}
}

// Two failures joined are each a problem and a line of their own, and the
// code is the higher of theirs: one joined error never hides another.
func TestJoinedFailuresAreEachAProblemUnderOneCode(t *testing.T) {
	joined := errors.Join(&port.Missing{Path: "a.txt"}, &port.Unreadable{Path: "b.txt"})
	var stdout, stderr strings.Builder
	ui := &UI{Stdout: &stdout, Stderr: &stderr}
	if code := ui.Fail(joined, true); code != CodeEnvironment {
		t.Fatalf("Fail of two joined = %d, want %d", code, CodeEnvironment)
	}
	if got, want := strings.Count(stdout.String(), `"rule":`), 2; got != want {
		t.Errorf("--json holds %d problems, want %d\n%s", got, want, stdout.String())
	}
	if got, want := strings.Count(stderr.String(), "itos-container: "), 2; got != want {
		t.Errorf("stderr holds %d lines, want %d\n%s", got, want, stderr.String())
	}
	if got, want := Message(joined), "the file a.txt is not there\nthe file b.txt cannot be read as text"; got != want {
		t.Errorf("Message = %q, want %q", got, want)
	}
}

// A usage failure is exit 2, names the command as it was written, and says
// what to do next: a person reads it, a script reads its rule.
func TestUsageNamesTheCommandAndTheWayOut(t *testing.T) {
	var stdout, stderr strings.Builder
	ui := &UI{Stdout: &stdout, Stderr: &stderr}
	if code := ui.Usage("count", errors.New(`expected "<file>"`), true); code != CodeUsage {
		t.Fatalf("Usage exit = %d, want %d", code, CodeUsage)
	}
	if got, want := stderr.String(), "itos-container: count: expected \"<file>\"\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
	want := `{"schema":1,"ok":false,"problems":[{"rule":"usage",` +
		`"message":"count: expected \"<file>\"",` +
		`"fix":"run itos-container --help for the commands and the flags each takes"}]}` + "\n"
	if got := stdout.String(); got != want {
		t.Errorf("--json = %q, want %q", got, want)
	}
}

// A usage error of a line that names no command says what was wrong and
// nothing more: there is no command to name.
func TestUsageOfALineNamingNoCommandSaysOnlyWhatWasWrong(t *testing.T) {
	var stdout, stderr strings.Builder
	ui := &UI{Stdout: &stdout, Stderr: &stderr}
	if code := ui.Usage("", errors.New("unknown flag --bogus"), false); code != CodeUsage {
		t.Fatalf("Usage exit = %d, want %d", code, CodeUsage)
	}
	if got, want := stderr.String(), "itos-container: unknown flag --bogus\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

// A writer that cannot hold what it is given is a line on stderr, not a
// silent loss: --json's contract is what a script reads.
func TestAFailureToWriteJSONIsALineOnStderr(t *testing.T) {
	var stderr strings.Builder
	ui := &UI{Stdout: failingWriter{}, Stderr: &stderr}
	ui.JSON(struct {
		Schema int `json:"schema"`
	}{1})
	if got, want := stderr.String(), "itos-container: the disk is full\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
}

// failingWriter is a writer that cannot be written, whatever a machine makes
// one: no disk, no permission bit.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("the disk is full") }
