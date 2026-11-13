package features

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

// The effective root and the child's working directory are separate inputs.
// These steps move only one of them, so a policy read from the wrong one shows.
func (w *world) credentialEffectiveRootSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the credential command runs in another directory with its own policy and private source$`, w.credentialOtherDirectory)
	sc.Step(`^the credential fixture checkout has no policy file$`, func() error {
		return os.Remove(filepath.Join(w.manifest.checkout, "itos-container.yaml"))
	})
	sc.Step(`^the credential effective root is "([^"]*)"$`, w.credentialEffectiveRoot)
	sc.Step(`^the credential boundary problem is "([^"]*)" with exit (\d+) at the effective root and no grant is reported$`, w.credentialRootRefusal)
}

// The other directory is a sibling of the checkout, outside sources and state.
// Its request is valid and private there, so only its origin can refuse it.
func (w *world) credentialOtherDirectory() error {
	dir := filepath.Join(w.dir, "elsewhere")
	token := filepath.Join(dir, "token")
	if err := w.credentialMarkerFile(token); err != nil {
		return err
	}
	policy, err := json.Marshal(map[string]any{
		"version": 3, "manifest": w.manifest.selected, "network": map[string]string{"mode": "offline"},
		"credentials": []credentialGrant{{Kind: "claude-token", Source: token}},
	})
	if err != nil {
		return err
	}
	w.credentials.cwd = dir
	return os.WriteFile(filepath.Join(dir, "itos-container.yaml"), policy, 0o600)
}

func (w *world) credentialEffectiveRoot(kind string) error {
	root := ""
	switch kind {
	case "relative":
		// The name resolves to the checkout from the working directory, so a
		// refusal cannot come from the name merely being unused.
		root = filepath.Join("..", filepath.Base(w.manifest.checkout))
	case "absent":
		root = filepath.Join(w.dir, "absent-root")
	case "symbolic link":
		root = filepath.Join(w.dir, "checkout-link")
		if err := os.Symlink(w.manifest.checkout, root); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown effective root %q", kind)
	}
	w.manifest.env["ITOS_ROOT"] = root
	return nil
}

func (w *world) credentialRootRefusal(rule string, exit int) error {
	obj, err := w.json()
	if err != nil {
		return err
	}
	if obj["schema"] != float64(1) || obj["scope"] != "credentials" || obj["ok"] != false || len(obj) != 5 {
		return fmt.Errorf("incorrect credential envelope\n%s", w.report())
	}
	if grants, ok := obj["credentials"].([]any); !ok || len(grants) != 0 {
		return fmt.Errorf("a request was reported for a root whose policy was never read\n%s", w.report())
	}
	problems, _ := obj["problems"].([]any)
	if len(problems) != 1 {
		return fmt.Errorf("expected one boundary problem\n%s", w.report())
	}
	p := object(problems[0])
	message, _ := p["message"].(string)
	fix, _ := p["fix"].(string)
	if p["rule"] != rule || p["exit"] != float64(exit) || p["path"] != w.manifest.env["ITOS_ROOT"] || strings.TrimSpace(message) == "" || strings.TrimSpace(fix) == "" {
		return fmt.Errorf("wrong boundary problem, want %s exit %d at %q\n%s", rule, exit, w.manifest.env["ITOS_ROOT"], w.report())
	}
	return nil
}
