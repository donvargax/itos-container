package features

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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
	sc.Step(`^a live exec fixture whose primary stays running and whose exec exits (\d+)$`, func(exit int) error {
		if err := w.execFixture(""); err != nil {
			return err
		}
		w.exec.execExit = exit
		w.exec.execStdout, w.exec.execStderr = "slice-38-exec-stdout-tail\n", "slice-38-exec-stderr-tail\n"
		w.exec.stdin = "slice-38 finite stdin\n"
		return nil
	})
	sc.Step(`^the exec argument array is JSON$`, func(doc *godog.DocString) error {
		var argv []string
		if err := json.Unmarshal([]byte(doc.Content), &argv); err != nil {
			return err
		}
		if len(argv) == 0 {
			return fmt.Errorf("the exec argument array is empty")
		}
		w.exec.argv = argv
		return nil
	})
	sc.Step(`^the exec CLI sends that array and finite stdin through both local backends$`, func() error {
		return w.execEachBackends(nil)
	})
	sc.Step(`^the recorded exec argv and separate output bytes match exactly and exec exits (\d+)$`, w.execRecordedStreams)
	sc.Step(`^exec starts exactly once without a shell or added flags and no host helper marker exists$`, w.execStartsOnce)
	sc.Step(`^the original primary remains running with its unchanged grant and unknown primary exit$`, w.execPrimaryUntouched)
	sc.Step(`^exec help explains literal argv and that cancellation or disconnect stops the whole session$`, w.execHelpText)
	sc.Step(`^a live exec fixture with original frozen settings and "([^"]*)"$`, func(change string) error { return w.execSettingsFixture(change) })
	sc.Step(`^the exec CLI requests a settings probe through both local backends$`, func() error {
		return w.execEachBackends(nil)
	})
	sc.Step(`^exec succeeds with only the original UID GID workspace environment mounts and network restrictions$`, w.execFrozenSettingsUsed)
	sc.Step(`^no exec operation reads current approval or credential bytes, acquires images or creates session resources$`, w.execNoCurrentInputs)
	sc.Step(`^the original grant, primary result and unrelated resources are unchanged$`, w.execGrantUnchanged)
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

// execCreateRequest is the recording of the one exec create a successful
// probe owes, from the backend that made it.
func (b execBackend) createRequest() (sessionRequest, bool) {
	for _, q := range b.execRequests {
		if q.Method == "POST" && strings.HasPrefix(q.Path, "/containers/") && strings.HasSuffix(q.Path, "/exec") {
			return q, true
		}
	}
	return sessionRequest{}, false
}

func (b execBackend) startedRequests() int {
	count := 0
	for _, q := range b.execRequests {
		if q.Method == "POST" && strings.HasPrefix(q.Path, "/exec/") && strings.HasSuffix(q.Path, "/start") {
			count++
		}
	}
	return count
}

func (w *world) execRecordedStreams(exit int) error {
	for _, backend := range []string{"memory", "disk"} {
		b, ok := w.exec.results[backend]
		if !ok {
			return fmt.Errorf("no %s backend result was recorded", backend)
		}
		if b.missing {
			return fmt.Errorf("%s backend published no exec behaviour evidence\n%s", backend, w.execReport(backend))
		}
		r := b.result()
		if r.exit != exit {
			return fmt.Errorf("%s: exec exited %d, not %d\n%s", backend, r.exit, exit, w.execReport(backend))
		}
		if r.stdout != w.exec.execStdout || r.stderr != w.exec.execStderr {
			return fmt.Errorf("%s: separate output bytes did not arrive intact\n%s", backend, w.execReport(backend))
		}
		if err := w.execRecordedArgv(backend, b); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) execRecordedArgv(backend string, b execBackend) error {
	if backend == "disk" {
		q, found := b.createRequest()
		if !found {
			return fmt.Errorf("%s: no exec create was recorded", backend)
		}
		argv := make([]any, 0, len(w.exec.argv))
		for _, arg := range w.exec.argv {
			argv = append(argv, arg)
		}
		if !reflect.DeepEqual(q.Body["Cmd"], argv) {
			return fmt.Errorf("%s: exec argv was not the literal array\n%s", backend, w.execReport(backend))
		}
		if q.Body["AttachStdin"] != true || q.Body["Tty"] != false {
			return fmt.Errorf("%s: exec create lacked redirected stream attachment", backend)
		}
		return nil
	}
	if !slices.Equal(b.report.Argv, w.exec.argv) {
		return fmt.Errorf("%s: exec argv was not the literal array\n%s", backend, w.execReport(backend))
	}
	return nil
}

func (w *world) execStartsOnce() error {
	for _, backend := range []string{"memory", "disk"} {
		b := w.exec.results[backend]
		if b.missing {
			return fmt.Errorf("%s backend published no exec behaviour evidence", backend)
		}
		if b.report.StartedExecs != 1 {
			return fmt.Errorf("%s: exec started %d times, not once", backend, b.report.StartedExecs)
		}
		if backend == "disk" {
			if _, found := b.createRequest(); !found {
				return fmt.Errorf("%s: no exec create was recorded", backend)
			}
			if count := b.startedRequests(); count != 1 {
				return fmt.Errorf("%s: %d exec start requests were recorded", backend, count)
			}
		}
	}
	for _, root := range []string{w.doctor.checkout, w.dir, w.doctor.host} {
		if _, err := os.Stat(filepath.Join(root, "slice-38-host-marker")); !os.IsNotExist(err) {
			return fmt.Errorf("a host helper marker exists under %s", root)
		}
	}
	return nil
}

func (w *world) execPrimaryUntouched() error {
	worker := w.session.api.local[w.exec.worker]
	if worker == nil || object(worker["State"])["Running"] != true {
		return fmt.Errorf("the original primary is no longer running")
	}
	data, err := os.ReadFile(filepath.Join(w.execSessionDir(), "record.json"))
	if err != nil {
		return err
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		return err
	}
	if record["state"] != "running" || record["workload_exit"] != nil {
		return fmt.Errorf("the primary's durable result changed: %s", data)
	}
	return w.execGrantUnchanged()
}

func (w *world) execGrantUnchanged() error {
	after, err := w.sessionProtected()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(w.exec.immutable, after) {
		return fmt.Errorf("an exec operation changed the original grant or unrelated state")
	}
	resourcesAfter := map[string]string{}
	for id, c := range w.session.api.local {
		resourcesAfter[id] = string(approvalJSON(c))
	}
	if !reflect.DeepEqual(w.exec.resourcesBefore, resourcesAfter) {
		return fmt.Errorf("an exec operation changed an engine resource")
	}
	return nil
}

func (w *world) execHelpText() error {
	if err := w.runEnv(w.doctor.checkout, w.env(), w.bin, "exec", "--help"); err != nil {
		return err
	}
	if w.exit != 0 {
		return fmt.Errorf("exec help failed\n%s", w.report())
	}
	for _, word := range []string{"argv", "cancel", "disconnect", "session"} {
		if !strings.Contains(w.stdout, word) {
			return fmt.Errorf("exec help omits %s\n%s", word, w.report())
		}
	}
	return nil
}

func (w *world) execFrozenSettingsUsed() error {
	for _, backend := range []string{"memory", "disk"} {
		b, ok := w.exec.results[backend]
		if !ok {
			return fmt.Errorf("no %s backend result was recorded", backend)
		}
		if b.missing {
			return fmt.Errorf("%s backend published no exec behaviour evidence\n%s", backend, w.execReport(backend))
		}
		if b.result().exit != 0 {
			return fmt.Errorf("%s: the settings probe did not succeed\n%s", backend, w.execReport(backend))
		}
		if backend == "disk" {
			q, found := b.createRequest()
			if !found {
				return fmt.Errorf("%s: no exec create was recorded", backend)
			}
			if q.Body["User"] != w.exec.frozenUser || q.Body["WorkingDir"] != w.exec.frozenWorkspace {
				return fmt.Errorf("%s: exec did not use the frozen identity and workspace\n%s", backend, w.execReport(backend))
			}
			env := make([]string, 0, len(q.Body["Env"].([]any)))
			for _, value := range q.Body["Env"].([]any) {
				env = append(env, fmt.Sprint(value))
			}
			if !execEnvEqual(env, w.exec.frozenEnv) {
				return fmt.Errorf("%s: exec did not use exactly the frozen environment\n%s", backend, w.execReport(backend))
			}
			for _, key := range []string{"Privileged", "HostConfig", "NetworkMode", "Mounts", "DetachKeys"} {
				if _, present := q.Body[key]; present {
					return fmt.Errorf("%s: exec create carried %s", backend, key)
				}
			}
			continue
		}
		if b.report.User != w.exec.frozenUser || b.report.WorkingDir != w.exec.frozenWorkspace {
			return fmt.Errorf("%s: exec did not use the frozen identity and workspace\n%s", backend, w.execReport(backend))
		}
		if !execEnvEqual(b.report.Env, w.exec.frozenEnv) {
			return fmt.Errorf("%s: exec did not use exactly the frozen environment\n%s", backend, w.execReport(backend))
		}
	}
	return nil
}

func (w *world) execNoCurrentInputs() error {
	if w.exec.inputWatch == nil {
		return fmt.Errorf("the fixture watches no current inputs")
	}
	reads, err := w.exec.inputWatch.finish()
	if err != nil {
		return err
	}
	if len(reads) != 0 {
		return fmt.Errorf("an exec operation read current approval or credential bytes: %v", reads)
	}
	for _, q := range w.exec.results["disk"].execRequests {
		if strings.HasPrefix(q.Path, "/images/") || q.Path == "/containers/create" || strings.HasPrefix(q.Path, "/volumes") {
			return fmt.Errorf("an exec operation acquired an image or created a session resource: %s %s", q.Method, q.Path)
		}
	}
	return nil
}

func execEnvEqual(observed []string, frozen []string) bool {
	if len(observed) != len(frozen) {
		return false
	}
	observed, frozen = slices.Clone(observed), slices.Clone(frozen)
	slices.Sort(observed)
	slices.Sort(frozen)
	return slices.Equal(observed, frozen)
}
