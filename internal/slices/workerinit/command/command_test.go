package command_test

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/slices/workerinit/command"
	"github.com/donvargax/itos-container/internal/slices/workerinit/port"
	"github.com/donvargax/itos-container/internal/slices/workerinit/port/porttest"
)

func TestCommandRefusals(t *testing.T) {
	boom := errors.New("boom secret-errno-text")
	run := func(args []string, abi int, abiErr, restrict, look, exec error) (int, string, []string) {
		rec := &porttest.Recorder{}
		l := &porttest.Landlock{Rec: rec, Version: abi, VersionErr: abiErr, RestrictErr: restrict}
		p := &porttest.Process{Rec: rec, Bare: map[string]string{"tool": "/bin/tool"}, Files: map[string]bool{"/bin/tool": true, "/x/y": true}, LookupErr: look, ExecErr: exec}
		var stderr bytes.Buffer
		code := command.Init{Landlock: l, Process: p}.Run(args, &stderr)
		return code, stderr.String(), rec.Calls
	}

	usage := [][]string{nil, {}, {"--"}, {"tool"}, {"-h"}, {"--help"}, {"--version"}, {"-h", "--", "tool"}, {"x", "--", "tool"}, {"---", "tool"}, {"", "tool"}}
	for _, args := range usage {
		code, stderr, calls := run(args, 10, nil, nil, nil, nil)
		if code != 125 || stderr != "itos-container-init: usage\n" || len(calls) != 0 {
			t.Errorf("usage %q = %d, %q, calls %v; want 125, the usage line, no call", args, code, stderr, calls)
		}
	}

	rows := []struct {
		name                          string
		abi                           int
		abiErr, restrict, look, execE error
		code                          int
		line                          string
		calls                         []string
	}{
		{"unavailable", 0, port.ErrUnavailable, nil, nil, nil, 125, "landlock-unavailable", []string{"abi"}},
		{"ABI 8", 8, nil, nil, nil, nil, 125, "landlock-unavailable", []string{"abi"}},
		{"ABI error", 0, boom, nil, nil, nil, 125, "landlock-failed", []string{"abi"}},
		{"restrict error", 10, nil, boom, nil, nil, 125, "landlock-failed", []string{"abi", "restrict"}},
		{"lookup error", 10, nil, nil, boom, nil, 127, "not-found", []string{"abi", "restrict", "lookup"}},
		{"exec not found", 10, nil, nil, nil, port.ErrNotFound, 127, "not-found", []string{"abi", "restrict", "lookup", "exec"}},
		{"exec error", 10, nil, nil, nil, boom, 126, "exec-failed", []string{"abi", "restrict", "lookup", "exec"}},
		{"exec returns nil", 10, nil, nil, nil, nil, 126, "exec-failed", []string{"abi", "restrict", "lookup", "exec"}},
	}
	for _, row := range rows {
		argv := []string{"--", "tool", "--", "SECRET=hunter2", "--help"}
		code, stderr, calls := run(argv, row.abi, row.abiErr, row.restrict, row.look, row.execE)
		if code != row.code || stderr != "itos-container-init: "+row.line+"\n" || !slices.Equal(calls, row.calls) {
			t.Errorf("%s = %d, %q, calls %v; want %d, %q, calls %v", row.name, code, stderr, calls, row.code, row.line, row.calls)
		}
		if strings.Contains(stderr, "hunter2") || strings.Contains(stderr, "tool") || strings.Contains(stderr, "secret") {
			t.Errorf("%s: stderr %q holds argv or error text", row.name, stderr)
		}
	}
}
