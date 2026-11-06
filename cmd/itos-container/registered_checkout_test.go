package main

import (
	"bytes"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/cli"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
	runfake "github.com/donvargax/itos-container/internal/slices/run/port/porttest"
)

func publicRegisteredCheckout(t *testing.T, change, boundary string) {
	t.Helper()
	for _, substitute := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged control", true: "substituted checkout"}[substitute], func(t *testing.T) {
			m := publicMemoryFixture(t)
			m.e.WaitFailure = ""
			before := bytes.Clone(m.storage.Directories["snapshots/"+m.c.Project].Files["1.json"])
			barrier := false
			m.e.After = func(event string) {
				if event != boundary || barrier {
					return
				}
				barrier = true
				if substitute {
					if strings.HasPrefix(change, "root") {
						m.checkout.Identity.Inode += "1"
					} else {
						m.checkout.Identity.GitInode += "1"
					}
				}
			}
			runner := domain.Runner{Store: m.registry.Store, Registry: m.registry, Engine: m.e, Workspace: &sf.Workspace{}, Entropy: &publicMemoryEntropy{}}
			approval := &runfake.Approval{View: ap.View{Inspection: af.Ready(), Candidate: m.c, Record: ap.Record{Candidate: m.c, Generation: "1"}, Status: "matching"}}
			var out, errout bytes.Buffer
			code := (runcommand.CLI{Approval: approval, Snapshots: &af.Snapshots{}, Runner: runner, Args: []string{"--", "probe"}}).Run(&cli.UI{Stdout: &out, Stderr: &errout, Classify: runcommand.Classify})
			if substitute {
				if code != 1 || !strings.Contains(errout.String(), "run.ownership") || slices.Contains(m.e.Events, "start") || (boundary == "open" && slices.Contains(m.e.Events, "create")) {
					t.Fatalf("registered launch accepted substituted checkout: exit %d, events %v, %s", code, m.e.Events, errout.String())
				}
			} else if code != 0 || errout.Len() != 0 || !slices.Contains(m.e.Events, "start") {
				t.Fatal("unchanged approved checkout could not run", code, m.e.Events, errout.String())
			}
			if !barrier || len(m.files.Sessions) != 1 || len(m.e.Resources) != 0 || !bytes.Equal(before, m.storage.Directories["snapshots/"+m.c.Project].Files["1.json"]) {
				t.Fatal("launch lost protected evidence or left a created worker")
			}
			// Keep the changed identity in place for fresh status/stop. A launch
			// refusal must not accidentally make cleanup depend on that identity.
			wantCode, wantCleanup := 0, "complete"
			if substitute && boundary == "open" {
				wantCode, wantCleanup = 75, "unknown"
			}
			status := publicMemoryCommand(t, m.service(), "status", "", wantCode)
			if len(status.Sessions) != 1 || status.Sessions[0].Project != m.c.Project || status.Sessions[0].Fingerprint != m.c.Fingerprint || status.Sessions[0].Cleanup != wantCleanup {
				t.Fatal("fresh discovery lost original binding after launch", status)
			}
			entry := status.Sessions[0]
			if substitute && entry.WorkloadExit != nil {
				t.Fatal("refused launch invented workload exit", entry)
			}
			stopped := publicMemoryCommand(t, m.service(), "stop", entry.ID, wantCode)
			if len(stopped.Sessions) != 1 || stopped.Sessions[0].Cleanup != wantCleanup || !reflect.DeepEqual(stopped.Sessions[0].WorkloadExit, entry.WorkloadExit) {
				t.Fatal("fresh stop lost recovery after checkout substitution", stopped)
			}
			for _, files := range m.files.Sessions {
				if files.Controller {
					t.Fatal("run retained its controller lease")
				}
			}
			if substitute && boundary == "open" && !slices.Equal(entry.Retry, []string{"itos", "container", "stop", entry.ID}) {
				t.Fatal("pre-create uncertainty lost its exact retry", entry)
			}
			if substitute && boundary == "attach" && (!m.e.Removed || slices.Contains(m.e.Events, "SIGTERM") || slices.Contains(m.e.Events, "SIGKILL")) {
				t.Fatal("unstarted worker was signalled or not removed", m.e.Events)
			}
		})
	}
}
