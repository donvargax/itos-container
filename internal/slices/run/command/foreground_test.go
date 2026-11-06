package command_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/cli"
	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	session "github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/donvargax/itos-container/internal/slices/run/command"
	rf "github.com/donvargax/itos-container/internal/slices/run/port/porttest"
)

func TestForegroundExitPrecedenceAndSeparateDiagnostics(t *testing.T) {
	for _, exit := range []int{0, 1, 2, 3, 7, 70, 75} {
		for _, cleanup := range []session.Outcome{session.Complete, session.Refused, session.Invalid, session.IdentityFailed, session.Incomplete} {
			a := &rf.Approval{View: approval.View{Status: "matching", Inspection: af.Ready(), Candidate: approval.Candidate{Project: "p", Fingerprint: "hash", Canonical: "selected"}, Record: approval.Record{Generation: "1"}}}
			r := &rf.Runner{Result: session.RunResult{Result: session.Result{Reference: "intents/safe.json", Cleanup: cleanup}, Exited: true, Exit: exit}, Output: []byte("out\x00\xff"), ErrorOutput: []byte("err\x00\xfe")}
			var out, errout strings.Builder
			c := command.CLI{Args: []string{"--", "probe", "", "two words", "--json"}, Approval: a, Snapshots: &af.Snapshots{}, Runner: r}
			code := c.Run(&cli.UI{Stdout: &out, Stderr: &errout, Classify: command.Classify})
			want := exit
			if exit == 0 {
				switch cleanup {
				case session.Refused:
					want = 1
				case session.Invalid:
					want = 2
				case session.IdentityFailed:
					want = 70
				case session.Incomplete:
					want = 75
				}
			}
			if code != want || out.String() != "out\x00\xff" || !strings.HasPrefix(errout.String(), "err\x00\xfe") {
				t.Fatalf("%d/%s: exit=%d out=%q err=%q", exit, cleanup, code, out.String(), errout.String())
			}
			if !slices.Equal(r.Argv, c.Args[1:]) || r.Frozen.Frozen.Settings != "selected" {
				t.Fatal("handoff changed")
			}
			if (cleanup != session.Complete) != strings.Contains(errout.String(), "run.cleanup-") {
				t.Fatal("lost cleanup outcome")
			}
		}
	}
}
func TestForegroundStartupFailureWinsOverCleanup(t *testing.T) {
	a := &rf.Approval{View: approval.View{Status: "matching", Inspection: af.Ready(), Candidate: approval.Candidate{Project: "p", Canonical: "selected"}, Record: approval.Record{Generation: "1"}}}
	r := &rf.Runner{Result: session.RunResult{Result: session.Result{Outcome: session.IsolationUnverified, Reference: "intents/safe.json", Cleanup: session.Incomplete}}}
	var out, errout strings.Builder
	c := command.CLI{Args: []string{"--", "probe"}, Approval: a, Snapshots: &af.Snapshots{}, Runner: r}
	if code := c.Run(&cli.UI{Stdout: &out, Stderr: &errout, Classify: command.Classify}); code != 1 || !strings.Contains(errout.String(), "run.isolation-unverified") || !strings.Contains(errout.String(), "run.cleanup-temporary") {
		t.Fatal(code, errout.String())
	}
	a.View.Status = "missing"
	r.Argv = nil
	if code := c.Run(&cli.UI{Stdout: &out, Stderr: &errout, Classify: command.Classify}); code != 1 || r.Argv != nil {
		t.Fatal("started without approval")
	}
}
