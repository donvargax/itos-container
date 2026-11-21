package process

import (
	"errors"
	"slices"
	"syscall"
	"testing"

	"github.com/donvargax/itos-container/internal/slices/workerinit/port"
	"github.com/donvargax/itos-container/internal/slices/workerinit/port/porttest"
)

func TestProcessContract(t *testing.T) {
	t.Run("fake", func(t *testing.T) {
		porttest.ProcessContract(t, func() (port.Process, porttest.ProcessWant) {
			return &porttest.Process{Rec: &porttest.Recorder{}, Bare: map[string]string{"tool": "/opt/bin/tool"}},
				porttest.ProcessWant{Bare: "tool", Resolved: "/opt/bin/tool", Missing: "absent", MissingPath: "/no/such/tool"}
		})
	})
	t.Run("real", testRealProcess)
}

func TestLookupDecisions(t *testing.T) {
	errLook := errors.New("look failed")
	rows := []struct {
		name  string
		input string
		found string
		err   error
		want  string
		fail  error
		asked bool
	}{
		{name: "slash path unchanged", input: "./rel/tool", want: "./rel/tool"},
		{name: "absolute path unchanged", input: "/missing/tool", want: "/missing/tool"},
		{name: "bare name found", input: "tool", found: "/usr/bin/tool", want: "/usr/bin/tool", asked: true},
		{name: "look error", input: "tool", err: errLook, fail: port.ErrNotFound, asked: true},
		{name: "relative result", input: "tool", found: "bin/tool", fail: port.ErrNotFound, asked: true},
		{name: "relative result with dot", input: "tool", found: "./tool", fail: port.ErrNotFound, asked: true},
		{name: "empty result", input: "tool", fail: port.ErrNotFound, asked: true},
		{name: "relative result with error", input: "tool", found: "./tool", err: errLook, fail: port.ErrNotFound, asked: true},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			var asked []string
			got, err := lookup(row.input, func(name string) (string, error) {
				asked = append(asked, name)
				return row.found, row.err
			})
			if got != row.want || !errors.Is(err, row.fail) || (err == nil) != (row.fail == nil) {
				t.Errorf("lookup(%q) = %q, %v; want %q, %v", row.input, got, err, row.want, row.fail)
			}
			if (len(asked) == 1 && asked[0] == row.input) != row.asked || len(asked) > 1 {
				t.Errorf("search calls = %q; want one for a bare name only (%v)", asked, row.asked)
			}
		})
	}
}

func TestReplaceDecisions(t *testing.T) {
	other := errors.New("other")
	for _, row := range []struct {
		name string
		raw  error
		want error
		nil_ bool
	}{
		{name: "ENOENT is not found", raw: syscall.ENOENT, want: port.ErrNotFound},
		{name: "wrapped ENOENT is not found", raw: errors.Join(other, syscall.ENOENT), want: port.ErrNotFound},
		{name: "EACCES stays itself", raw: syscall.EACCES, want: syscall.EACCES},
		{name: "other stays itself", raw: other, want: other},
		{name: "nil stays nil", nil_: true},
	} {
		t.Run(row.name, func(t *testing.T) {
			calls := 0
			argv, env := []string{"a", "", "--"}, []string{"K=v", "E="}
			err := replace("/bin/x", argv, env, func(path string, gotArgv, gotEnv []string) error {
				calls++
				if path != "/bin/x" || !slices.Equal(gotArgv, argv) || !slices.Equal(gotEnv, env) {
					t.Errorf("execve got %q %q %q; want /bin/x %q %q", path, gotArgv, gotEnv, argv, env)
				}
				return row.raw
			})
			if calls != 1 {
				t.Errorf("execve called %d times; want once", calls)
			}
			if row.nil_ && err != nil || !row.nil_ && !errors.Is(err, row.want) {
				t.Errorf("replace = %v; want %v", err, row.want)
			}
			if !row.nil_ && row.want != port.ErrNotFound && errors.Is(err, port.ErrNotFound) {
				t.Errorf("replace = %v; only ENOENT is not found", err)
			}
		})
	}
}

func TestExecPreservesArgvAndEnv(t *testing.T) { testExec(t) }
