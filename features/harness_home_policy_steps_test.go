package features

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/cucumber/godog"
)

type homePolicyObservation struct {
	input, rule, subject string
	code                 int
	output               map[string]any
}

type homeWorld struct {
	version int
	policy  []homePolicyObservation
}

func (w *world) homeSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a home policy fixture using version (\d+) and no credential grants$`, func(version int) error {
		w.home = &homeWorld{version: version}
		return w.isolatePolicy()
	})
	sc.Step(`^file-only doctor inspects an omitted home, a volume home and the malformed home requests$`, w.homePolicyInspect)
	sc.Step(`^the volume request succeeds with exactly the normalized home storage and the omitted request keeps its old output$`, w.homePolicyAccepted)
	sc.Step(`^malformed home requests have the specified rules and subjects without state or engine access$`, w.homePolicyRefused)
}

func (w *world) homePolicyInspect() error {
	inputs := []homePolicyObservation{
		{},
		{input: "home: {storage: volume}\n"},
		{input: "home: null\n", rule: "policy.type", subject: "/home"},
		{input: "home: []\n", rule: "policy.type", subject: "/home"},
		{input: "home: volume\n", rule: "policy.type", subject: "/home"},
		{input: "home: {}\n", rule: "policy.required", subject: "/home/storage"},
		{input: "home: {storage: null}\n", rule: "policy.type", subject: "/home/storage"},
		{input: "home: {storage: tmpfs}\n", rule: "policy.home_storage", subject: "/home/storage"},
		{input: "home: {storage: host}\n", rule: "policy.home_storage", subject: "/home/storage"},
		{input: "home: {storage: volume, source: /outside}\n", rule: "policy.unknown_key", subject: "/home/source"},
		{input: "home: {storage: volume}\nhome: {storage: volume}\n", rule: "policy.duplicate_key", subject: "/home"},
		{input: "home: {storage: volume, storage: volume}\n", rule: "policy.duplicate_key", subject: "/home/storage"},
	}
	for _, input := range inputs {
		data := fmt.Sprintf("version: %d\nmanifest: devcontainer.json\n%s", w.home.version, input.input)
		if err := os.WriteFile(filepath.Join(w.dir, "itos-container.yaml"), []byte(data), 0600); err != nil {
			return err
		}
		before, err := w.policyFiles()
		if err != nil {
			return err
		}
		if err = w.runWith(w.env(), "doctor --policy-only --json"); err != nil {
			return err
		}
		input.code, input.output = w.exit, nil
		input.output, err = w.json()
		if err != nil {
			return err
		}
		w.home.policy = append(w.home.policy, input)
		after, err := w.policyFiles()
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(before, after) {
			return fmt.Errorf("file-only doctor changed checkout or isolated host state")
		}
	}
	return nil
}

func (w *world) homePolicyAccepted() error {
	for i, observation := range w.home.policy[:2] {
		expected := map[string]any{"version": float64(w.home.version), "manifest": "devcontainer.json", "network": map[string]any{"mode": "offline"}}
		if i == 1 {
			expected["home"] = map[string]any{"storage": "volume"}
		}
		out := observation.output
		problems, ok := out["problems"].([]any)
		if observation.code != 0 || out["schema"] != float64(1) || out["ok"] != true || out["scope"] != "policy" || !ok || len(problems) != 0 || !reflect.DeepEqual(out["policy"], expected) {
			return fmt.Errorf("home input %q: exit %d, output %s; expected normalized policy %s", observation.input, observation.code, approvalJSON(out), approvalJSON(expected))
		}
	}
	return nil
}

func (w *world) homePolicyRefused() error {
	for _, observation := range w.home.policy[2:] {
		out := observation.output
		problems, ok := out["problems"].([]any)
		if observation.code != 2 || out["ok"] != false || !ok || len(problems) != 1 || out["policy"] != nil {
			return fmt.Errorf("malformed home %q: exit %d output %s", observation.input, observation.code, approvalJSON(out))
		}
		problem := object(problems[0])
		if problem["rule"] != observation.rule || problem["path"] != observation.subject || problem["exit"] != float64(2) || problem["file"] != "itos-container.yaml" {
			return fmt.Errorf("malformed home %q: expected %s at %s; got %s", observation.input, observation.rule, observation.subject, approvalJSON(problem))
		}
		for _, field := range []string{"message", "fix"} {
			if text, _ := problem[field].(string); strings.TrimSpace(text) == "" {
				return fmt.Errorf("home diagnostic lacks %s", field)
			}
		}
	}
	if w.policy.connections.Load() != 0 {
		return fmt.Errorf("file-only doctor contacted the engine")
	}
	return nil
}
