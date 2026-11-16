package features

import (
	"encoding/json"
	"fmt"
	"github.com/cucumber/godog"
	"os"
	"strconv"
	"strings"
	"unicode"
)

func (w *world) credentialApprovalNativeSteps(sc *godog.ScenarioContext) {
	sc.Step(`^credential doctor inspects before and after "([^"]*)"$`, func(edit string) error {
		if err := w.runDoctor(w.env(), "doctor", "--json"); err != nil {
			return err
		}
		w.approval.results = append(w.approval.results, doctorResult{stdout: w.stdout, stderr: w.stderr, exit: w.exit})
		if err := w.credentialApprovalEdit(edit); err != nil {
			return err
		}
		if err := w.runDoctor(w.env(), "doctor", "--json"); err != nil {
			return err
		}
		w.approval.results = append(w.approval.results, doctorResult{stdout: w.stdout, stderr: w.stderr, exit: w.exit})
		return nil
	})
	sc.Step(`^the credential approval policy version is (\d+)$`, func(version int) error {
		w.approval.policyVersion = version
		if err := w.credentialApprovalPolicy(); err != nil {
			return err
		}
		if err := w.approvalOracle(); err != nil {
			return err
		}
		return w.approvalCapture()
	})
	sc.Step(`^the legacy approval bytes and generation are unchanged$`, w.approvalRecordUnchanged)
	sc.Step(`^the no-grant profile is "([^"]*)" with no credentials member$`, func(profile string) error {
		v, err := w.approvalView()
		if err != nil {
			return err
		}
		s := object(v["effective"])
		_, present := s["credentials"]
		if s["profile"] != profile || present {
			return fmt.Errorf("unexpected no-grant settings")
		}
		return nil
	})
	sc.Step(`^terminal approval is attempted and doctor then inspects the version 3 no-grant record$`, func() error {
		if err := w.credentialApprovalTerminal("plain", "yes", "", ""); err != nil {
			return err
		}
		w.approval.results = []doctorResult{{stdout: w.stdout, stderr: w.stderr, exit: w.exit}}
		if err := w.runDoctor(w.env(), "doctor", "--json"); err != nil {
			return err
		}
		w.approval.results = append(w.approval.results, doctorResult{stdout: w.stdout, stderr: w.stderr, exit: w.exit})
		return nil
	})
	sc.Step(`^both results report matching no-grant approval at generation "([^"]*)" with the independent fingerprint$`, func(g string) error {
		if len(w.approval.results) != 2 {
			return fmt.Errorf("missing approval/doctor results")
		}
		last := doctorResult{stdout: w.stdout, stderr: w.stderr, exit: w.exit}
		defer func() { w.stdout, w.stderr, w.exit = last.stdout, last.stderr, last.exit }()
		for _, r := range w.approval.results {
			w.stdout, w.stderr, w.exit = r.stdout, r.stderr, r.exit
			if err := w.exitsWith(0); err != nil {
				return err
			}
			if err := w.approvalStatus("matching"); err != nil {
				return err
			}
			if err := w.approvalCandidate(); err != nil {
				return err
			}
		}
		return w.approvalStored(g)
	})
	sc.Step(`^a seeded credential approval with defect "([^"]*)" and its recomputed fingerprint$`, w.credentialApprovalDefect)
	sc.Step(`^a seeded credential approval at generation "([^"]*)" with target "([^"]*)"$`, func(g, target string) error {
		if err := w.approvalSeed(g, false); err != nil {
			return err
		}
		grants := w.approval.prior["credentials"].([]any)
		object(grants[0])["target"] = target
		return w.credentialApprovalSeedSettings(w.approval.prior, g)
	})
	sc.Step(`^the credential review shows complete settings and the exact profile restrictions before one y/N prompt$`, func() error {
		text, err := w.approvalBeforePrompt()
		if err != nil {
			return err
		}
		if !strings.Contains(text, "Effective settings:") {
			return fmt.Errorf("missing effective-settings label")
		}
		if err := approvalDisplayedJSONLine(text, w.approval.settings); err != nil {
			return err
		}
		if !strings.Contains(text, w.credentialApprovalRestriction()) {
			return fmt.Errorf("credential restriction line differs")
		}
		return nil
	})
	sc.Step(`^the credential review lists exactly the sorted kinds sources and fixed targets before consent$`, func() error {
		text, err := w.approvalBeforePrompt()
		if err != nil {
			return err
		}
		return w.credentialApprovalLines(text)
	})
	sc.Step(`^the credential review gives every disclosure for "([^"]*)" in the specified order before consent$`, func(network string) error {
		text, err := w.approvalBeforePrompt()
		if err != nil {
			return err
		}
		return credentialApprovalDisclosure(text, network, true)
	})
	sc.Step(`^the credential review describes exactly the current request including its applicable disclosure$`, func() error {
		text, err := w.approvalBeforePrompt()
		if err != nil {
			return err
		}
		return w.credentialApprovalDescribe(text, true)
	})
	sc.Step(`^the plain approval result lists exactly the current grants and complete credential disclosure$`, func() error { return w.credentialApprovalDescribe(w.stdout, true) })
	sc.Step(`^credential consent writes only approval state and reads no source data$`, func() error {
		if err := w.credentialApprovalNoAccess(); err != nil {
			return err
		}
		return w.approvalOnlyState()
	})
	sc.Step(`^credential observations read no source data and leak no synthetic marker$`, w.credentialApprovalNoAccess)
	sc.Step(`^credential approve supplies fingerprint "([^"]*)" with "([^"]*)" in "([^"]*)"$`, w.credentialApprovalFingerprint)
	sc.Step(`^the credential refusal names terminal "itos container approve" without offering a fingerprint route$`, func() error {
		text := w.stderr
		if json.Valid([]byte(w.stdout)) {
			obj, err := w.json()
			if err != nil {
				return err
			}
			text = string(approvalJSON(obj["problems"]))
		}
		if !strings.Contains(text, "itos container approve") || !strings.Contains(strings.ToLower(text), "terminal") || strings.Contains(text, "--fingerprint") {
			return fmt.Errorf("credential fix offers the wrong consent route")
		}
		return nil
	})
	sc.Step(`^the credential terminal review is displayed then the fixture applies "([^"]*)" before answering yes$`, func(edit string) error { return w.credentialApprovalTerminal("plain", "yes", edit, "") })
	sc.Step(`^credential source "([^"]*)" is changed to "([^"]*)"$`, func(name, defect string) error {
		if err := w.credentialApprovalSourceEdit(name, defect); err != nil {
			return err
		}
		return w.approvalCapture()
	})
	sc.Step(`^the credential readiness command "([^"]*)" is invoked$`, w.credentialApprovalInvoke)
	sc.Step(`^credential readiness reports "([^"]*)" at "([^"]*)" with exit (\d+) before consent$`, w.credentialApprovalProblem)
	sc.Step(`^the requested grants remain visible without a usable approval candidate$`, w.credentialApprovalRequested)
	sc.Step(`^the credential refusal publishes neither approval nor frozen input$`, w.credentialApprovalNoPublication)
	sc.Step(`^the deliberately edited credential fixture is otherwise unchanged$`, w.credentialApprovalReadOnly)
	sc.Step(`^the fixture will make claude-a group-readable while image inspection is paused$`, func() { w.doctor.engine.paused = true; w.approval.condition = "credential engine barrier" })
	sc.Step(`^both credential sources have mode "([^"]*)"$`, func(mode string) error {
		n, err := strconv.ParseUint(mode, 8, 32)
		if err != nil {
			return err
		}
		for _, g := range w.credentialApproval.grants {
			if err := os.Chmod(g.Source, os.FileMode(n)); err != nil {
				return err
			}
		}
		return w.credentialApprovalCapturePrivate()
	})
	sc.Step(`^the credential data-access observer and marker scanner have positive controls$`, w.credentialApprovalMarkerControl)
	sc.Step(`^credential readiness succeeds with the complete independently calculated candidate$`, func() error {
		if err := w.exitsWith(0); err != nil {
			return err
		}
		return w.approvalCandidate()
	})
	sc.Step(`^the product opens and reads no credential source data$`, w.credentialApprovalNoAccess)
	sc.Step(`^no synthetic marker occurs in command output engine traffic checkout or protected state$`, w.credentialApprovalScan)
	sc.Step(`^only the command's permitted approval publication occurs$`, func() error {
		if strings.Contains(w.approval.mode, "doctor") {
			return w.credentialApprovalReadOnly()
		}
		return w.approvalOnlyState()
	})
	sc.Step(`^the prior and requested credential source filenames contain "([^"]*)"$`, w.credentialApprovalLiteralPaths)
	sc.Step(`^credential approval reviews the changed path at a terminal and doctor then inspects as JSON$`, func() error {
		if err := w.credentialApprovalTerminal("plain", "yes", "", ""); err != nil {
			return err
		}
		w.approval.results = []doctorResult{{stdout: w.stdout, stderr: w.stderr, exit: w.exit}}
		return w.runDoctor(w.env(), "doctor", "--json")
	})
	sc.Step(`^the credential review escapes the prior and current paths before consent and preserves their exact decoded JSON values$`, func() error {
		text := w.approval.beforeReview
		for _, settings := range []map[string]any{w.approval.prior, w.approval.settings} {
			for _, raw := range approvalClone(settings)["credentials"].([]any) {
				path := object(raw)["source"].(string)
				if !strings.Contains(text, credentialApprovalEscape(string(approvalJSON(path)))) {
					return fmt.Errorf("literal source path missing from review")
				}
				for _, r := range path {
					if !unicode.IsPrint(r) && strings.ContainsRune(text, r) {
						return fmt.Errorf("raw source control in review")
					}
				}
			}
		}
		return w.approvalCandidate()
	})
	sc.Step(`^both commands succeed with the independently calculated current credential fingerprint$`, func() error {
		if len(w.approval.results) != 1 || w.approval.results[0].exit != 0 || !strings.Contains(w.approval.results[0].stdout, w.approval.fingerprint) {
			return fmt.Errorf("terminal approval did not succeed")
		}
		if err := w.exitsWith(0); err != nil {
			return err
		}
		return w.approvalCandidate()
	})
}
