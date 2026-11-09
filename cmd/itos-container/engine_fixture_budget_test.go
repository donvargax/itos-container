package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/cli"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
	runfake "github.com/donvargax/itos-container/internal/slices/run/port/porttest"
)

type fixtureStartupApproval struct{ *runfake.Approval }

func (a fixtureStartupApproval) InspectContext(ctx context.Context) ap.View {
	<-ctx.Done()
	return a.Inspect()
}

func (a fixtureStartupApproval) FreezeContext(_ context.Context, v ap.View) (ap.Frozen, ap.Failure) {
	return a.Freeze(v)
}

// Runs both normally and in the harness's overlaid compilation. Every assertion
// goes through the real run command and shared-contract fakes, with no engine I/O.
func TestEngineFixtureBudgetContract(t *testing.T) {
	t.Logf("compiled budgets: call=%s startup=%s grace=%s", budget.CallDuration, budget.StartupDuration, budget.GraceDuration)
	for _, phase := range []string{"startup", "drain", "quiet and delayed signal"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := publicMemoryFixture(t)
				m.e.WaitFailure, m.e.Exit = "", 17
				ctx, cancel := context.WithCancelCause(context.Background())
				defer cancel(nil)
				runner := domain.Runner{Store: m.registry.Store, Registry: m.registry, Engine: m.e, Workspace: sf.MountWorkspace(m.b), Entropy: &publicMemoryEntropy{}}
				approval := &runfake.Approval{View: ap.View{Inspection: af.Ready(), Candidate: m.c, Record: ap.Record{Candidate: m.c, Generation: "1"}, Status: "matching"}}
				command := runcommand.CLI{Context: ctx, Approval: approval, Snapshots: &af.Snapshots{}, Runner: runner, Args: []string{"--", "probe"}}
				wantTime, wantCode, diagnostic := budget.CallDuration, 17, "run.stream-timeout"
				var cancelled time.Time
				var killAt time.Duration
				switch phase {
				case "startup":
					command.Approval = fixtureStartupApproval{approval}
					wantTime, wantCode, diagnostic = budget.StartupDuration, 75, "run.startup-timeout"
				case "drain":
					m.e.DrainUntilCancel = true
				case "quiet and delayed signal":
					quiet := max(budget.CallDuration, budget.StartupDuration) + time.Second
					wantTime, wantCode, diagnostic = quiet+budget.GraceDuration, 137, "run.cancelled"
					m.e.WaitUntilCancel, m.e.IgnoreSignal, m.e.Exit = true, true, 137
					m.e.AfterStart = func() {
						go func() {
							time.Sleep(quiet)
							cancelled = time.Now()
							cancel(sp.Interrupt{Signal: "SIGTERM"})
						}()
					}
					m.e.After = func(event string) {
						if event == "SIGTERM" {
							time.Sleep(3 * budget.GraceDuration / 5)
						}
						if event == "SIGKILL" {
							killAt = time.Since(cancelled)
						}
					}
				}
				var out, diagnostics bytes.Buffer
				start := time.Now()
				code := command.Run(&cli.UI{Stdout: &out, Stderr: &diagnostics, Classify: runcommand.Classify})
				if code != wantCode || time.Since(start) != wantTime || !strings.Contains(diagnostics.String(), diagnostic) {
					t.Fatalf("%s budget lost: exit %d, elapsed %s (want %s), diagnostic %s", phase, code, time.Since(start), wantTime, diagnostics.String())
				}
				if phase == "quiet and delayed signal" && killAt != budget.GraceDuration {
					t.Fatalf("acknowledgement restarted grace: SIGKILL at %s", killAt)
				}
				if phase != "startup" {
					r := m.service().Status(context.Background())
					if len(r.Failures) != 0 || len(r.Entries) != 1 || r.Entries[0].Cleanup != "complete" || len(m.e.Resources) != 0 {
						t.Fatal("fixture budgets lost owned cleanup", r)
					}
				} else if len(m.e.Events) != 0 {
					t.Fatal("startup expiry reached the engine", m.e.Events)
				}
			})
		})
	}
}
