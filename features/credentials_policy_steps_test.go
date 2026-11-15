package features

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

// These fixtures generate policy text only; no named host source is touched.
func (w *world) credentialPolicySteps(sc *godog.ScenarioContext) {
	var generated string
	sc.Step(`^a credential policy source repeats "([^"]*)" to exactly (\d+) bytes$`, func(text string, size int) error {
		if text == "" || size < 1 {
			return fmt.Errorf("source generation needs nonempty text and an absolute path")
		}
		generated = "/" + strings.Repeat(text, (size-1)/len(text)) + strings.Repeat("a", (size-1)%len(text))
		data, err := json.Marshal(map[string]any{
			"version": 3, "manifest": "a.json",
			"credentials": []map[string]string{{"kind": "claude-token", "source": generated}},
		})
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(w.dir, "itos-container.yaml"), data, 0o600)
	})
	sc.Step(`^its policy JSON preserves the generated credential source exactly$`, func() error {
		data, err := json.Marshal(map[string]any{
			"version": 3, "manifest": "a.json", "network": map[string]string{"mode": "offline"},
			"credentials": []map[string]string{{"kind": "claude-token", "source": generated}},
		})
		if err != nil {
			return err
		}
		return w.effectivePolicy(&godog.DocString{Content: string(data)})
	})
}
