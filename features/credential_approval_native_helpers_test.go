package features

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"
)

func approvalDisplayedJSONLine(text string, want map[string]any) error {
	for _, line := range strings.Split(text, "\n") {
		start := strings.Index(line, "{")
		if start >= 0 && approvalDisplayedJSON(strings.TrimSpace(line[start:]), want) == nil {
			return nil
		}
	}
	return fmt.Errorf("missing complete displayed settings")
}

func credentialApprovalEscape(text string) string {
	var b strings.Builder
	for _, r := range text {
		if unicode.IsPrint(r) {
			b.WriteRune(r)
		} else {
			for _, u := range utf16.Encode([]rune{r}) {
				fmt.Fprintf(&b, `\u%04x`, u)
			}
		}
	}
	return b.String()
}

func (w *world) credentialApprovalRestriction() string {
	p := w.approval.settings["profile"].(string)
	network := "network none"
	if w.approval.allow != nil {
		network = "internal network with no route out"
	}
	return p + ": checkout-only host directory mount at /workspace plus read-only credential files under /run/itos-container/credentials; UID:GID override; no supplementary groups; read-only root; dropped capabilities; no-new-privileges; " + network + "; no ports or hooks; ephemeral tmpfs; pids limit 4096"
}

func (w *world) credentialApprovalLines(text string) error {
	var actual []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "credentials:") {
			actual = append(actual, line)
		}
	}
	var want []string
	var grants []credentialGrant
	_ = json.Unmarshal(approvalJSON(w.approval.settings["credentials"]), &grants)
	for _, g := range grants {
		want = append(want, "credentials: "+g.Kind+" source="+credentialApprovalEscape(string(approvalJSON(g.Source)))+" target="+string(approvalJSON(g.Target)))
	}
	if !slices.Equal(actual, want) {
		return fmt.Errorf("credential display lines differ from exact sorted grants")
	}
	return nil
}

func credentialApprovalDisclosure(text, network string, warning bool) error {
	want := slices.Clone(credentialDisclosure[:3])
	if network == "allowlist" {
		want = append(want, credentialDisclosure[3:5]...)
	}
	if warning {
		want = append(want, credentialDisclosure[5])
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	for _, line := range want {
		_, rest, ok := strings.Cut(text, line+"\n")
		if !ok {
			return fmt.Errorf("missing ordered credential disclosure %q", line)
		}
		text = rest
	}
	return nil
}

func (w *world) credentialApprovalDescribe(text string, warning bool) error {
	if err := w.credentialApprovalLines(text); err != nil {
		return err
	}
	if len(w.credentialApproval.grants) == 0 {
		if strings.Contains(text, credentialDisclosure[0]) {
			return fmt.Errorf("no-grant result claims credential access")
		}
		return nil
	}
	network := "offline"
	if w.approval.allow != nil {
		network = "allowlist"
	}
	return credentialApprovalDisclosure(text, network, warning)
}

func (w *world) credentialApprovalSeedSettings(settings map[string]any, generation string) error {
	a := w.approval
	record := map[string]any{"schema": 1, "project": a.project, "generation": generation, "fingerprint": "sha256:" + approvalHash(settings), "settings": settings}
	if err := writeManifestFixture(a.record, approvalJSON(record), 0600); err != nil {
		return err
	}
	a.prior = approvalClone(settings)
	return w.approvalCapture()
}

func (w *world) credentialApprovalDefect(defect string) error {
	if defect == "allowlist credential profile without proxy" {
		if err := w.credentialApprovalRequest("both", "allowlist"); err != nil {
			return err
		}
	}
	s := approvalClone(w.approval.settings)
	grants := s["credentials"].([]any)
	first := object(grants[0])
	switch defect {
	case "credential profile without credentials":
		delete(s, "credentials")
	case "credential profile with empty credentials":
		s["credentials"] = []any{}
	case "credential profile with null credentials":
		s["credentials"] = nil
	case "credentials under offline-v1":
		s["profile"] = "offline-v1"
	case "credential profile at policy version 2":
		object(s["policy"])["version"] = 2
	case "unknown credential kind":
		first["kind"] = "other-token"
	case "duplicate credential kind":
		object(grants[1])["kind"] = first["kind"]
	case "grants out of kind order":
		slices.Reverse(grants)
	case "relative source path":
		first["source"] = "relative/token"
	case "source containing a dot-dot component":
		first["source"] = "/vault/../token"
	case "target outside the reserved credential directory":
		first["target"] = "/run/token"
	case "duplicate target":
		object(grants[1])["target"] = first["target"]
	case "extra value member containing synthetic fixture data":
		first["value"] = w.credentials.markers[0]
	case "allowlist credential profile without proxy":
		delete(s, "proxy")
	default:
		return fmt.Errorf("unknown credential record defect %q", defect)
	}
	return w.credentialApprovalSeedSettings(s, "4")
}

func (w *world) credentialApprovalSourceEdit(name, defect string) error {
	path := w.credentials.paths[name]
	switch defect {
	case "missing":
		return os.Remove(path)
	case "group-readable":
		return os.Chmod(path, 0640)
	case "checkout alias":
		return os.Link(path, filepath.Join(w.doctor.checkout, "credential-alias"))
	case "symlink":
		if err := os.Remove(path); err != nil {
			return err
		}
		return os.Symlink(w.credentials.paths["claude-b"], path)
	case "rotation":
		temp := path + "-replacement"
		if err := os.WriteFile(temp, []byte(w.credentials.markers[1]), 0600); err != nil {
			return err
		}
		return os.Rename(temp, path)
	default:
		return fmt.Errorf("unknown source edit %q", defect)
	}
}

func (w *world) credentialApprovalEdit(edit string) error {
	switch edit {
	case "add the github grant":
		return w.credentialApprovalRequest("both", "offline")
	case "remove the last grant":
		return w.credentialApprovalRequest("none", "offline")
	case "repoint claude to claude-b":
		return w.credentialApprovalRequest("repointed claude", "offline")
	case "remove claude-a":
		return w.credentialApprovalSourceEdit("claude-a", "missing")
	case "make claude-a group-readable":
		return w.credentialApprovalSourceEdit("claude-a", "group-readable")
	case "replace claude-a with a symlink":
		return w.credentialApprovalSourceEdit("claude-a", "symlink")
	case "replace claude-a with a different valid private inode":
		return w.credentialApprovalSourceEdit("claude-a", "rotation")
	case "rotate both source contents in place within the permitted size":
		for _, g := range w.credentialApproval.grants {
			if err := os.WriteFile(g.Source, []byte(w.credentials.markers[1]), 0600); err != nil {
				return err
			}
		}
		return nil
	case "reverse credential entries and reorder keys with YAML comments":
		grants := slices.Clone(w.credentialApproval.grants)
		slices.Reverse(grants)
		var b strings.Builder
		b.WriteString("# equivalent request\nnetwork: {mode: offline}\ncredentials:\n")
		for _, g := range grants {
			fmt.Fprintf(&b, "  - source: %s\n    kind: %s\n", approvalJSON(g.Source), g.Kind)
		}
		b.WriteString("manifest: .devcontainer/devcontainer.json\nversion: 3\n")
		return os.WriteFile(filepath.Join(w.doctor.checkout, "itos-container.yaml"), []byte(b.String()), 0600)
	case "use equivalent JSON escapes in the source paths":
		data, err := os.ReadFile(filepath.Join(w.doctor.checkout, "itos-container.yaml"))
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(w.doctor.checkout, "itos-container.yaml"), []byte(strings.ReplaceAll(string(data), "/", `\u002f`)), 0600)
	default:
		return fmt.Errorf("unknown credential edit %q", edit)
	}
}

func (w *world) credentialApprovalTerminal(mode, answer, edit, streams string) error {
	if err := w.approvalOracle(); err != nil {
		return err
	}
	finish, err := w.credentialApprovalWatch()
	if err != nil {
		return err
	}
	p, err := w.startApproval(mode, streams, answer)
	if err != nil {
		finish()
		return err
	}
	if edit != "" && strings.Contains(p.review, "y/N") {
		// Harness edits and baseline reads occur outside the product observer.
		finish()
		if err := w.credentialApprovalEdit(edit); err != nil {
			p.cancel()
			w.finishApproval(p)
			return err
		}
		if err := w.approvalCapture(); err != nil {
			p.cancel()
			w.finishApproval(p)
			return err
		}
		finish, err = w.credentialApprovalWatch()
		if err != nil {
			p.cancel()
			w.finishApproval(p)
			return err
		}
	}
	p.answer(answer)
	w.finishApproval(p)
	finish()
	return nil
}

func (w *world) credentialApprovalFingerprint(which, streams, mode string) error {
	value := w.approval.fingerprint
	switch which {
	case "exact":
	case "mismatching":
		value = "sha256:" + strings.Repeat("0", 64)
	case "malformed":
		value = "malformed"
	case "empty":
		value = ""
	default:
		return fmt.Errorf("unknown fingerprint")
	}
	if streams == "both terminals" {
		// The PTY helper's exact-fingerprint route takes its value from the oracle.
		original := w.approval.fingerprint
		w.approval.fingerprint = value
		finish, err := w.credentialApprovalWatch()
		if err != nil {
			return err
		}
		p, err := w.startApproval(mode, "", "exact fingerprint")
		w.approval.fingerprint = original
		if err != nil {
			finish()
			return err
		}
		p.answer("yes")
		w.finishApproval(p)
		finish()
		return nil
	}
	env, args := w.approvalMode(mode)
	args = append(args, "--fingerprint="+value)
	return w.runDoctor(env, args...)
}

func (w *world) credentialApprovalInvoke(command string) error {
	run := func() error {
		w.approval.mode = command
		switch command {
		case "bare terminal approve":
			return w.credentialApprovalTerminal("plain", "unavailable", "", "")
		case "terminal approve with yes":
			return w.credentialApprovalTerminal("plain", "yes", "", "")
		case "terminal approve with inherited JSON overridden by --no-json and yes":
			return w.credentialApprovalTerminal("ITOS_JSON=1 with --no-json", "yes", "", "")
		default:
			return w.runDoctor(w.env(), strings.Fields(command)...)
		}
	}
	if w.approval.condition != "credential engine barrier" {
		return run()
	}
	done := make(chan error, 1)
	go func() { done <- run() }()
	select {
	case err := <-done:
		return err
	case <-w.doctor.engine.reached:
		err := os.Chmod(w.credentials.paths["claude-a"], 0640)
		w.doctor.engine.unpause()
		return errors.Join(err, <-done)
	case <-time.After(9 * time.Second):
		w.doctor.engine.unpause()
		<-done
		return fmt.Errorf("credential engine barrier timed out")
	}
}

func (w *world) credentialApprovalProblem(rule, path string, exit int) error {
	if err := w.exitsWith(exit); err != nil {
		return err
	}
	if strings.Contains(w.stdout, "y/N") {
		return fmt.Errorf("credential refusal prompted")
	}
	if json.Valid([]byte(w.stdout)) {
		obj, err := w.json()
		if err != nil {
			return err
		}
		for _, raw := range obj["problems"].([]any) {
			p := object(raw)
			if p["rule"] == rule && p["path"] == path && p["exit"] == float64(exit) && p["message"] != "" && p["fix"] != "" {
				return nil
			}
		}
		return fmt.Errorf("missing located credential problem %s at %s", rule, path)
	}
	if !strings.Contains(w.stderr, rule) || !strings.Contains(w.stderr, "itos-container.yaml"+path) {
		return fmt.Errorf("missing plain located credential refusal")
	}
	return nil
}

func (w *world) credentialApprovalRequested() error {
	if !json.Valid([]byte(w.stdout)) {
		return w.credentialApprovalLines(w.stdout)
	}
	obj, err := w.json()
	if err != nil {
		return err
	}
	if object(obj["inspection"])["ready"] != false {
		return fmt.Errorf("unsafe credential inspection is ready")
	}
	a := object(obj["approval"])
	if a["fingerprint"] != "" || a["effective"] != nil {
		return fmt.Errorf("unsafe credential candidate usable")
	}
	var got any
	_ = json.Unmarshal(approvalJSON(w.approval.settings["credentials"]), &got)
	if !reflect.DeepEqual(obj["credentials"], got) {
		return fmt.Errorf("requested grants missing after source refusal")
	}
	return nil
}

func (w *world) credentialApprovalNoPublication() error {
	if err := w.approvalRecordUnchanged(); err != nil {
		return err
	}
	root := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")
	return filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if strings.Contains(path, string(filepath.Separator)+"snapshots"+string(filepath.Separator)) && !e.IsDir() {
			return fmt.Errorf("unexpected frozen publication")
		}
		return nil
	})
}

func (w *world) credentialApprovalCapturePrivate() error { return w.approvalCapture() }

func (w *world) credentialApprovalTree() (map[string]string, error) {
	tree := map[string]string{}
	err := filepath.WalkDir(w.dir, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		// Access/change timestamps include the harness's baseline reads and
		// temporary permission for installing a mode-0000 source watch.
		stat := reflect.Indirect(reflect.ValueOf(info.Sys()))
		value := fmt.Sprintf("%v|%d", info.Mode(), info.Size())
		for _, name := range []string{"Uid", "Gid", "Dev", "Ino", "Nlink"} {
			if f := stat.FieldByName(name); f.IsValid() {
				value += fmt.Sprintf("|%s=%v", name, f.Interface())
			}
		}
		if info.Mode().IsRegular() {
			mode := info.Mode().Perm()
			if mode&0400 == 0 {
				if err := os.Chmod(path, mode|0400); err != nil {
					return err
				}
			}
			data, readErr := os.ReadFile(path)
			if mode&0400 == 0 {
				readErr = errors.Join(readErr, os.Chmod(path, mode))
			}
			if readErr != nil {
				return readErr
			}
			value += "|" + string(data)
		} else if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			value += "|" + target
		}
		tree[path] = fmt.Sprintf("%s|mtime=%d", value, info.ModTime().UnixNano())
		return nil
	})
	return tree, err
}

func (w *world) credentialApprovalNoAccess() error {
	if w.credentialApproval.accessError != nil {
		return w.credentialApproval.accessError
	}
	return w.credentialApprovalScan()
}

func (w *world) credentialApprovalNeedles() []string {
	var needles []string
	for _, marker := range w.credentials.markers {
		needles = append(needles, marker)
		for i := 0; i < 3; i++ {
			text := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", i) + marker))
			needles = append(needles, text[4:len(text)-4])
		}
	}
	return needles
}

func (w *world) credentialApprovalMarkerControl() error {
	needles := w.credentialApprovalNeedles()
	if len(needles) == 0 {
		return fmt.Errorf("no marker control")
	}
	for _, n := range needles {
		if !strings.Contains(strings.Join(needles, "\n"), n) {
			return fmt.Errorf("marker positive control failed")
		}
	}
	return nil
}

func (w *world) credentialApprovalScan() error {
	needles := w.credentialApprovalNeedles()
	check := func(data string) error {
		for _, n := range needles {
			if strings.Contains(data, n) {
				return fmt.Errorf("synthetic credential escaped")
			}
		}
		return nil
	}
	if err := check(w.stdout + w.stderr); err != nil {
		return err
	}
	for _, r := range w.approval.results {
		if err := check(r.stdout + r.stderr); err != nil {
			return err
		}
	}
	journal := string(approvalJSON(w.doctor.engine.requests()))
	if err := check(journal); err != nil {
		return err
	}
	for _, root := range []string{w.doctor.checkout, filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")} {
		if err := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if !e.Type().IsRegular() {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return check(string(data))
		}); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) credentialApprovalLiteralPaths(text string) error {
	suffix := "\u0085\u202e\u2066"
	if text == "quotes dollar signs backticks and U+2028" {
		suffix = "\"$()`echo nope`\u2028"
	} else if text != "Unicode C1 and bidirectional controls" {
		return fmt.Errorf("unknown literal fixture")
	}
	prior := filepath.Join(w.credentials.sources, "prior"+suffix)
	current := filepath.Join(w.credentials.sources, "current"+suffix)
	for _, path := range []string{prior, current} {
		if err := os.WriteFile(path, []byte(w.credentials.markers[0]), 0600); err != nil {
			return err
		}
	}
	w.credentials.paths["claude-a"] = prior
	if err := w.credentialApprovalRequest("claude", "offline"); err != nil {
		return err
	}
	if err := w.approvalSeed("3", false); err != nil {
		return err
	}
	w.credentials.paths["claude-a"] = current
	return w.credentialApprovalRequest("claude", "offline")
}
