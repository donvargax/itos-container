package cli

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// An error of no kind in the sealed set is a defect of ours: 70, and never
// some other code a child process might have returned.
func TestAnErrorOfNoKnownKindIsInternal(t *testing.T) {
	cause := fmt.Errorf("cannot write the report: %w", errors.New("no space"))
	var stdout, stderr strings.Builder
	ui := &UI{Classify: syntheticClassify, Stdout: &stdout, Stderr: &stderr}
	if code := ui.Fail(cause, false); code != CodeInternal || Code(cause, syntheticClassify) != CodeInternal {
		t.Fatalf("Fail = %d and Code = %d, want %d", code, Code(cause, syntheticClassify), CodeInternal)
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
	joined := errors.Join(syntheticError("the file a.txt is not there"), syntheticError("the file b.txt cannot be read as text"))
	var stdout, stderr strings.Builder
	ui := &UI{Classify: syntheticClassify, Stdout: &stdout, Stderr: &stderr}
	if code := ui.Fail(joined, true); code != CodeEnvironment {
		t.Fatalf("Fail of two joined = %d, want %d", code, CodeEnvironment)
	}
	if got, want := strings.Count(stdout.String(), `"rule":`), 2; got != want {
		t.Errorf("--json holds %d problems, want %d\n%s", got, want, stdout.String())
	}
	if got, want := strings.Count(stderr.String(), "itos-container: "), 2; got != want {
		t.Errorf("stderr holds %d lines, want %d\n%s", got, want, stderr.String())
	}
	if got, want := Message(joined, syntheticClassify), "the file a.txt is not there\nthe file b.txt cannot be read as text"; got != want {
		t.Errorf("Message = %q, want %q", got, want)
	}
}

// A usage failure is exit 2, names the command as it was written, and says
// what to do next: a person reads it, a script reads its rule.
func TestUsageNamesTheCommandAndTheWayOut(t *testing.T) {
	var stdout, stderr strings.Builder
	ui := &UI{Classify: syntheticClassify, Stdout: &stdout, Stderr: &stderr}
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
	ui := &UI{Classify: syntheticClassify, Stdout: &stdout, Stderr: &stderr}
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
	ui := &UI{Classify: syntheticClassify, Stdout: failingWriter{}, Stderr: &stderr}
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

// Error groups may have no leaves; their aggregate has no failure code or
// message. Nested groups still preserve the highest code and every message.
type errorGroup []error

func (errorGroup) Error() string     { return "error group" }
func (g errorGroup) Unwrap() []error { return g }

func TestEmptyAndNestedErrorGroups(t *testing.T) {
	if got := Code(errorGroup{}, syntheticClassify); got != 0 {
		t.Errorf("empty group code = %d, want 0", got)
	}
	if got := Message(errorGroup{}, syntheticClassify); got != "" {
		t.Errorf("empty group message = %q", got)
	}
	group := errorGroup{syntheticError("the file a is not there"), errorGroup{errors.New("broken"), syntheticError("the file b cannot be read as text")}}
	if got := Code(group, syntheticClassify); got != CodeInternal {
		t.Errorf("nested group code = %d, want %d", got, CodeInternal)
	}
	if got, want := Message(group, syntheticClassify), "the file a is not there\nbroken\nthe file b cannot be read as text"; got != want {
		t.Errorf("nested group message = %q, want %q", got, want)
	}
}

type syntheticError string

func (e syntheticError) Error() string { return string(e) }
func syntheticClassify(err error) (int, []Problem) {
	var e syntheticError
	if errors.As(err, &e) {
		return CodeEnvironment, []Problem{{Rule: "synthetic", Message: e.Error(), Fix: "test fixture"}}
	}
	return 0, nil
}
func TestNoClassifierUsesTheInternalFallback(t *testing.T) {
	if got := Code(errors.New("unknown"), nil); got != CodeInternal {
		t.Fatalf("Code = %d", got)
	}
}
