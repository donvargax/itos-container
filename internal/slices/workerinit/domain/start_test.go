package domain_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/donvargax/itos-container/internal/slices/workerinit/domain"
	"github.com/donvargax/itos-container/internal/slices/workerinit/port"
	"github.com/donvargax/itos-container/internal/slices/workerinit/port/porttest"
)

func TestStartRestrictsBeforeExec(t *testing.T) {
	boom := errors.New("boom")
	rows := []struct {
		name  string
		abi   int
		abiEr error
		rest  error
		look  error
		exec  error
		want  domain.Code
		calls []string
	}{
		{name: "exec returning nil", abi: 9, want: domain.ExecFailed, calls: []string{"abi", "restrict", "lookup", "exec"}},
		{name: "newer ABI", abi: 10, exec: boom, want: domain.ExecFailed, calls: []string{"abi", "restrict", "lookup", "exec"}},
		{name: "unavailable", abiEr: port.ErrUnavailable, want: domain.Unavailable, calls: []string{"abi"}},
		{name: "wrapped unavailable", abiEr: errors.Join(boom, port.ErrUnavailable), want: domain.Unavailable, calls: []string{"abi"}},
		{name: "ABI 8", abi: 8, want: domain.Unavailable, calls: []string{"abi"}},
		{name: "ABI 0", abi: 0, want: domain.Unavailable, calls: []string{"abi"}},
		{name: "ABI read error", abiEr: boom, want: domain.Failed, calls: []string{"abi"}},
		{name: "restrict error", abi: 9, rest: boom, want: domain.Failed, calls: []string{"abi", "restrict"}},
		{name: "lookup error", abi: 9, look: boom, want: domain.NotFound, calls: []string{"abi", "restrict", "lookup"}},
		{name: "lookup not found", abi: 9, look: port.ErrNotFound, want: domain.NotFound, calls: []string{"abi", "restrict", "lookup"}},
		{name: "exec not found", abi: 9, exec: port.ErrNotFound, want: domain.NotFound, calls: []string{"abi", "restrict", "lookup", "exec"}},
		{name: "wrapped exec not found", abi: 9, exec: errors.Join(boom, port.ErrNotFound), want: domain.NotFound, calls: []string{"abi", "restrict", "lookup", "exec"}},
		{name: "exec other error", abi: 9, exec: boom, want: domain.ExecFailed, calls: []string{"abi", "restrict", "lookup", "exec"}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			rec := &porttest.Recorder{}
			l := &porttest.Landlock{Rec: rec, Version: row.abi, VersionErr: row.abiEr, RestrictErr: row.rest}
			p := &porttest.Process{Rec: rec, Bare: map[string]string{"tool": "/opt/bin/tool"}, Files: map[string]bool{"/opt/bin/tool": true}, LookupErr: row.look, ExecErr: row.exec}
			argv := []string{"tool", "--", "-h", ""}
			got := domain.Start(l, p, argv)
			if got != row.want {
				t.Errorf("Start = %q; want %q", got, row.want)
			}
			if !slices.Equal(rec.Calls, row.calls) {
				t.Errorf("calls = %v; want %v", rec.Calls, row.calls)
			}
			if !slices.Contains(row.calls, "exec") {
				return
			}
			if p.ExecPath != "/opt/bin/tool" && row.look == nil {
				t.Errorf("Exec path = %q; want the path Lookup returned", p.ExecPath)
			}
			if !slices.Equal(p.ExecArgv, argv) || !slices.Equal(p.Looked, []string{"tool"}) {
				t.Errorf("Exec argv = %q, looked %q; want argv %q and argv[0] looked up", p.ExecArgv, p.Looked, argv)
			}
			if len(p.ExecArgv) == 0 {
				return
			}
			p.ExecArgv[0] = "changed"
			if argv[0] != "tool" {
				t.Error("Exec argv shares storage with the input")
			}
		})
	}
}
