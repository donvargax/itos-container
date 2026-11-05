package features

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

func (w *world) approvalContains(values ...string) error {
	for _, v := range values {
		if !strings.Contains(w.stdout+w.stderr, v) {
			return fmt.Errorf("missing %q\n%s", v, w.report())
		}
	}
	return nil
}
func (w *world) approvalView() (map[string]any, error) {
	obj, err := w.json()
	if err != nil {
		return nil, err
	}
	if obj["schema"] != float64(1) {
		return nil, fmt.Errorf("missing schema 1")
	}
	a, ok := obj["approval"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing approval object\n%s", w.report())
	}
	return a, nil
}
func (w *world) approvalCandidate() error {
	if err := w.approvalOracle(); err != nil {
		return err
	}
	if strings.HasPrefix(strings.TrimSpace(w.stdout), "{") {
		view, err := w.approvalView()
		if err != nil {
			return err
		}
		if view["fingerprint"] != w.approval.fingerprint || !bytes.Equal(approvalJSON(view["effective"]), approvalJSON(w.approval.settings)) {
			return fmt.Errorf("candidate differs from independent canonical oracle\n%s\nwant %s", w.report(), approvalJSON(w.approval.settings))
		}
		return nil
	}
	if err := w.approvalContains(w.approval.fingerprint); err != nil {
		return err
	}
	return w.approvalTextSettings(w.stdout)
}
func (w *world) approvalTextSettings(text string) error {
	// Every leaf, including null/default values and all identity fields, appears as
	// canonical JSON in the review. The whole object allows unambiguous comparison.
	want := string(approvalJSON(w.approval.settings))
	if !strings.Contains(text, want) {
		return fmt.Errorf("review lacks complete canonical settings\n%s", w.report())
	}
	return nil
}
func (w *world) approvalStatus(status string) error {
	if strings.HasPrefix(strings.TrimSpace(w.stdout), "{") {
		view, err := w.approvalView()
		if err != nil {
			return err
		}
		if view["status"] != status {
			return fmt.Errorf("approval status = %v, want %s\n%s", view["status"], status, w.report())
		}
		return nil
	}
	return w.approvalContains("approval", status)
}
func (w *world) approvalStatusGuidance(status, guidance string) error {
	if err := w.approvalStatus(status); err != nil {
		return err
	}
	return w.approvalContains(guidance)
}
func (w *world) approvalProblem(rule string, code int, field string) error {
	if err := w.exitsWith(code); err != nil {
		return err
	}
	if !strings.HasPrefix(strings.TrimSpace(w.stdout), "{") {
		for _, line := range strings.Split(w.stderr, "\n") {
			if strings.HasPrefix(line, rule+": ") {
				message, fix, ok := strings.Cut(strings.TrimPrefix(line, rule+": "), "; ")
				if ok && message != "" && fix != "" {
					return nil
				}
			}
		}
		return fmt.Errorf("missing complete plain problem %s\n%s", rule, w.report())
	}
	obj, err := w.json()
	if err != nil {
		return err
	}
	if obj["schema"] != float64(1) || obj["ok"] != false {
		return fmt.Errorf("invalid refusal envelope\n%s", w.report())
	}
	problems, ok := obj["problems"].([]any)
	if !ok || len(problems) == 0 {
		return fmt.Errorf("missing problem")
	}
	for _, raw := range problems {
		p, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if p["rule"] != rule {
			continue
		}
		if p["exit"] != float64(code) || p["message"] == "" || p["fix"] == "" || p["message"] == nil || p["fix"] == nil {
			return fmt.Errorf("incomplete problem %v", p)
		}
		if field != "" && p["field"] != field {
			return fmt.Errorf("problem field = %v", p["field"])
		}
		if strings.HasPrefix(rule, "approval.state.") {
			a, err := w.approvalView()
			if err != nil {
				return err
			}
			if a["status"] != "invalid" {
				return fmt.Errorf("unsafe state was not invalid")
			}
		}
		return nil
	}
	return fmt.Errorf("missing rule %s\n%s", rule, w.report())
}
func (w *world) approvalInspections(changed bool) error {
	if len(w.approval.results) != 2 {
		return fmt.Errorf("need two captured inspections")
	}
	first, last := w.approval.results[0], w.approval.results[1]
	var a, b map[string]any
	if err := json.Unmarshal([]byte(first.stdout), &a); err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(last.stdout), &b); err != nil {
		return err
	}
	av, aok := a["approval"].(map[string]any)
	bv, bok := b["approval"].(map[string]any)
	if !aok || !bok || first.exit != 0 || last.exit != 0 || av["status"] != "matching" {
		return fmt.Errorf("inspections lack successful matching baseline\n%s", w.report())
	}
	expected := "matching"
	if changed {
		expected = "changed"
	}
	if bv["status"] != expected || (av["fingerprint"] != bv["fingerprint"]) != changed {
		return fmt.Errorf("wrong approval comparison")
	}
	if av["fingerprint"] != "sha256:"+approvalHash(w.approval.prior) {
		return fmt.Errorf("baseline fingerprint differs from oracle")
	}
	return w.approvalCandidate()
}
func (w *world) approvalDifference(pointer string) error {
	view, err := w.approvalView()
	if err != nil {
		return err
	}
	diffs, ok := view["differences"].([]any)
	if !ok {
		return fmt.Errorf("missing differences")
	}
	var paths []string
	found := false
	for _, raw := range diffs {
		d := raw.(map[string]any)
		path, _ := d["path"].(string)
		paths = append(paths, path)
		if path == pointer {
			if !bytes.Equal(approvalJSON(d["before"]), approvalJSON(approvalAt(w.approval.prior, path))) || !bytes.Equal(approvalJSON(d["after"]), approvalJSON(approvalAt(w.approval.settings, path))) {
				return fmt.Errorf("incorrect old/new values at %s", path)
			}
			found = true
		}
	}
	if !found || !sort.StringsAreSorted(paths) {
		return fmt.Errorf("missing or unsorted difference %s", pointer)
	}
	return nil
}
func approvalAt(obj map[string]any, pointer string) any {
	var v any = obj
	for _, key := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[key]
	}
	return v
}
func (w *world) approvalReview(differences bool) error {
	text := w.approval.beforeReview
	at := strings.Index(text, "y/N")
	if at < 0 || strings.Count(w.stdout, "y/N") != 1 {
		return fmt.Errorf("expected one review before input\n%s", w.report())
	}
	if err := w.approvalTextSettings(text[:at]); err != nil {
		return err
	}
	for _, v := range []string{"/workspace", "read-only", "no-new-privileges", "4096", "tmpfs", "capabilities", "network", "supplementary"} {
		if !strings.Contains(text[:at], v) {
			return fmt.Errorf("review lacks restriction %q", v)
		}
	}
	if differences {
		prev := -1
		for _, path := range []string{"/manifest/containerEnv", "/manifest/workspaceFolder"} {
			i := strings.Index(text[:at], path)
			if i <= prev {
				return fmt.Errorf("missing sorted differences")
			}
			prev = i
			for _, value := range []any{approvalAt(w.approval.prior, path), approvalAt(w.approval.settings, path)} {
				if !strings.Contains(text[:at], string(approvalJSON(value))) {
					return fmt.Errorf("difference lacks value")
				}
			}
		}
	}
	return nil
}
func (w *world) approvalTerminalRefusal(rule string, exit int) error {
	if err := w.exitsWith(exit); err != nil {
		return err
	}
	if err := w.approvalContains(rule); err != nil {
		return err
	}
	if !strings.Contains(w.approval.beforeReview, "y/N") {
		return fmt.Errorf("no review before refusal")
	}
	return nil
}
func (w *world) approvalSuccess() error {
	if err := w.exitsWith(0); err != nil {
		return err
	}
	if strings.Contains(w.stdout+w.stderr, "y/N") {
		return fmt.Errorf("unexpected prompt")
	}
	if err := w.approvalStatus("matching"); err != nil {
		return err
	}
	return w.approvalCandidate()
}
func (w *world) approvalStored(generation string) error {
	a := w.approval
	data, err := os.ReadFile(a.record)
	if err != nil {
		return fmt.Errorf("missing active record: %w\n%s", err, w.report())
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	expected := map[string]any{"schema": 1, "project": a.project, "generation": generation, "fingerprint": a.fingerprint, "settings": a.settings}
	if !bytes.Equal(data, approvalJSON(expected)) {
		return fmt.Errorf("active record differs from exact expected canonical record\ngot %s\nwant %s", data, approvalJSON(expected))
	}
	return w.approvalPermissions()
}
func (w *world) approvalPermissions() error {
	a := w.approval
	root := filepath.Dir(filepath.Dir(filepath.Dir(a.record)))
	if !filepath.IsAbs(root) {
		return nil
	}
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		mode := os.FileMode(0600)
		if info.IsDir() {
			mode = 0700
		}
		if info.Mode().Perm() != mode || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("unsafe managed entry %s (%s)", path, info.Mode())
		}
		stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
		uid := stat.FieldByName("Uid")
		if !uid.IsValid() || fmt.Sprint(uid.Interface()) != strconv.Itoa(os.Geteuid()) {
			return fmt.Errorf("wrong owner for %s", path)
		}
		return nil
	})
}
func (w *world) approvalRecordUnchanged() error {
	a := w.approval
	data, err := os.ReadFile(a.record)
	if os.IsNotExist(err) && len(a.recordBefore) == 0 {
		return nil
	}
	if err != nil {
		return err
	}
	if !bytes.Equal(data, a.recordBefore) {
		return fmt.Errorf("active approval bytes changed")
	}
	if a.recordInfo != nil {
		info, err := os.Stat(a.record)
		if err != nil {
			return err
		}
		if !os.SameFile(info, a.recordInfo) || !info.ModTime().Equal(a.recordInfo.ModTime()) {
			return fmt.Errorf("active approval was rewritten")
		}
	}
	return nil
}
func (w *world) approvalNoRecord() error {
	if _, err := os.Lstat(w.approval.record); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if w.approval.before != nil {
		return w.approvalSameTree()
	}
	return fmt.Errorf("unexpected approval record")
}

func (w *world) approvalOnlyState() error {
	actual, err := doctorTree(w.dir)
	if err != nil {
		return err
	}
	before := w.approval.before
	if before == nil {
		before = w.doctor.snapshot
	}
	allowed := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")
	if w.doctor.env["XDG_STATE_HOME"] == "" {
		allowed = filepath.Join(w.doctor.env["HOME"], ".local")
	}
	for path, value := range actual {
		if path == allowed || strings.HasPrefix(path, allowed+string(os.PathSeparator)) {
			continue
		}
		if before[path] != value {
			return fmt.Errorf("changed outside approval state: %s", path)
		}
	}
	for path := range before {
		if _, ok := actual[path]; !ok {
			return fmt.Errorf("deleted %s", path)
		}
	}
	return nil
}
func (w *world) approvalOutputMode() error {
	jsonMode := w.approval.mode == "--json" || w.approval.mode == "inherited ITOS_JSON=1"
	if jsonMode {
		obj, err := w.json()
		if err != nil {
			return err
		}
		if obj["schema"] != float64(1) || obj["ok"] != true {
			return fmt.Errorf("wrong success envelope")
		}
		problems, ok := obj["problems"].([]any)
		if !ok || len(problems) != 0 {
			return fmt.Errorf("success problems")
		}
		return w.approvalCandidate()
	}
	if strings.HasPrefix(w.stdout, "{") {
		return fmt.Errorf("plain output is JSON")
	}
	return nil
}
func (w *world) approvalLocation(outcome, path string, code int) error {
	if err := w.exitsWith(code); err != nil {
		return err
	}
	if outcome != "matching" {
		if path != "none" {
			return fmt.Errorf("invalid fixture path")
		}
		return w.approvalProblem(outcome, code, "")
	}
	want := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")
	if strings.HasPrefix(path, "HOME/") {
		want = filepath.Join(w.doctor.env["HOME"], ".local/state/itos-container")
	}
	if filepath.Dir(filepath.Dir(filepath.Dir(w.approval.record))) != want {
		return fmt.Errorf("wrong expected state path")
	}
	return w.approvalStored("1")
}
func (w *world) approvalFirstWinner() error {
	if len(w.approval.results) != 1 || w.approval.results[0].exit != 0 {
		return fmt.Errorf("first approval did not succeed")
	}
	return w.approvalStored("5")
}
func (w *world) approvalBoundary() error {
	if w.approval.settings["profile"] != "offline-v1" {
		return fmt.Errorf("wrong profile")
	}
	// Exact complete settings excludes mount extensions and host-control paths.
	if err := w.approvalStored("1"); err != nil {
		return err
	}
	for _, path := range []string{w.doctor.env["HOME"], w.doctor.env["XDG_STATE_HOME"]} {
		if strings.Contains(string(approvalJSON(w.approval.settings)), path) {
			return fmt.Errorf("host control path in approval")
		}
	}
	return nil
}
func (w *world) approvalPresentation(presentation string, code int) error {
	if err := w.exitsWith(code); err != nil {
		return err
	}
	switch presentation {
	case "escaped review before consent":
		if strings.Contains(w.approval.beforeReview, "\x1b") {
			return fmt.Errorf("raw terminal control")
		}
		return w.approvalReview(false)
	case "confirmation-required JSON, no prompt":
		if strings.Contains(w.stdout, "y/N") {
			return fmt.Errorf("JSON prompted")
		}
		return w.approvalProblem("approve.confirmation-required", 1, "")
	case "one matching JSON object, no prompt":
		return w.approvalSuccess()
	default:
		return fmt.Errorf("unknown presentation")
	}
}
func (w *world) approvalLiteral() error {
	var settings any
	if strings.HasPrefix(strings.TrimSpace(w.stdout), "{") {
		view, err := w.approvalView()
		if err != nil {
			return err
		}
		settings = view["effective"]
	} else {
		data, err := os.ReadFile(w.approval.record)
		if err != nil {
			return err
		}
		var record map[string]any
		if err := json.Unmarshal(data, &record); err != nil {
			return err
		}
		settings = record["settings"]
	}
	obj, ok := settings.(map[string]any)
	if !ok || approvalAt(obj, "/manifest/containerEnv/APP") != w.approval.literal {
		return fmt.Errorf("literal value was not preserved")
	}
	return nil
}
