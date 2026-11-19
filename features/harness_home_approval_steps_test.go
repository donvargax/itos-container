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

func (w *world) homeApprovalSteps(sc *godog.ScenarioContext) {
	var expected map[string]any
	var fingerprint string
	var inspected doctorResult
	var checkout, host map[string]runEntry
	sc.Step(`^a home approval fixture with an independently seeded legacy approval and a requested volume home$`, func() error {
		if err := w.approvalCheckout(); err != nil {
			return err
		}
		if err := w.approvalSeed("1", false); err != nil {
			return err
		}
		expected = approvalClone(w.approval.settings)
		expected["profile"] = "offline-home-v1"
		expected["home"] = map[string]any{"storage": "volume", "target": "/home/itos", "lifetime": "project", "mode": "0700"}
		fingerprint = "sha256:" + approvalHash(expected)
		if err := os.WriteFile(filepath.Join(w.doctor.checkout, "itos-container.yaml"), []byte("version: 1\nmanifest: .devcontainer/devcontainer.json\nhome: {storage: volume}\n"), 0600); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(w.doctor.env["HOME"], "slice-34-sentinel"), []byte("synthetic private home data"), 0600); err != nil {
			return err
		}
		var err error
		checkout, err = runTree(w.doctor.checkout)
		if err == nil {
			host, err = runTree(w.doctor.env["HOME"])
		}
		return err
	})
	sc.Step(`^ordinary doctor and terminal approve with consent inspect the requested home$`, func() error {
		if err := w.runDoctor(w.env(), "doctor", "--json"); err != nil {
			return err
		}
		inspected = doctorResult{stdout: w.stdout, stderr: w.stderr, exit: w.exit}
		return w.approvalTerminal("plain", "y", "", "")
	})
	sc.Step(`^both show the exact home settings and changed fingerprint and approval records the independently expected home profile$`, func() error {
		var report map[string]any
		if json.Unmarshal([]byte(inspected.stdout), &report) != nil {
			return fmt.Errorf("doctor did not return JSON: %s", inspected.stdout)
		}
		a := object(report["approval"])
		if inspected.exit != 0 || a["status"] != "changed" || a["fingerprint"] != fingerprint || !reflect.DeepEqual(a["effective"], expected) {
			return fmt.Errorf("doctor did not review the requested home: exit %d, %s", inspected.exit, inspected.stdout)
		}
		if w.exit != 0 || !strings.Contains(w.approval.beforeReview, fingerprint) && !strings.Contains(w.approval.beforeReview, `"profile":"offline-home-v1"`) {
			return fmt.Errorf("home approval did not review and succeed: %d %s %s", w.exit, w.stdout, w.stderr)
		}
		data, err := os.ReadFile(w.approval.record)
		if err != nil {
			return err
		}
		want := approvalJSON(map[string]any{"schema": 1, "project": w.approval.project, "generation": "2", "fingerprint": fingerprint, "settings": expected})
		if string(data) != string(want) {
			return fmt.Errorf("home approval record differs: %s", data)
		}
		return nil
	})
	sc.Step(`^review explains retention and project scope and sorted home differences before consent$`, func() error {
		review := w.approval.beforeReview
		for _, text := range []string{"/home/itos", "0700", w.approval.uid + ":" + w.approval.gid, "checkout", "exit", "stop", "host home", "reset", "delete", "/home:", "/profile:", "y/N"} {
			if !strings.Contains(review, text) {
				return fmt.Errorf("home review omits %q: %s", text, review)
			}
		}
		if strings.Index(review, "/home:") > strings.Index(review, "/profile:") || strings.Index(review, "/profile:") > strings.Index(review, "y/N") {
			return fmt.Errorf("home review differences are not sorted before consent")
		}
		return nil
	})
	sc.Step(`^no home volume or worker is created and checkout and host-home sentinels are unchanged$`, func() error {
		for _, req := range w.doctor.engine.requests() {
			if req.Method != "GET" && req.Method != "HEAD" {
				return fmt.Errorf("review mutated engine: %+v", req)
			}
		}
		after, err := runTree(w.doctor.checkout)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(after, checkout) {
			return fmt.Errorf("review changed checkout")
		}
		after, err = runTree(w.doctor.env["HOME"])
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(after, host) {
			return fmt.Errorf("review changed host-home sentinel tree")
		}
		return nil
	})
}
