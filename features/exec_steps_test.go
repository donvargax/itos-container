package features

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

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
	sc.Step(`^an exec admission fixture that substitutes "([^"]*)" at "([^"]*)"$`, w.execSubstitutionFixture)
	sc.Step(`^the exec attempt and its unchanged control run through both local backends$`, w.execAttemptAndControl)
	sc.Step(`^the changed attempt reports "exec.not-owned" with exit 1 and the control exits 23$`, w.execSubstitutedRefusal)
	sc.Step(`^no exec start or cleanup mutation targets substituted authority and no late reply resumes launch$`, w.execNoLateLaunch)
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

// execSubstitutionFixture arms one observation barrier on otherwise valid
// original authority. The control proves the fixture admits a healthy exec;
// the armed barrier proves the next revalidation refuses what changed.
func (w *world) execSubstitutionFixture(authority, boundary string) error {
	if err := w.execFixture(""); err != nil {
		return err
	}
	e := w.exec
	switch authority {
	case "protected intent", "full worker ID and labels", "pinned engine socket", "original daemon identity":
	case "returned exec belongs to another container", "returned exec ID differs from the created ID":
		if boundary != "before exec start" {
			return fmt.Errorf("the %q lie is only observable before exec start", authority)
		}
		e.execLie = authority
	default:
		return fmt.Errorf("unknown substitution authority %q", authority)
	}
	if boundary != "before exec create" && boundary != "before exec start" {
		return fmt.Errorf("unknown substitution boundary %q", boundary)
	}
	e.substituteAuthority, e.substituteBoundary = authority, boundary
	e.execExit = 23
	e.execStdout, e.execStderr = "slice-38-control-stdout\n", "slice-38-control-stderr\n"
	return nil
}

// execAttemptAndControl runs the healthy control through both backends
// first, with the barrier disarmed, then the attempt with it armed: the
// control sees original authority, the attempt's next revalidation sees
// what the fixture substituted.
func (w *world) execAttemptAndControl() error {
	authority, boundary := w.exec.substituteAuthority, w.exec.substituteBoundary
	w.exec.substituteAuthority = ""
	for _, backend := range []string{"memory", "disk"} {
		if err := w.execInvoke(backend+"-control", backend, execProbeRequest{}); err != nil {
			return err
		}
	}
	w.exec.substituteAuthority, w.exec.substituteBoundary = authority, boundary
	for _, backend := range []string{"memory", "disk"} {
		if err := w.execInvoke(backend+"-attempt", backend, execProbeRequest{}); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) execSubstitutedRefusal() error {
	for _, backend := range []string{"memory", "disk"} {
		attempt, ok := w.exec.results[backend+"-attempt"]
		if !ok {
			return fmt.Errorf("no %s attempt result was recorded", backend)
		}
		control, ok := w.exec.results[backend+"-control"]
		if !ok {
			return fmt.Errorf("no %s control result was recorded", backend)
		}
		if attempt.missing || control.missing {
			return fmt.Errorf("%s backend published no exec behaviour evidence\n%s", backend, w.execReport(backend+"-attempt"))
		}
		r := attempt.result()
		if r.exit != 1 || !strings.Contains(r.stderr, "exec.not-owned") {
			return fmt.Errorf("%s: the substituted attempt was not refused as not-owned\n%s", backend, w.execReport(backend+"-attempt"))
		}
		if attempt.report.ExecStart || attempt.report.StartedExecs != 0 {
			return fmt.Errorf("%s: the substituted attempt started an exec", backend)
		}
		c := control.result()
		if c.exit != 23 || control.report.StartedExecs != 1 {
			return fmt.Errorf("%s: the unchanged control did not start and exit 23\n%s", backend, w.execReport(backend+"-control"))
		}
		for _, q := range attempt.execRequests {
			if strings.HasPrefix(q.Path, "/exec/") && strings.HasSuffix(q.Path, "/start") {
				return fmt.Errorf("%s: the substituted attempt's exec was started", backend)
			}
			if q.Method == "DELETE" || strings.HasSuffix(q.Path, "/kill") {
				return fmt.Errorf("%s: a cleanup mutation followed the substituted authority: %s %s", backend, q.Method, q.Path)
			}
		}
	}
	return nil
}

// execNoLateLaunch holds the observation after both attempts: the fixture
// stands ready to answer any late engine request, and none may arrive, so a
// reply the product abandoned cannot resume the launch it refused.
func (w *world) execNoLateLaunch() error {
	if !w.exec.substituted {
		return fmt.Errorf("the fixture's substitution barrier never fired")
	}
	w.session.api.mu.Lock()
	before := len(w.session.api.requests)
	w.session.api.mu.Unlock()
	timer := time.NewTimer(200 * time.Millisecond)
	defer timer.Stop()
	<-timer.C
	w.session.api.mu.Lock()
	after := len(w.session.api.requests)
	w.session.api.mu.Unlock()
	if after != before {
		return fmt.Errorf("a late engine request resumed the refused launch")
	}
	return nil
}
