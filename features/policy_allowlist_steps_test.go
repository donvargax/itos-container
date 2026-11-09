package features

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// Steps for slice-25's host-name allowlist. They only write fixtures or point a
// child at the recorder; none of them calls the product or asserts on it.
func (w *world) policyAllowlistSteps(sc *godog.ScenarioContext) {
	// Version 2, manifest a.json and allowlist mode wrap every generated list.
	allowlist := func(entries ...string) error {
		data := "version: 2\nmanifest: a.json\nnetwork:\n  mode: allowlist\n  allow:\n"
		for _, entry := range entries {
			data += "    - " + entry + "\n"
		}
		return os.WriteFile(filepath.Join(w.dir, "itos-container.yaml"), []byte(data), 0o600)
	}
	sc.Step(`^the policy file allows one host name of (\d+) distinct characters$`, func(count int) error {
		var name strings.Builder
		for i := range count {
			name.WriteRune(rune(0x4E00 + i))
		}
		return allowlist(name.String() + ".example")
	})
	sc.Step(`^the policy file allows "([^"]*)" padded with soft hyphens to (\d+) bytes$`, func(name string, size int) error {
		const softHyphen = "\u00AD"
		padding := size - len(name)
		if padding < 0 || padding%len(softHyphen) != 0 {
			return fmt.Errorf("%q cannot be padded with soft hyphens to exactly %d bytes", name, size)
		}
		return allowlist(`"` + name + strings.Repeat(softHyphen, padding/len(softHyphen)) + `"`)
	})
	sc.Step(`^the policy file allows (\d+) generated host names$`, func(count int) error {
		entries := make([]string, count)
		for i := range entries {
			entries[i] = "n" + strconv.Itoa(i) + ".example"
		}
		return allowlist(entries...)
	})
	sc.Step(`^the manifest inspection policy contains:$`, func(doc *godog.DocString) error {
		return os.WriteFile(filepath.Join(w.manifest.checkout, "itos-container.yaml"), []byte(doc.Content), 0o600)
	})
	sc.Step(`^the checkout policy contains:$`, func(doc *godog.DocString) error {
		return os.WriteFile(filepath.Join(w.doctor.checkout, "itos-container.yaml"), []byte(doc.Content), 0o600)
	})
	// manifestEnv removes the DOCKER_HOST policyEnv set, so a --manifest-only run
	// could never reach the recorder unless this step gives it back.
	sc.Step(`^manifest inspection is given the recording engine endpoint$`, func() error {
		if w.policy == nil || w.manifest == nil {
			return fmt.Errorf("the recording engine endpoint and the manifest checkout must exist first")
		}
		w.manifest.env["DOCKER_HOST"] = w.policy.env["DOCKER_HOST"]
		return nil
	})
}
