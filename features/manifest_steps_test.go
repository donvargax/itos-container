package features

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/cucumber/godog"
)

const manifestImage = "registry.example/worker@sha256:1111111111111111111111111111111111111111111111111111111111111111"
const manifestBytes = `{"image":"` + manifestImage + `"}`

type manifestWorld struct {
	checkout, host, recorder, selected string
	env                                map[string]string
	label                              *string
	metadata                           bool
	snapshot                           map[string]string
}

func (w *world) manifestSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a manifest inspection checkout$`, w.manifestCheckout)
	sc.Step(`^the manifest is located only at "([^"]*)"$`, w.moveManifest)
	sc.Step(`^the policy selects manifest "([^"]*)"$`, w.selectManifest)
	sc.Step(`^the selected manifest does not exist$`, func() error { return os.RemoveAll(w.manifestPath()) })
	sc.Step(`^valid decoy manifests exist at "([^"]*)" and "([^"]*)"$`, func(a, b string) error { return w.decoyManifests(a, b) })
	sc.Step(`^valid decoy manifests exist at "([^"]*)", "([^"]*)" and "([^"]*)"$`, func(a, b, c string) error { return w.decoyManifests(a, b, c) })
	sc.Step(`^a valid decoy manifest exists in the checkout's parent directory$`, func() error {
		return writeManifestFixture(filepath.Join(w.dir, ".devcontainer.json"), []byte(manifestBytes), 0o600)
	})
	sc.Step(`^the unselected conventional manifest exists with "([^"]*)"$`, w.unselectedManifest)
	sc.Step(`^the manifest text is:$`, func(doc *godog.DocString) error { return w.manifestText(doc.Content) })
	sc.Step(`^the manifest inspection input "([^"]*)" has the exact text:$`, func(source string, doc *godog.DocString) error {
		if source != "manifest" {
			return fmt.Errorf("unknown manifest fixture source %q", source)
		}
		return w.manifestText(doc.Content)
	})
	sc.Step(`^the manifest member "([^"]*)" has JSON value (.*)$`, w.manifestMember)
	sc.Step(`^the manifest has no "([^"]*)" member$`, func(key string) error { return w.manifestMember(key, "") })
	sc.Step(`^the manifest candidate has the layout "([^"]*)"$`, w.manifestLayout)
	sc.Step(`^a checkout directory "([^"]*)" exists$`, func(name string) error { return os.MkdirAll(filepath.Join(w.manifest.checkout, name), 0o700) })
	sc.Step(`^the checkout workspace entry "([^"]*)" has layout "([^"]*)"$`, w.workspaceLayout)
	sc.Step(`^host execution sentinels are installed$`, w.manifestSentinels)
	sc.Step(`^inspection has no host side effects$`, w.manifestUnchanged)
	sc.Step(`^the supplied devcontainer.metadata label is absent$`, func() { w.manifest.label = nil })
	sc.Step(`^the supplied devcontainer.metadata label is:$`, func(doc *godog.DocString) { w.manifest.label = &doc.Content })
	sc.Step(`^the manifest inspection contract runs with supplied metadata in (JSON|plain) mode$`, w.runManifestContract)
	sc.Step(`^its JSON output reports a valid manifest at "([^"]*)"$`, w.validManifest)
	sc.Step(`^its JSON value at "([^"]*)" is:$`, w.manifestJSONValue)
	sc.Step(`^its JSON output reports "([^"]*)" at "([^"]*)" with exit (\d+)$`, w.manifestProblem)
}

func (w *world) manifestPath() string { return filepath.Join(w.manifest.checkout, w.manifest.selected) }

func writeManifestFixture(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, mode)
}

func (w *world) selectManifest(path string) error {
	w.manifest.selected = path
	data, err := json.Marshal(map[string]any{"version": 1, "manifest": path, "network": map[string]string{"mode": "offline"}})
	if err != nil {
		return err
	}
	return writeManifestFixture(filepath.Join(w.manifest.checkout, "itos-container.yaml"), data, 0o600)
}

func (w *world) moveManifest(path string) error {
	data, err := os.ReadFile(w.manifestPath())
	if err != nil {
		return err
	}
	if err := os.Remove(w.manifestPath()); err != nil {
		return err
	}
	if err := w.selectManifest(path); err != nil {
		return err
	}
	return w.manifestText(string(data))
}

func (w *world) manifestText(text string) error {
	return writeManifestFixture(w.manifestPath(), []byte(text), 0o600)
}

func (w *world) manifestMember(key, value string) error {
	data, err := os.ReadFile(w.manifestPath())
	if err != nil {
		return err
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(data, &members); err != nil {
		return err
	}
	if value == "" {
		delete(members, key)
	} else {
		members[key] = json.RawMessage(value)
	}
	data, err = json.Marshal(members)
	if err != nil {
		return err
	}
	return w.manifestText(string(data))
}

func (w *world) decoyManifests(paths ...string) error {
	for _, path := range paths {
		if err := writeManifestFixture(filepath.Join(w.manifest.checkout, path), []byte(manifestBytes), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) unselectedManifest(contents string) error {
	path := ".devcontainer.json"
	if w.manifest.selected == path {
		path = ".devcontainer/devcontainer.json"
	}
	data := manifestBytes
	if contents == "malformed JSON" {
		data = "{"
	}
	return writeManifestFixture(filepath.Join(w.manifest.checkout, path), []byte(data), 0o600)
}

func (w *world) validManifest(path string) error {
	object, err := w.json()
	if err != nil {
		return err
	}
	manifest, _ := object["manifest"].(map[string]any)
	problems, ok := object["problems"].([]any)
	if object["schema"] != float64(1) || object["ok"] != true || !ok || len(problems) != 0 || manifest["valid"] != true || manifest["path"] != path {
		return fmt.Errorf("expected a valid manifest at %q with schema 1 and no problems\n%s", path, w.report())
	}
	if !w.manifest.metadata && object["scope"] != "manifest" {
		return fmt.Errorf("expected manifest scope\n%s", w.report())
	}
	return nil
}

func (w *world) manifestJSONValue(pointer string, doc *godog.DocString) error {
	object, err := w.json()
	if err != nil {
		return err
	}
	var actual any = object
	for _, segment := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		key := strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~")
		parent, ok := actual.(map[string]any)
		if !ok {
			return fmt.Errorf("no object parent at %q\n%s", pointer, w.report())
		}
		actual, ok = parent[key]
		if !ok {
			return fmt.Errorf("missing JSON member at %q\n%s", pointer, w.report())
		}
	}
	var expected any
	if err := json.Unmarshal([]byte(doc.Content), &expected); err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, expected) {
		return fmt.Errorf("JSON value at %q is %#v, expected %#v\n%s", pointer, actual, expected, w.report())
	}
	return nil
}

func (w *world) manifestProblem(rule, path string, exit int) error {
	object, err := w.json()
	if err != nil {
		return err
	}
	manifest, _ := object["manifest"].(map[string]any)
	_, effective := manifest["effective"]
	if object["schema"] != float64(1) || object["ok"] != false || manifest["valid"] != false || effective {
		return fmt.Errorf("expected manifest refusal without effective settings\n%s", w.report())
	}
	problems, _ := object["problems"].([]any)
	for _, entry := range problems {
		p, _ := entry.(map[string]any)
		if p["rule"] != rule || p["path"] != path || p["exit"] != float64(exit) {
			continue
		}
		message, _ := p["message"].(string)
		fix, _ := p["fix"].(string)
		if message == "" || fix == "" {
			return fmt.Errorf("incomplete manifest problem\n%s", w.report())
		}
		if w.manifest.metadata {
			if p["image"] != manifestImage || p["label"] != "devcontainer.metadata" {
				return fmt.Errorf("missing image label subject\n%s", w.report())
			}
			if _, exists := p["file"]; exists {
				return fmt.Errorf("metadata problem attributed to a file\n%s", w.report())
			}
		} else if p["file"] != w.manifest.selected {
			return fmt.Errorf("missing selected manifest subject\n%s", w.report())
		}
		return nil
	}
	return fmt.Errorf("missing %s at %q with exit %d\n%s", rule, path, exit, w.report())
}
