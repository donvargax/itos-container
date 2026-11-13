package features

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strings"
)

func (w *world) doctorJSON() (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(w.stdout))
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("doctor must emit one JSON object: %w\n%s", err, w.report())
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("doctor emitted trailing JSON/output\n%s", w.report())
	}
	if value["schema"] != float64(1) {
		return nil, fmt.Errorf("doctor schema is not 1\n%s", w.report())
	}
	if _, exists := value["run_ready"]; exists {
		return nil, fmt.Errorf("inspection claims run readiness")
	}
	if doctorClaimsApproval(value) {
		return nil, fmt.Errorf("inspection claims approval or runtime isolation")
	}
	return value, nil
}
func doctorClaimsApproval(value any) bool {
	switch x := value.(type) {
	case map[string]any:
		for key, child := range x {
			if key == "run_ready" || ((key == "approved" || key == "isolated" || key == "runtime_isolation") && child == true) {
				return true
			}
			if doctorClaimsApproval(child) {
				return true
			}
		}
	case []any:
		for _, child := range x {
			if doctorClaimsApproval(child) {
				return true
			}
		}
	}
	return false
}
func (w *world) doctorReady() error {
	obj, err := w.doctorJSON()
	if err != nil {
		return err
	}
	inspection, _ := obj["inspection"].(map[string]any)
	problems, ok := obj["problems"].([]any)
	if obj["ok"] != true || inspection["ready"] != true || !ok || len(problems) != 0 || w.exit != 0 {
		return fmt.Errorf("expected schema-1 successful ready inspection, no problems and exit 0\n%s", w.report())
	}
	return nil
}
func (w *world) doctorNotReady() error {
	obj, err := w.doctorJSON()
	if err != nil {
		return err
	}
	inspection, _ := obj["inspection"].(map[string]any)
	if obj["ok"] != false || inspection["ready"] != false {
		return fmt.Errorf("expected ok=false and inspection.ready=false\n%s", w.report())
	}
	return nil
}
func (w *world) doctorProblem(rule string, exit int) error {
	if err := w.doctorNotReady(); err != nil {
		return err
	}
	if err := w.exitsWith(exit); err != nil {
		return err
	}
	obj, err := w.doctorJSON()
	if err != nil {
		return err
	}
	problems, ok := obj["problems"].([]any)
	if !ok || len(problems) == 0 {
		return fmt.Errorf("missing problems\n%s", w.report())
	}
	found := false
	for _, value := range problems {
		p, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("problem is not an object")
		}
		r, _ := p["rule"].(string)
		message, _ := p["message"].(string)
		fix, _ := p["fix"].(string)
		if !regexp.MustCompile(`^[a-z]+(?:\.[a-z]+)+$`).MatchString(r) || strings.TrimSpace(message) == "" || strings.TrimSpace(fix) == "" || p["exit"] == nil {
			return fmt.Errorf("incomplete doctor problem\n%s", w.report())
		}
		subject := "field"
		if strings.HasPrefix(r, "doctor.checkout.") {
			subject = "path"
		}
		if strings.HasPrefix(r, "doctor.image.") {
			subject = "image"
		}
		if strings.HasPrefix(r, "manifest.") {
			subject = "path"
		}
		if _, ok := p[subject].(string); !ok {
			return fmt.Errorf("problem lacks flat %s subject\n%s", subject, w.report())
		}
		if r == rule && p["exit"] == float64(exit) {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("missing %s problem with exit %d\n%s", rule, exit, w.report())
	}
	return nil
}
func (w *world) doctorOutcome(outcome string, exit int) error {
	if outcome == "approval.identity.mismatch" {
		return w.approvalProblem(outcome, exit, "")
	}
	if outcome == "ready" {
		if err := w.doctorReady(); err != nil {
			return err
		}
		return w.exitsWith(exit)
	}
	return w.doctorProblem(outcome, exit)
}
func (w *world) doctorIdentifies() error {
	if err := w.doctorReady(); err != nil {
		return err
	}
	if len(w.doctor.results) == 0 {
		return fmt.Errorf("no doctor run")
	}
	result := w.doctor.results[len(w.doctor.results)-1]
	obj, err := w.doctorJSON()
	if err != nil {
		return err
	}
	inspection := obj["inspection"].(map[string]any)
	expected := map[string]any{
		"checkout": result.checkout,
		"engine":   map[string]any{"endpoint": result.endpoint, "id": result.daemon, "api_version": w.doctor.api, "os": "linux", "architecture": "amd64"},
		"image":    map[string]any{"reference": w.doctor.image, "id": w.doctor.imageID, "os": "linux", "architecture": "amd64"},
	}
	for field, value := range expected {
		if !reflect.DeepEqual(inspection[field], value) {
			return fmt.Errorf("inspection.%s = %#v, want independent observation %#v\n%s", field, inspection[field], value, w.report())
		}
	}
	return nil
}
func (w *world) doctorEffective() error {
	obj, err := w.doctorJSON()
	if err != nil {
		return err
	}
	manifest, _ := obj["manifest"].(map[string]any)
	request := w.doctor.manifest
	expected := map[string]any{"name": nil, "image": request["image"], "workspaceFolder": "/workspace", "containerEnv": map[string]any{}, "containerUser": request["containerUser"], "remoteUser": request["remoteUser"]}
	if manifest["valid"] != true || !reflect.DeepEqual(manifest["effective"], expected) {
		return fmt.Errorf("effective manifest does not preserve all six requested/default fields\n%s", w.report())
	}
	return nil
}
func (w *world) doctorMetadataProblem(path string, exit int) error {
	if err := w.doctorProblem("manifest.metadata", exit); err != nil {
		return err
	}
	obj, err := w.doctorJSON()
	if err != nil {
		return err
	}
	manifest, _ := obj["manifest"].(map[string]any)
	metadata, _ := manifest["image_metadata"].(map[string]any)
	_, effective := manifest["effective"]
	if manifest["valid"] != false || effective || metadata["checked"] != true || metadata["present"] != true {
		return fmt.Errorf("metadata refusal lacks checked/present invalid manifest without effective settings\n%s", w.report())
	}
	for _, value := range obj["problems"].([]any) {
		p := value.(map[string]any)
		if p["rule"] == "manifest.metadata" && p["path"] == path && p["image"] == w.doctor.image && p["label"] == "devcontainer.metadata" && p["exit"] == float64(exit) {
			return nil
		}
	}
	return fmt.Errorf("missing inspected image metadata subject at %q\n%s", path, w.report())
}
func (w *world) doctorIndependentProblems() error {
	if err := w.doctorProblem("doctor.image.digest", 1); err != nil {
		return err
	}
	if err := w.doctorProblem("doctor.image.platform", 1); err != nil {
		return err
	}
	obj, err := w.doctorJSON()
	if err != nil {
		return err
	}
	p := obj["problems"].([]any)
	if len(p) != 2 || p[0].(map[string]any)["rule"] != "doctor.image.digest" || p[1].(map[string]any)["rule"] != "doctor.image.platform" {
		return fmt.Errorf("independent problems not in digest/platform order\n%s", w.report())
	}
	return nil
}
func (w *world) doctorProblemSubject(key, expected string) error {
	obj, err := w.doctorJSON()
	if err != nil {
		return err
	}
	p, _ := obj["problems"].([]any)
	for _, value := range p {
		if value.(map[string]any)[key] == expected {
			return nil
		}
	}
	return fmt.Errorf("missing problem subject %s=%q\n%s", key, expected, w.report())
}
func (w *world) doctorPrepareFix() error {
	obj, err := w.doctorJSON()
	if err != nil {
		return err
	}
	p, _ := obj["problems"].([]any)
	for _, value := range p {
		problem := value.(map[string]any)
		if problem["rule"] != "doctor.image.missing" {
			continue
		}
		fix, _ := problem["fix"].(string)
		fix = strings.ToLower(fix)
		if strings.Contains(fix, "host") && strings.Contains(fix, "exact") && strings.Contains(fix, "outside") && strings.Contains(fix, "doctor") && (strings.Contains(fix, "prepare") || strings.Contains(fix, "acquire")) {
			return nil
		}
	}
	return fmt.Errorf("missing host-operator exact-image preparation instruction outside doctor\n%s", w.report())
}

func (w *world) doctorResultObject(index int) (map[string]any, error) {
	if index >= len(w.doctor.results) {
		return nil, fmt.Errorf("missing doctor result %d", index)
	}
	result := w.doctor.results[index]
	savedOut, savedErr, savedExit := w.stdout, w.stderr, w.exit
	defer func() { w.stdout, w.stderr, w.exit = savedOut, savedErr, savedExit }()
	w.stdout, w.stderr, w.exit = result.stdout, result.stderr, result.exit
	if err := w.doctorReady(); err != nil {
		return nil, err
	}
	return w.doctorJSON()
}
func (w *world) doctorThreeIdentities() error {
	if len(w.doctor.results) != 3 {
		return fmt.Errorf("expected three captured doctor results")
	}
	for i, result := range w.doctor.results {
		obj, err := w.doctorResultObject(i)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(obj["inspection"].(map[string]any)["checkout"], result.checkout) {
			return fmt.Errorf("result %d checkout differs from filesystem observation", i)
		}
	}
	return nil
}
func (w *world) doctorCompareCheckouts(first, second int, equal bool) error {
	a, err := w.doctorResultObject(first)
	if err != nil {
		return err
	}
	b, err := w.doctorResultObject(second)
	if err != nil {
		return err
	}
	same := reflect.DeepEqual(a["inspection"].(map[string]any)["checkout"], b["inspection"].(map[string]any)["checkout"])
	if same != equal {
		return fmt.Errorf("checkout identity equality = %v, want %v", same, equal)
	}
	return nil
}
func (w *world) doctorAllExits(count int) error {
	if len(w.doctor.results) != count {
		return fmt.Errorf("have %d results, want %d", len(w.doctor.results), count)
	}
	for i, r := range w.doctor.results {
		if r.exit != 0 {
			return fmt.Errorf("inspection %d exit %d", i, r.exit)
		}
	}
	return nil
}
func (w *world) doctorTwoIdentities() error {
	if len(w.doctor.results) != 2 {
		return fmt.Errorf("expected two captured doctor results")
	}
	for i, r := range w.doctor.results {
		obj, err := w.doctorResultObject(i)
		if err != nil {
			return err
		}
		e, _ := obj["inspection"].(map[string]any)["engine"].(map[string]any)
		if e["endpoint"] != r.endpoint || e["id"] != r.daemon {
			return fmt.Errorf("inspection %d differs from fixture identity", i)
		}
	}
	return nil
}
func (w *world) doctorCompareEngines() error {
	a, err := w.doctorResultObject(0)
	if err != nil {
		return err
	}
	b, err := w.doctorResultObject(1)
	if err != nil {
		return err
	}
	first := a["inspection"].(map[string]any)["engine"].(map[string]any)
	second := b["inspection"].(map[string]any)["engine"].(map[string]any)
	if first["endpoint"] != second["endpoint"] || first["id"] == second["id"] {
		return fmt.Errorf("daemon replacement not visible at unchanged endpoint")
	}
	return nil
}

func (w *world) doctorJournal() error {
	if err := w.doctorAllowedJournal(true); err != nil {
		return err
	}
	if w.doctor.engine == nil {
		return fmt.Errorf("missing API fixture")
	}
	seen := map[string]bool{}
	for _, r := range w.doctor.engine.requests() {
		seen[doctorRequestPath(r.Path)] = true
	}
	for _, path := range []string{"/_ping", "/version", "/info", "/images/" + w.doctor.image + "/json"} {
		if !seen[path] {
			return fmt.Errorf("API journal lacks %s", path)
		}
	}
	return nil
}
func (w *world) doctorEmptyJournal() error {
	if w.doctor.engine == nil {
		return fmt.Errorf("missing API fixture")
	}
	if requests := w.doctor.engine.requests(); len(requests) != 0 {
		return fmt.Errorf("unexpected API requests: %+v", requests)
	}
	return nil
}
func (w *world) doctorAllowedJournal(allowImage bool) error {
	if w.doctor.engine == nil {
		return nil
	} // unavailable socket has no responder
	for _, r := range w.doctor.engine.requests() {
		if !doctorAllowedRequest(r, w.doctor.image, allowImage) && !w.egressProxyInspection(r, allowImage) {
			return fmt.Errorf("forbidden API attempt: %+v", r)
		}
	}
	return nil
}

// An allowlist fixture also permits the plain inspection of the pinned proxy
// reference, only where image inspection is permitted at all. Every other image
// path, any query, and every mutation or acquisition stays forbidden.
func (w *world) egressProxyInspection(r doctorRequest, allowImage bool) bool {
	return allowImage && w.doctor.engine.egress != nil && r.Method == http.MethodGet && r.Query == "" && doctorRequestPath(r.Path) == "/images/"+busyboxFixturePin+"/json"
}
func (w *world) doctorNoConnection() error {
	a := w.doctor.engine
	if a == nil {
		return fmt.Errorf("missing connection recorder")
	}
	count := a.trapConnections.Load()
	// Timeout/redirect assertions concern only the redirect trap, not the selected
	// Engine API. Endpoint selection cases have not issued any engine inspection.
	if a.failure == "" && a.listener != nil {
		count += a.listener.connections.Load()
	}
	if count != 0 {
		return fmt.Errorf("trap endpoint received %d connections", count)
	}
	return nil
}
func (w *world) doctorNoMarker() error {
	entries, err := os.ReadDir(w.doctor.recorder)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("a checkout/helper marker ran: %v", entries)
	}
	return nil
}
func (w *world) doctorUnchanged() error {
	actual, err := doctorTree(w.dir)
	if err != nil {
		return err
	}
	if w.doctor.snapshot == nil || !reflect.DeepEqual(actual, w.doctor.snapshot) {
		return fmt.Errorf("doctor changed fixture files, permissions or host control state")
	}
	return nil
}
func (w *world) doctorReplacementUnchanged() error {
	if w.doctor.replacementSnapshot == nil {
		return fmt.Errorf("image response was never reached; no replacement occurred")
	}
	actual, err := doctorTree(w.doctor.replacement)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, w.doctor.replacementSnapshot) {
		return fmt.Errorf("doctor changed the replacement tree")
	}
	return nil
}
func (w *world) doctorNoCanary() error {
	if strings.Contains(w.stdout+w.stderr, doctorCanary) {
		return fmt.Errorf("synthetic canary escaped into output")
	}
	current, err := doctorTree(w.dir)
	if err != nil {
		return err
	}
	for path, contents := range current {
		if w.doctor.snapshot[path] != contents && strings.Contains(contents, doctorCanary) {
			return fmt.Errorf("synthetic canary persisted at %s", path)
		}
	}
	return nil
}
