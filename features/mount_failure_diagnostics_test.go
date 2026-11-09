package features

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// Capture controller diagnostics at the failing assertion, before harness
// cleanup can destroy the processes or subordinate worlds. Never dump workload
// streams, request bodies, argv, environment or entire fixture records.
func (w *world) reportMountDiagnosticFailure(ctx context.Context, _ *godog.Step, _ godog.StepResultStatus, err error) (context.Context, error) {
	if err != nil {
		if detail := w.mountFailureDiagnostics(); detail != "" {
			return ctx, fmt.Errorf("controller mount diagnostics:\n%s", detail)
		}
	}
	return ctx, nil
}

func (w *world) mountFailureDiagnostics() string {
	var lines []string
	seen := map[*world]bool{}
	add := func(label, stderr string) {
		for _, line := range strings.Split(stderr, "\n") {
			if strings.HasPrefix(line, "itos-container: run.mount-") {
				lines = append(lines, label+": "+strconv.QuoteToASCII(line))
			}
		}
	}
	var visit func(*world, string)
	visit = func(w *world, label string) {
		if w == nil || seen[w] {
			return
		}
		seen[w] = true
		add(label, w.stderr)
		if w.offline != nil {
			add(label+"/offline", string(w.offline.stderr))
			if w.offline.isolation != nil {
				for i, run := range w.offline.isolation.runs {
					add(fmt.Sprintf("%s/isolation-%d", label, i), string(run.stderr))
				}
			}
		}
		if w.recovery != nil {
			for i, p := range w.recovery.processes {
				add(fmt.Sprintf("%s/process-%d", label, i), string(p.errout.bytes()))
			}
			visit(w.recovery.second, label+"/second")
		}
		if w.anchor != nil {
			visit(w.anchor.control, label+"/anchor-control")
		}
		if w.lifecycle != nil {
			visit(w.lifecycle.control, label+"/lifecycle-control")
		}
		if w.overlap != nil {
			var names []string
			for name := range w.overlap.worlds {
				names = append(names, name)
			}
			slices.Sort(names)
			for _, name := range names {
				visit(w.overlap.worlds[name], label+"/"+name)
			}
			for i, run := range w.overlap.runs {
				if run.process != nil {
					add(fmt.Sprintf("%s/overlap-%d", label, i), string(run.process.errout.bytes()))
				}
			}
		}
		if w.hardlinks != nil {
			for i, run := range w.hardlinks.native {
				add(fmt.Sprintf("%s/hardlink-%d", label, i), run.result.stderr)
			}
		}
	}
	visit(w, "scenario")
	return strings.Join(lines, "\n")
}

func TestMountFailureDiagnostics(t *testing.T) {
	const diagnostic = `itos-container: run.mount-refused check=mounted-stat expected=1:2 errno=13 error="permission denied"`
	p := &recoveryProcess{}
	_, _ = p.errout.Write([]byte("private workload output\n" + diagnostic + "\n"))
	control := &world{offline: &offlineRuntime{isolation: &isolationWorld{runs: []*isolationRun{{stderr: []byte(diagnostic)}}}}}
	w := &world{recovery: &recoveryNative{processes: []*recoveryProcess{p}, second: control}, anchor: &anchorWorld{control: control}}
	control.anchor = &anchorWorld{control: w} // Shared/cyclic references must be finite.
	got := w.mountFailureDiagnostics()
	if strings.Count(got, "run.mount-refused") != 2 || !strings.Contains(got, "expected=1:2") || strings.Contains(got, "private workload") {
		t.Fatal(got)
	}
	if _, err := w.reportMountDiagnosticFailure(t.Context(), nil, godog.StepPassed, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := (&world{}).reportMountDiagnosticFailure(t.Context(), nil, godog.StepFailed, fmt.Errorf("original")); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	suite := godog.TestSuite{Options: &godog.Options{Format: "pretty", Output: &output, FeatureContents: []godog.Feature{{Name: "diagnostic.feature", Contents: []byte("Feature: diagnostics\nScenario: original failure\nThen the native assertion fails\n")}}}, ScenarioInitializer: func(sc *godog.ScenarioContext) {
		sc.Step(`^the native assertion fails$`, func() error { return fmt.Errorf("original assertion failure") })
		sc.StepContext().After(w.reportMountDiagnosticFailure)
	}}
	if suite.Run() == 0 || !strings.Contains(output.String(), "original assertion failure") || !strings.Contains(output.String(), "check=mounted-stat") {
		t.Fatal(output.String())
	}
}
