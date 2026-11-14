package features

import (
	"debug/elf"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
)

func (w *world) egressOfflineWorker() error {
	a := w.doctor.engine.offline
	if a == nil {
		return fmt.Errorf("missing local foreground fixture")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.created) != 1 {
		return fmt.Errorf("offline run created %d workers", len(a.created))
	}
	for _, resource := range a.created {
		body := object(resource["Config"])
		if object(body["HostConfig"])["NetworkMode"] != "none" || len(object(object(body["NetworkingConfig"])["EndpointsConfig"])) != 0 {
			return fmt.Errorf("offline worker has a network")
		}
		for _, raw := range anyList(body["Env"]) {
			if key, _, _ := strings.Cut(strings.ToUpper(fmt.Sprint(raw)), "="); key == "HTTP_PROXY" || key == "HTTPS_PROXY" || key == "NO_PROXY" || key == "ALL_PROXY" {
				return fmt.Errorf("offline worker has the proxy variable %v", raw)
			}
		}
	}
	creates := 0
	for _, r := range w.doctor.engine.requests() {
		path := doctorRequestPath(r.Path)
		if strings.HasPrefix(path, "/networks") {
			return fmt.Errorf("offline run asked for a network: %+v", r)
		}
		if path == "/containers/create" {
			creates++
		}
	}
	if creates != 1 {
		return fmt.Errorf("offline run sent %d container creates", creates)
	}
	return nil
}

// The controller ran from inside the checkout against a daemon older than the
// isolated bridge, so a run that had consulted either egress prerequisite, or
// the proxy image, would have refused or left the request in the journal.
func (w *world) egressNoProxyInspection() error {
	for _, r := range w.doctor.engine.requests() {
		path := doctorRequestPath(r.Path)
		if strings.Contains(path, "busybox") || strings.Contains(path, strings.TrimPrefix(egressProxyConfigID, "sha256:")) {
			return fmt.Errorf("offline run inspected the proxy image: %+v", r)
		}
	}
	if strings.Contains(w.stdout+w.stderr, "doctor.egress-") {
		return fmt.Errorf("offline run consulted an egress prerequisite\n%s", w.report())
	}
	controller := w.runFixture().controller
	if inside, err := filepath.Rel(w.doctor.checkout, controller); controller == "" || err != nil || strings.HasPrefix(inside, "..") {
		return fmt.Errorf("the offline control did not run from inside the checkout")
	}
	return nil
}

func (w *world) egressDoctorResults() (plain, structured egressResult, err error) {
	results := w.egressFixture().results
	if len(results) != 2 || results[0].command != "doctor" || results[1].command != "doctor --json" {
		return plain, structured, fmt.Errorf("missing plain and JSON doctor results")
	}
	return results[0], results[1], nil
}

// withResult lets the existing single-result assertions read one captured invocation.
func (w *world) withResult(r egressResult, check func() error) error {
	stdout, stderr, exit := w.stdout, w.stderr, w.exit
	defer func() { w.stdout, w.stderr, w.exit = stdout, stderr, exit }()
	w.stdout, w.stderr, w.exit = r.result.stdout, r.result.stderr, r.result.exit
	return check()
}

func (w *world) egressDoctorReady() error {
	plain, structured, err := w.egressDoctorResults()
	if err != nil {
		return err
	}
	if err = w.withResult(structured, w.doctorReady); err != nil {
		return err
	}
	if plain.result.exit != 0 || !strings.Contains(plain.result.stdout, "inspection ready") {
		return fmt.Errorf("plain doctor is not ready\n%s%s", plain.result.stdout, plain.result.stderr)
	}
	for _, r := range []egressResult{plain, structured} {
		seen := map[string]bool{}
		for _, request := range r.journal {
			if request.Method == http.MethodGet && request.Query == "" {
				seen[doctorRequestPath(request.Path)] = true
			}
		}
		for _, image := range []string{w.doctor.image, busyboxFixturePin} {
			if !seen["/images/"+image+"/json"] {
				return fmt.Errorf("%s reported ready without inspecting %s", r.command, image)
			}
		}
	}
	// Ready also claims the controller's own program can be delivered to the
	// proxy. Read the built executable independently of the product.
	return egressStaticOutside(w.bin, w.doctor.checkout)
}

// egressStaticOutside is the harness's own reading of an executable: a regular
// static Linux amd64 ELF whose resolved path lies outside root.
func egressStaticOutside(path, root string) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if inside, err := filepath.Rel(root, resolved); err != nil || !strings.HasPrefix(inside, "..") {
		return fmt.Errorf("the scenario's product lies inside the checkout")
	}
	file, err := elf.Open(resolved)
	if err != nil {
		return fmt.Errorf("the scenario's product is not a readable ELF: %w", err)
	}
	defer func() { _ = file.Close() }()
	if file.Class != elf.ELFCLASS64 || file.Machine != elf.EM_X86_64 {
		return fmt.Errorf("the scenario's product is not a 64-bit amd64 ELF")
	}
	for _, program := range file.Progs {
		if program.Type == elf.PT_INTERP || program.Type == elf.PT_DYNAMIC {
			return fmt.Errorf("the scenario's product is dynamically linked; doctor cannot truthfully report it ready (the harness must build what the release ships)")
		}
	}
	return nil
}

var egressNoInterception = regexp.MustCompile(`(?i)\b(no|without)\b[^.\n]*\binterception\b`)

func (w *world) egressDoctorPlain() error {
	plain, _, err := w.egressDoctorResults()
	if err != nil {
		return err
	}
	out := plain.result.stdout
	if err = w.approvalListedNames(out); err != nil {
		return err
	}
	for _, text := range []string{"allowlist-v1", busyboxFixturePin, "CONNECT", "443", "isolated internal network"} {
		if !strings.Contains(out, text) {
			return fmt.Errorf("plain doctor does not name %q\n%s", text, out)
		}
	}
	if !egressNoInterception.MatchString(out) {
		return fmt.Errorf("plain doctor does not say the proxy works without interception\n%s", out)
	}
	if strings.Contains(out, "Run cannot start this profile yet") {
		return fmt.Errorf("plain doctor still says run cannot start the profile")
	}
	return nil
}

func (w *world) egressDoctorCandidate() error {
	plain, structured, err := w.egressDoctorResults()
	if err != nil {
		return err
	}
	for _, r := range []egressResult{plain, structured} {
		if err := w.withResult(r, func() error {
			if err := w.approvalCandidate(); err != nil {
				return err
			}
			return w.approvalStatus("missing")
		}); err != nil {
			return fmt.Errorf("%s: %w", r.command, err)
		}
	}
	if w.approval.settings["profile"] != "allowlist-v1" {
		return fmt.Errorf("the oracle is not an allowlist candidate")
	}
	return nil
}

func (w *world) egressCopyUnchanged() error {
	e := w.egressFixture()
	if e.copyBefore == nil {
		return fmt.Errorf("no copied executable was recorded")
	}
	after, err := egressTree(w.dir)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(after, e.copyBefore) {
		for path, value := range after {
			if e.copyBefore[path] != value {
				return fmt.Errorf("changed or new entry %s", path)
			}
		}
		return fmt.Errorf("an entry was removed")
	}
	return w.approvalRecordUnchanged()
}

func (w *world) egressBoundaryRefusal(rule string, exit int) error {
	a, err := w.egressAPI()
	if err != nil {
		return err
	}
	if w.exit != exit || w.stdout != "" || !strings.Contains(w.stderr, "itos-container: "+rule) {
		return fmt.Errorf("run did not report %s with exit %d\n%s", rule, exit, w.report())
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	// The refusal must follow the daemon's own answer, not a local shortcut.
	defect := a.defect
	role := "internal"
	if strings.HasPrefix(defect, "proxy ") {
		role = "proxy"
	} else if strings.HasPrefix(defect, "worker ") {
		role = "worker"
	}
	kind := "inspect"
	if role == "internal" {
		kind = "network-inspect"
	}
	if egressIndex(w.egressRunEvents(a), 0, kind, role, nil) < 0 {
		return fmt.Errorf("run refused before observing the %s", role)
	}
	return nil
}

func (w *world) egressNoFallback() error {
	a, err := w.egressAPI()
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.starts["worker"] != 0 {
		return fmt.Errorf("the worker was started")
	}
	if a.starts["proxy"] > 1 {
		return fmt.Errorf("the proxy was started again")
	}
	creates := map[string]int{}
	for _, event := range w.egressRunEvents(a) {
		switch {
		case event.Kind == "network-create" || event.Kind == "container-create":
			creates[event.Role]++
			if creates[event.Role] > 1 {
				return fmt.Errorf("a second %s was requested", event.Role)
			}
		case strings.HasPrefix(event.Kind, "forbidden-") || strings.HasPrefix(event.Kind, "network-post") || strings.HasPrefix(event.Kind, "network-put"):
			return fmt.Errorf("run asked the engine for %s on the %s", event.Kind, event.Role)
		}
	}
	for _, r := range w.doctor.engine.requests() {
		path := doctorRequestPath(r.Path)
		if strings.HasSuffix(path, "/exec") || strings.HasSuffix(path, "/restart") || strings.HasSuffix(path, "/connect") || strings.HasSuffix(path, "/update") {
			return fmt.Errorf("run asked the engine for %s", path)
		}
	}
	return nil
}

func (w *world) egressRecordedCleanup() error {
	a, err := w.egressAPI()
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	// Every removal was checked against the protected record when it arrived.
	if a.problem != nil {
		return a.problem
	}
	if len(a.ids) == 0 {
		return fmt.Errorf("run created no resource to clean")
	}
	if len(a.containers) != 0 || len(a.networks) != 0 || len(a.volumes) != 0 {
		return fmt.Errorf("created resources remain: %d containers, %d networks, %d volumes", len(a.containers), len(a.networks), len(a.volumes))
	}
	removed := map[string]bool{}
	for _, role := range a.removed {
		removed[role] = true
	}
	for role := range a.ids {
		if !removed[role] {
			return fmt.Errorf("the created %s was not removed", role)
		}
	}
	record, err := a.intent(a.token())
	if err != nil {
		return err
	}
	for role := range a.ids {
		if role == "worker" {
			continue
		}
		if stage, id := egressStage(record, role); stage != "absent" || id != a.ids[role] {
			return fmt.Errorf("protected record does not confirm the %s absent", role)
		}
	}
	if object(record["anchor"])["stage"] != "absent" {
		return fmt.Errorf("protected record keeps the anchor")
	}
	return nil
}

func (w *world) egressRunnableReview() error {
	text, err := w.approvalBeforePrompt()
	if err != nil {
		return err
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	rest := text
	for _, want := range append([]string{"Effective settings:", "allowlist-v1:", "This profile grants network."}, egressLimitations...) {
		_, after, ok := strings.Cut(rest, want)
		if !ok {
			return fmt.Errorf("review lacks, in order, %q\n%s", want, w.report())
		}
		rest = after
	}
	if strings.Contains(text, "Run cannot start this profile yet") {
		return fmt.Errorf("review still says run cannot start the profile it grants\n%s", w.report())
	}
	return w.approvalTextSettings(text)
}
