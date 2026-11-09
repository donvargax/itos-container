package features

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

func (w *world) approvalNetworkSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the checkout (?:then )?requests the allowlist "([^"]*)"$`, func(names string) error {
		list := strings.Fields(names)
		if len(list) == 0 || !sort.StringsAreSorted(list) {
			return fmt.Errorf("fixture names must be sorted and nonempty")
		}
		for i, name := range list {
			if name != strings.ToLower(name) || strings.ContainsAny(name, "* ") || (i > 0 && name == list[i-1]) {
				return fmt.Errorf("noncanonical fixture name %q", name)
			}
		}
		return w.approvalNetworkRequest(2, list)
	})
	sc.Step(`^the checkout (?:then )?requests offline at policy version "([^"]*)"$`, func(v string) error {
		n, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		return w.approvalNetworkRequest(n, nil)
	})
	sc.Step(`^the checkout (?:then )?requests an allowlist of (\d+) generated host names$`, func(count int) error {
		names := make([]string, count)
		for i := range names {
			names[i] = fmt.Sprintf("n%d.example", i)
		}
		sort.Strings(names)
		return w.approvalNetworkRequest(2, names)
	})
	sc.Step(`^the checkout (?:then )?requests an allowlist of one host name of 253 bytes$`, func() error {
		return w.approvalNetworkRequest(2, []string{approvalLongName(61)})
	})
	sc.Step(`^a seeded allowlist approval at generation "([^"]*)" that bound another proxy image digest$`, func(g string) error {
		if err := w.approvalSeed(g, false); err != nil {
			return err
		}
		s := w.approval.prior
		s["proxy"] = map[string]any{"image": "docker.io/library/busybox@sha256:" + strings.Repeat("a", 64)}
		record := map[string]any{"schema": 1, "project": w.approval.project, "generation": g, "settings": s, "fingerprint": "sha256:" + approvalHash(s)}
		if err := os.WriteFile(w.approval.record, approvalJSON(record), 0600); err != nil {
			return err
		}
		return w.approvalCapture()
	})
	sc.Step(`^approve runs without a terminal with the independently calculated fingerprint of "([^"]*)" as JSON$`, w.approvalOtherNetworkFingerprint)
	sc.Step(`^approve runs at a terminal in "([^"]*)" with streams "([^"]*)"$`, func(mode, streams string) error {
		return w.approvalTerminal(mode, "y", "", streams)
	})
	sc.Step(`^its approval differences are exactly "([^"]*)" with the prior and current values$`, w.approvalExactDifferences)
	sc.Step(`^the transcript shows differences "([^"]*)" with old and new values before exactly one y/N prompt$`, w.approvalReviewDifferences)
	sc.Step(`^the transcript shows all effective settings and the worker's restrictions before exactly one y/N prompt$`, func() error { return w.approvalReview(false) })
	sc.Step(`^the review before its prompt says, in this order:$`, func(table *godog.Table) error {
		text, err := w.approvalBeforePrompt()
		if err != nil {
			return err
		}
		return approvalOrdered(text, table)
	})
	sc.Step(`^the review before its prompt does not say "([^"]*)"$`, func(value string) error {
		text, err := w.approvalBeforePrompt()
		if err != nil {
			return err
		}
		if strings.Contains(text, value) {
			return fmt.Errorf("review contains %q", value)
		}
		return nil
	})
	sc.Step(`^its standard output says, in this order:$`, func(table *godog.Table) error { return approvalOrdered(w.stdout, table) })
	sc.Step(`^the review before its prompt lists exactly the requested host names$`, func() error {
		text, err := w.approvalBeforePrompt()
		if err != nil {
			return err
		}
		return w.approvalListedNames(text)
	})
	sc.Step(`^its standard output lists exactly the requested host names$`, func() error { return w.approvalListedNames(w.stdout) })
	sc.Step(`^its approval status is "([^"]*)"$`, w.approvalStatus)
	sc.Step(`^the host state is unchanged$`, w.approvalHostUnchanged)
}

func approvalLongName(last int) string {
	return strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", last)
}

func (w *world) approvalNetworkRequest(version int, names []string) error {
	w.approval.policyVersion, w.approval.allow = version, names
	if err := w.approvalNetworkFile(names); err != nil {
		return err
	}
	if err := w.approvalOracle(); err != nil {
		return err
	}
	return w.approvalCapture()
}
func (w *world) approvalNetworkFile(names []string) error {
	data := fmt.Sprintf("version: %d\nmanifest: .devcontainer/devcontainer.json\n", w.approval.policyVersion)
	if names != nil {
		data += "network:\n  mode: allowlist\n  allow:\n"
		for _, name := range names {
			data += "    - " + name + "\n"
		}
	}
	return os.WriteFile(filepath.Join(w.doctor.checkout, "itos-container.yaml"), []byte(data), 0600)
}
func (w *world) approvalNetworkEdit(edit string) error {
	a := w.approval
	names := append([]string(nil), a.allow...)
	switch {
	case edit == "reorder the allowlist":
		for i, j := 0, len(names)-1; i < j; i, j = i+1, j-1 {
			names[i], names[j] = names[j], names[i]
		}
		return w.approvalNetworkFile(names)
	case edit == "respell the allowlist's names":
		return w.approvalNetworkFile([]string{"API.Example.COM.", "BÜCHER.example"})
	case edit == "write the allowlist in flow style with comments and reordered keys":
		data := "# unchanged\nnetwork: {allow: [" + strings.Join(names, ", ") + "], mode: allowlist}\nmanifest: .devcontainer/devcontainer.json\nversion: 2\n"
		return os.WriteFile(filepath.Join(w.doctor.checkout, "itos-container.yaml"), []byte(data), 0600)
	case edit == "write the policy version as 2.0":
		path := filepath.Join(w.doctor.checkout, "itos-container.yaml")
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(path, []byte(strings.Replace(string(b), "version: 2\n", "version: 2.0\n", 1)), 0600)
	case strings.HasPrefix(edit, "add the host name "):
		names = append(names, strings.TrimPrefix(edit, "add the host name "))
	case strings.HasPrefix(edit, "remove the host name "):
		remove := strings.TrimPrefix(edit, "remove the host name ")
		names = nil
		for _, name := range a.allow {
			if name != remove {
				names = append(names, name)
			}
		}
	case strings.HasPrefix(edit, "change the host name "):
		old, next, ok := strings.Cut(strings.TrimPrefix(edit, "change the host name "), " to ")
		if !ok {
			return fmt.Errorf("bad edit %q", edit)
		}
		if next == "its Cyrillic look-alike" {
			next = "xn--pi-6kc.example.com"
		}
		for i, name := range names {
			if name == old {
				names[i] = next
			}
		}
	case strings.HasPrefix(edit, "request the allowlist "):
		return w.approvalNetworkRequest(2, strings.Fields(strings.TrimPrefix(edit, "request the allowlist ")))
	case strings.HasPrefix(edit, "request offline at policy version "):
		n, err := strconv.Atoi(strings.TrimPrefix(edit, "request offline at policy version "))
		if err != nil {
			return err
		}
		return w.approvalNetworkRequest(n, nil)
	default:
		return fmt.Errorf("unknown edit %q", edit)
	}
	sort.Strings(names)
	a.allow = names
	return w.approvalNetworkFile(names)
}

func approvalNetworkDefect(s map[string]any, defect string) error {
	p := s["policy"].(map[string]any)
	n := p["network"].(map[string]any)
	bad := map[string]string{
		"an allowlist name in upper case": "b.EXAMPLE", "a wildcard allowlist name": "a.*.example", "a single-label allowlist name": "localhost", "a literal address in the allowlist": "192.0.2.1", "a hexadecimal final allowlist label": "host.0x7f", "an empty allowlist label": "a..example", "an allowlist label of 64 bytes": strings.Repeat("b", 64) + ".example", "an allowlist name of 254 bytes": approvalLongName(62),
	}
	if name, ok := bad[defect]; ok {
		list := []string{"0.example", name, "z.example"}
		sort.Strings(list)
		n["allow"] = list
		return nil
	}
	switch defect {
	case "allowlist names out of byte order":
		n["allow"] = []string{"registry.example.org", "api.example.com"}
	case "a repeated allowlist name":
		n["allow"] = []string{"api.example.com", "api.example.com"}
	case "an empty allowlist":
		n["allow"] = []string{}
	case "no allowlist under allowlist-v1":
		delete(n, "allow")
	case "mode offline under allowlist-v1":
		n["mode"] = "offline"
	case "policy version 1 under allowlist-v1":
		p["version"] = 1
	case "policy version 3 under allowlist-v1":
		p["version"] = 3
	case "no proxy image under allowlist-v1":
		delete(s, "proxy")
	case "a proxy image without a digest":
		s["proxy"] = map[string]any{"image": "busybox:latest"}
	case "a proxy image with a short digest":
		s["proxy"] = map[string]any{"image": "busybox@sha256:" + strings.Repeat("a", 63)}
	case "a proxy image with an upper-case digest":
		s["proxy"] = map[string]any{"image": strings.Replace(busyboxFixturePin, strings.Split(busyboxFixturePin, "@sha256:")[1], strings.ToUpper(strings.Split(busyboxFixturePin, "@sha256:")[1]), 1)}
	case "a proxy image digest with no name":
		s["proxy"] = map[string]any{"image": "@sha256:" + strings.Repeat("a", 64)}
	case "profile offline-v1 with mode allowlist":
		s["profile"] = "offline-v1"
		delete(s, "proxy")
	case "an allowlist under offline-v1":
		s["profile"] = "offline-v1"
		n["mode"] = "offline"
		delete(s, "proxy")
	case "a proxy image under offline-v1":
		s["profile"] = "offline-v1"
		n["mode"] = "offline"
		delete(n, "allow")
	case "policy version 3 under offline-v1":
		s["profile"] = "offline-v1"
		p["version"] = 3
		n["mode"] = "offline"
		delete(n, "allow")
		delete(s, "proxy")
	case "an unknown network profile":
		s["profile"] = "unknown-v1"
	default:
		return fmt.Errorf("unknown defect %q", defect)
	}
	return nil
}

func (w *world) approvalOtherNetworkFingerprint(which string) error {
	if which == "an empty flag value" || which == "a malformed fingerprint" {
		value := ""
		if which == "a malformed fingerprint" {
			value = "not-a-fingerprint"
		}
		return w.runDoctor(w.env(), "approve", "--json", "--fingerprint="+value)
	}
	s := approvalClone(w.approval.settings)
	p := s["policy"].(map[string]any)
	n := p["network"].(map[string]any)
	switch which {
	case "the same checkout offline at policy version 1", "the same checkout offline at policy version 2":
		s["profile"] = "offline-v1"
		delete(s, "proxy")
		n["mode"] = "offline"
		delete(n, "allow")
		p["version"] = 1
		if strings.HasSuffix(which, "2") {
			p["version"] = 2
		}
	case "the list without registry.example.org":
		n["allow"] = []string{"api.example.com"}
	case "the list with other.example added":
		n["allow"] = []string{"api.example.com", "other.example", "registry.example.org"}
	case "the list with no proxy member":
		delete(s, "proxy")
	case "the list bound to another proxy image digest":
		s["proxy"] = map[string]any{"image": "busybox@sha256:" + strings.Repeat("a", 64)}
	default:
		return fmt.Errorf("unknown settings %q", which)
	}
	return w.runDoctor(w.env(), "approve", "--json", "--fingerprint", "sha256:"+approvalHash(s))
}

func (w *world) approvalExactDifferences(pointers string) error {
	view, err := w.approvalView()
	if err != nil {
		return err
	}
	rows, ok := view["differences"].([]any)
	if !ok {
		return fmt.Errorf("missing differences")
	}
	want := strings.Fields(pointers)
	if len(rows) != len(want) {
		return fmt.Errorf("differences = %v, want %v", rows, want)
	}
	for i, path := range want {
		row, ok := rows[i].(map[string]any)
		if !ok || row["path"] != path {
			return fmt.Errorf("difference %d is not %s", i, path)
		}
		if err := w.approvalDifference(path); err != nil {
			return err
		}
	}
	return nil
}
func (w *world) approvalBeforePrompt() (string, error) {
	text, _, ok := strings.Cut(w.approval.beforeReview, "y/N")
	if !ok || strings.Count(w.stdout, "y/N") != 1 {
		return "", fmt.Errorf("expected one complete review before input\n%s", w.report())
	}
	return text, nil
}
func (w *world) approvalReviewDifferences(pointers string) error {
	text, err := w.approvalBeforePrompt()
	if err != nil {
		return err
	}
	for _, path := range strings.Fields(pointers) {
		line := path + ": " + string(approvalJSON(approvalAt(w.approval.prior, path))) + " -> " + string(approvalJSON(approvalAt(w.approval.settings, path)))
		_, rest, found := strings.Cut(text, line)
		if !found {
			return fmt.Errorf("review lacks ordered difference %s\n%s", line, w.report())
		}
		text = rest
	}
	return nil
}
func approvalOrdered(text string, table *godog.Table) error {
	for _, row := range table.Rows[1:] {
		_, rest, ok := strings.Cut(text, row.Cells[0].Value)
		if !ok {
			return fmt.Errorf("missing ordered text %q", row.Cells[0].Value)
		}
		text = rest
	}
	return nil
}
func (w *world) approvalListedNames(text string) error {
	var got []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if name, ok := strings.CutPrefix(line, "network.allow: "); ok {
			got = append(got, name)
		}
	}
	if !reflect.DeepEqual(got, w.approval.allow) {
		return fmt.Errorf("displayed names = %v, want %v", got, w.approval.allow)
	}
	return nil
}
func (w *world) approvalHostUnchanged() error {
	got, err := doctorTree(w.dir)
	if err != nil {
		return err
	}
	want := make(map[string]string)
	prefix := w.doctor.checkout + string(filepath.Separator)
	for path, value := range w.approval.before {
		if path != w.doctor.checkout && !strings.HasPrefix(path, prefix) {
			want[path] = value
		}
	}
	for path := range got {
		if path == w.doctor.checkout || strings.HasPrefix(path, prefix) {
			delete(got, path)
		}
	}
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("host state changed")
	}
	return nil
}
