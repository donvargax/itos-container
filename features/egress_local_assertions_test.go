package features

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// egressTree is doctorTree with each regular file's bytes replaced by their
// digest, for scenarios whose checkout holds a copy of the built executable.
func egressTree(root string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data := ""
		if info.Mode().IsRegular() {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(content)
			data = hex.EncodeToString(sum[:])
		} else if info.Mode()&os.ModeSymlink != 0 {
			if data, err = os.Readlink(path); err != nil {
				return err
			}
		}
		owner := "unavailable"
		if stat := reflect.Indirect(reflect.ValueOf(info.Sys())); stat.IsValid() && stat.Kind() == reflect.Struct {
			if uid := stat.FieldByName("Uid"); uid.IsValid() {
				owner = fmt.Sprint(uid.Interface())
			}
		}
		files[path] = fmt.Sprintf("%v:%s:%s", info.Mode(), owner, data)
		return nil
	})
	return files, err
}

func egressProblems(stdout string) ([]map[string]any, bool) {
	var object map[string]any
	if !strings.HasPrefix(strings.TrimSpace(stdout), "{") || json.Unmarshal([]byte(stdout), &object) != nil {
		return nil, false
	}
	var problems []map[string]any
	for _, raw := range anyList(object["problems"]) {
		if p, ok := raw.(map[string]any); ok {
			problems = append(problems, p)
		}
	}
	return problems, true
}

// egressProblem requires the typed prerequisite in whatever form the command
// reports problems: a schema-1 JSON problem with its flat subject, or a line on
// stderr. Plain doctor prints a problem's message and fix without its rule, so
// there the subject and the exit carry the assertion.
func egressProblem(r egressResult, rule string, exit int, key, subject string) error {
	if r.result.exit != exit {
		return fmt.Errorf("exit %d, want %d for %s", r.result.exit, exit, rule)
	}
	if strings.Contains(r.result.stdout+r.result.stderr, "y/N") {
		return fmt.Errorf("a prerequisite refusal prompted for consent")
	}
	if problems, ok := egressProblems(r.result.stdout); ok {
		for _, p := range problems {
			if p["rule"] != rule {
				continue
			}
			message, _ := p["message"].(string)
			fix, _ := p["fix"].(string)
			if p["exit"] != float64(exit) || strings.TrimSpace(message) == "" || strings.TrimSpace(fix) == "" {
				return fmt.Errorf("incomplete problem %v", p)
			}
			if key != "" && p[key] != subject {
				return fmt.Errorf("problem %s is %v, want %q", key, p[key], subject)
			}
			return nil
		}
		return fmt.Errorf("no JSON problem with rule %s", rule)
	}
	if subject != "" && !strings.Contains(r.result.stderr, subject) {
		return fmt.Errorf("plain refusal does not name its subject %q", subject)
	}
	// Plain doctor ends with its hint for scripts; every other plain refusal
	// starts its line with the rule.
	if !strings.Contains(r.result.stderr, "for scripts, use --json") && !strings.Contains(r.result.stderr, "itos-container: "+rule+" ") {
		return fmt.Errorf("plain refusal does not name %s", rule)
	}
	if !strings.Contains(r.result.stderr, "itos-container: ") {
		return fmt.Errorf("no plain diagnostic")
	}
	return nil
}

// egressFixes are the fix texts the invocation reported for rule, or every
// plain line's text after its last "; " where the output is not JSON.
func egressFixes(r egressResult, rule string) []string {
	var fixes []string
	if problems, ok := egressProblems(r.result.stdout); ok {
		for _, p := range problems {
			if fix, _ := p["fix"].(string); rule == "" || p["rule"] == rule {
				fixes = append(fixes, fix)
			}
		}
		return fixes
	}
	for _, line := range strings.Split(r.result.stderr, "\n") {
		if at := strings.LastIndex(line, "; "); at >= 0 && strings.HasPrefix(line, "itos-container: ") {
			fixes = append(fixes, line[at+2:])
		}
	}
	return fixes
}

func egressExactFix(r egressResult, want string) error {
	for _, fix := range egressFixes(r, "") {
		if fix == want {
			return nil
		}
	}
	return fmt.Errorf("no fix is exactly %q", want)
}

func egressGuidance(r egressResult, rule string, words ...string) error {
	for _, fix := range egressFixes(r, rule) {
		missing := ""
		for _, word := range words {
			if !strings.Contains(strings.ToLower(fix), strings.ToLower(word)) {
				missing = word
				break
			}
		}
		if missing == "" {
			return nil
		}
	}
	return fmt.Errorf("no fix for %s names %v", rule, words)
}

func (w *world) egressAPI() (*localEgressAPI, error) {
	if w.doctor == nil || w.doctor.engine == nil || w.doctor.engine.egress == nil {
		return nil, fmt.Errorf("missing egress-capable fixture")
	}
	return w.doctor.engine.egress, nil
}

// egressRunEvents are the fixture's events for the scenario's last run, in order.
func (w *world) egressRunEvents(a *localEgressAPI) []egressEvent {
	pid := 0
	if w.run != nil && len(w.run.results) != 0 {
		pid = w.run.results[len(w.run.results)-1].pid
	}
	if e := w.egressFixture(); len(e.results) != 0 {
		pid = e.results[len(e.results)-1].pid
	}
	var events []egressEvent
	for _, event := range a.events {
		if event.PID == pid {
			events = append(events, event)
		}
	}
	return events
}

func (w *world) egressRunCompleted() error {
	a, err := w.egressAPI()
	if err != nil {
		return err
	}
	if w.exit != 0 || w.stdout != "" || w.stderr != "" {
		return fmt.Errorf("allowlist run did not complete quietly with exit zero\n%s", w.report())
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.problem != nil {
		return a.problem
	}
	worker := a.bodies["worker"]
	if worker == nil {
		return fmt.Errorf("no worker was created")
	}
	if !sameJSON(worker["Cmd"], []any{"fixture-workload", "", "two words", "--json"}) || !sameJSON(worker["Entrypoint"], []any{}) {
		return fmt.Errorf("worker argv is %s, not the literal words after --", approvalJSON(worker["Cmd"]))
	}
	if a.starts["worker"] != 1 || a.containers[a.ids["worker"]] != nil {
		return fmt.Errorf("worker was not started once and removed")
	}
	return nil
}

func (w *world) egressProxyCreate() error {
	a, err := w.egressAPI()
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	proxy := a.bodies["proxy"]
	if proxy == nil {
		return fmt.Errorf("no proxy was created")
	}
	want := []any{"/itos-container", "__egress-proxy", "--"}
	for _, name := range w.approval.allow {
		want = append(want, name)
	}
	if !sameJSON(proxy["Cmd"], want) || !sameJSON(proxy["Entrypoint"], []any{}) {
		return fmt.Errorf("proxy argv is %s, want %s", approvalJSON(proxy["Cmd"]), approvalJSON(want))
	}
	if proxy["Image"] != egressProxyConfigID {
		return fmt.Errorf("proxy image is %v, not the inspected proxy config ID", proxy["Image"])
	}
	// The ID must come from this run's own inspection of the pinned reference.
	inspected := false
	for _, event := range w.egressRunEvents(a) {
		if event.Kind == "image" && event.Role == busyboxFixturePin {
			inspected = true
		}
		if event.Kind == "container-create" && event.Role == "proxy" && !inspected {
			return fmt.Errorf("proxy created before this run inspected the pinned reference")
		}
	}
	if !inspected {
		return fmt.Errorf("this run never inspected the pinned proxy reference")
	}
	return nil
}

// The durable form is the implementation's to choose and to record. What a
// scenario can hold is that the ID was already in protected state before the
// first engine create, is still there after cleanup, never entered the canonical
// snapshot, and lets a fresh process reopen the session after the cached image
// behind the same reference has been replaced.
func (w *world) egressEvidenceSurvives() error {
	a, err := w.egressAPI()
	if err != nil {
		return err
	}
	a.mu.Lock()
	before := a.evidence
	a.mu.Unlock()
	if len(before) == 0 {
		return fmt.Errorf("no protected record named the proxy config ID before the first engine create")
	}
	snapshot := w.runSnapshotPath()
	after := egressEvidence(w)
	for path := range before {
		if path == snapshot {
			return fmt.Errorf("the canonical snapshot carries runtime evidence")
		}
		if _, ok := after[path]; !ok {
			return fmt.Errorf("protected record %s no longer names the original proxy config ID", filepath.Base(path))
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			return fmt.Errorf("runtime evidence is not a private regular file")
		}
	}
	replacement := "sha256:" + strings.Repeat("7", 64)
	a.mu.Lock()
	a.proxyImage["Id"] = replacement
	a.mu.Unlock()
	saved := []any{w.stdout, w.stderr, w.exit}
	defer func() { w.stdout, w.stderr, w.exit = saved[0].(string), saved[1].(string), saved[2].(int) }()
	w.deadline = 0
	if err = w.runEnv(w.doctor.cwd, w.env(), w.bin, "status", "--json"); err != nil {
		return err
	}
	var report struct {
		OK       bool
		Sessions []map[string]any
	}
	if json.Unmarshal([]byte(w.stdout), &report) != nil || w.exit != 0 || !report.OK || len(report.Sessions) != 1 || report.Sessions[0]["cleanup"] != "complete" {
		return fmt.Errorf("a fresh process could not reopen the completed session\n%s", w.report())
	}
	if strings.Contains(w.stdout+w.stderr, replacement) {
		return fmt.Errorf("reopening consulted the replacement image")
	}
	for path := range egressEvidence(w) {
		if _, ok := before[path]; !ok {
			return fmt.Errorf("reopening wrote new runtime evidence")
		}
	}
	return nil
}

func egressIndex(events []egressEvent, from int, kind, role string, match func(egressEvent) bool) int {
	for i := from; i < len(events); i++ {
		if events[i].Kind == kind && events[i].Role == role && (match == nil || match(events[i])) {
			return i
		}
	}
	return -1
}

func egressFollows(e egressEvent) bool {
	return strings.Contains(e.Query, "follow=1") || strings.Contains(e.Query, "follow=true")
}

func (w *world) egressJournal() error {
	a, err := w.egressAPI()
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.problem != nil {
		return a.problem
	}
	events := w.egressRunEvents(a)
	at := 0
	step := func(kind, role string, match func(egressEvent) bool) error {
		next := egressIndex(events, at, kind, role, match)
		if next < 0 {
			return fmt.Errorf("journal lacks %s of %s after request %d: %+v", kind, role, at, events)
		}
		at = next + 1
		return nil
	}
	for _, want := range []struct {
		kind, role string
		match      func(egressEvent) bool
	}{
		{"volume-create", "anchor", nil},
		{"network-create", "internal", nil}, {"network-inspect", "internal", nil},
		{"network-create", "egress", nil}, {"network-inspect", "egress", nil},
		{"container-create", "proxy", nil}, {"inspect", "proxy", nil}, {"start", "proxy", nil},
		{"logs", "proxy", egressFollows}, {"inspect", "proxy", nil},
		{"container-create", "worker", nil}, {"inspect", "worker", nil}, {"attach", "worker", nil},
		{"network-inspect", "internal", nil}, {"network-inspect", "egress", nil},
		{"start", "worker", nil}, {"wait", "worker", nil},
		{"remove", "worker", nil}, {"kill", "proxy", nil},
		{"logs", "proxy", func(e egressEvent) bool { return !egressFollows(e) }},
		{"remove", "proxy", nil}, {"network-remove", "egress", nil}, {"network-remove", "internal", nil},
		{"volume-remove", "anchor", nil},
	} {
		if err := step(want.kind, want.role, want.match); err != nil {
			return err
		}
	}
	if !reflect.DeepEqual(a.removed, []string{"worker", "proxy", "egress", "internal", "anchor"}) {
		return fmt.Errorf("cleanup order is %v", a.removed)
	}
	if len(a.containers) != 0 || len(a.networks) != 0 || len(a.volumes) != 0 {
		return fmt.Errorf("owned resources remain after the run")
	}
	if a.starts["proxy"] != 1 || a.starts["worker"] != 1 {
		return fmt.Errorf("proxy or worker was not started exactly once")
	}
	record, err := a.intent(a.token())
	if err != nil {
		return err
	}
	for _, role := range []string{"internal", "egress", "proxy"} {
		if stage, id := egressStage(record, role); stage != "absent" || id != a.ids[role] {
			return fmt.Errorf("protected record does not confirm the %s absent", role)
		}
	}
	if record["stage"] != "absent" || record["container_id"] != a.ids["worker"] || object(record["anchor"])["stage"] != "absent" {
		return fmt.Errorf("protected record does not confirm complete cleanup")
	}
	return w.egressLaunchShape(a)
}

// egressLaunchShape holds T-40's exact launch at the request the product sent.
func (w *world) egressLaunchShape(a *localEgressAPI) error {
	token := a.token()
	internal, egress := "itos-container-internal-"+token, "itos-container-egress-"+token
	owner := w.approval.uid + ":" + w.approval.gid
	in, out := a.bodies["internal"], a.bodies["egress"]
	if in["Internal"] != true || in["EnableIPv6"] != false || in["Driver"] != "bridge" || object(in["Options"])["com.docker.network.bridge.gateway_mode_ipv4"] != "isolated" {
		return fmt.Errorf("internal network is not an isolated internal IPv4 bridge")
	}
	if out["Internal"] == true || out["EnableIPv6"] != false || out["Driver"] != "bridge" {
		return fmt.Errorf("egress network is not a plain IPv4 bridge")
	}
	proxy, worker := a.bodies["proxy"], a.bodies["worker"]
	ph, wh := object(proxy["HostConfig"]), object(worker["HostConfig"])
	mounts := anyList(ph["Mounts"])
	if len(mounts) != 1 {
		return fmt.Errorf("proxy has %d mounts", len(mounts))
	}
	bind := object(mounts[0])
	source, _ := bind["Source"].(string)
	inside, err := filepath.Rel(w.doctor.checkout, source)
	if err != nil || bind["Type"] != "bind" || bind["Target"] != "/itos-container" || bind["ReadOnly"] != true || !filepath.IsAbs(source) || !strings.HasPrefix(inside, "..") {
		return fmt.Errorf("proxy executable is not one read-only bind from outside the checkout")
	}
	if proxy["User"] != owner || ph["NetworkMode"] != egress || ph["ReadonlyRootfs"] != true || !sameJSON(ph["CapDrop"], []any{"ALL"}) ||
		object(ph["Sysctls"])["net.ipv4.ip_forward"] != "0" || len(object(ph["PortBindings"])) != 0 || ph["PublishAllPorts"] == true || object(ph["RestartPolicy"])["Name"] != "no" {
		return fmt.Errorf("proxy lost a specified restriction")
	}
	if !sameJSON(egressEndpointNames(proxy), []string{egress, internal}) || !sameJSON(egressEndpointNames(worker), []string{internal}) {
		return fmt.Errorf("proxy or worker joins other networks than specified")
	}
	if worker["User"] != owner || wh["NetworkMode"] != internal || len(object(wh["PortBindings"])) != 0 || wh["PublishAllPorts"] == true {
		return fmt.Errorf("worker is not on the internal network alone")
	}
	env := map[string]string{}
	for _, raw := range anyList(worker["Env"]) {
		key, value, _ := strings.Cut(fmt.Sprint(raw), "=")
		env[key] = value
	}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		if env[key] != "http://"+egressInternalAddress+":3128" {
			return fmt.Errorf("worker %s is %q", key, env[key])
		}
	}
	if env["NO_PROXY"] != "localhost,127.0.0.1,::1" || env["no_proxy"] != "localhost,127.0.0.1,::1" {
		return fmt.Errorf("worker NO_PROXY values differ")
	}
	return nil
}

func egressEndpointNames(body map[string]any) []string {
	names := []string{}
	for name := range object(object(body["NetworkingConfig"])["EndpointsConfig"]) {
		names = append(names, name)
	}
	for i := range names {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	return names
}
