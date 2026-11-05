package command

import (
	"errors"
	"fmt"
	"github.com/donvargax/itos-container/internal/cli"
	"github.com/donvargax/itos-container/internal/slices/count/port"
	"strings"
	"testing"
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
			code:       cli.CodeEnvironment,
			rule:       "COUNT_INPUT_MISSING",
			message:    "the file absent.txt is not there",
			fix:        "check the path, or give - to count standard input",
			wantStderr: "itos-container: the file absent.txt is not there\n",
		},
		{
			failure:    &port.Unreadable{Path: "adirectory"},
			code:       cli.CodeEnvironment,
			rule:       "COUNT_INPUT_UNREADABLE",
			message:    "the file adirectory cannot be read as text",
			fix:        "give a file this program can read, not a folder",
			wantStderr: "itos-container: the file adirectory cannot be read as text\n",
		},
	} {
		var stdout, stderr strings.Builder
		ui := &cli.UI{Classify: Classify, Stdout: &stdout, Stderr: &stderr}
		if code := ui.Fail(c.failure, true); code != c.code || cli.Code(c.failure, Classify) != c.code {
			t.Errorf("%T: exit %d, not %d", c.failure, code, c.code)
		}
		if got := cli.Message(c.failure, Classify); got != c.message {
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

func TestClassifyWrappedAndUnknown(t *testing.T) {
	for _, err := range []error{&port.Missing{Path: "a"}, &port.Unreadable{Path: "a"}} {
		code, problems := Classify(fmt.Errorf("wrapped: %w", err))
		if code != cli.CodeEnvironment || len(problems) != 1 {
			t.Fatalf("wrapped error = %d, %+v", code, problems)
		}
	}
	if code, problems := Classify(errors.New("unknown")); code != 0 || problems != nil {
		t.Fatalf("unknown = %d, %+v", code, problems)
	}
}
