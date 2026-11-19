package features

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/cucumber/godog"
)

// The exec scenarios' steps. Givens build disposable fixtures and seed
// protected records; they never invoke exec. The invocation steps run the
// command assembly's exec probe through both local backends, and a probe
// that does not exist yet is captured as data for the behaviour assertions
// in Then, which must fail first, never the setup.
func (w *world) execSteps(sc *godog.ScenarioContext) {
	sc.Step(`^an exec admission fixture with condition "([^"]*)"$`, func(condition string) error {
		return w.execFixture(condition)
	})
	sc.Step(`^the exec CLI requests a probe in the fixture session through both local backends$`, func() error {
		return w.execEachBackends(nil)
	})
	sc.Step(`^exec refuses with "([^"]*)" and exit (\d+) before creating or starting an exec$`, w.execRefused)
	sc.Step(`^the refused exec changed no session record, worker, unrelated resource or checkout file$`, w.execRefusalUnchanged)
}

func (w *world) execRefused(rule string, exit int) error {
	for _, backend := range []string{"memory", "disk"} {
		b, ok := w.exec.results[backend]
		if !ok {
			return fmt.Errorf("no %s backend result was recorded", backend)
		}
		r := b.result()
		if b.missing {
			return fmt.Errorf("%s backend published no exec behaviour evidence (subprocess exit %d)\n%s", backend, r.exit, r.stdout)
		}
		if r.exit != exit || !strings.Contains(r.stderr, rule) {
			return fmt.Errorf("%s: exec did not refuse with %q and exit %d\n%s", backend, rule, exit, w.execReport(backend))
		}
		if b.report.ExecCreate || b.report.ExecStart || b.report.StartedExecs != 0 {
			return fmt.Errorf("%s: the refused exec created or started an exec: %d started", backend, b.report.StartedExecs)
		}
		for _, q := range b.execRequests {
			if strings.HasPrefix(q.Path, "/containers/") && strings.HasSuffix(q.Path, "/exec") || strings.HasPrefix(q.Path, "/exec/") {
				return fmt.Errorf("%s: the refused exec reached the engine's exec endpoints: %s %s", backend, q.Method, q.Path)
			}
		}
	}
	return nil
}

func (w *world) execRefusalUnchanged() error {
	e := w.exec
	after, err := w.sessionProtected()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(e.immutable, after) {
		return fmt.Errorf("the refused exec changed protected state")
	}
	resourcesAfter := map[string]string{}
	for id, c := range w.session.api.local {
		resourcesAfter[id] = string(approvalJSON(c))
	}
	if !reflect.DeepEqual(e.resourcesBefore, resourcesAfter) {
		return fmt.Errorf("the refused exec changed an engine resource")
	}
	if lease := e.lease; lease != nil {
		select {
		case <-lease.done:
			return fmt.Errorf("the fixture's controller lease holder died")
		default:
		}
	}
	entries, err := os.ReadDir(w.doctor.recorder)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("checkout helper execution reached the recorder during exec")
	}
	return nil
}

// execReport is a backend's observed output for failure diagnostics.
func (w *world) execReport(backend string) string {
	b := w.exec.results[backend]
	if b.missing {
		return fmt.Sprintf("exit %d\n--- subprocess output\n%s", b.subprocess.exit, b.subprocess.stdout)
	}
	return fmt.Sprintf("exit %d\n--- stdout\n%s--- stderr\n%s", b.report.Exit, b.report.Stdout, b.report.Stderr)
}
