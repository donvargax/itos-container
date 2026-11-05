package features

import (
	"fmt"
	"github.com/cucumber/godog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func (w *world) approvalSteps(sc *godog.ScenarioContext) {
	sc.Step(`^an approval checkout and isolated host state$`, w.approvalCheckout)
	sc.Step(`^a seeded matching approval at generation "([^"]*)"$`, func(g string) error { return w.approvalSeed(g, false) })
	sc.Step(`^a seeded prior approval at generation "([^"]*)" with a different application environment and workspace$`, func(g string) error { return w.approvalSeed(g, true) })
	sc.Step(`^the approval manifest omits both users$`, func() error { return w.doctorUsers("omitted", "omitted") })
	sc.Step(`^misleading USER, LOGNAME, UID, GID, SUDO_UID and SUDO_GID fixture values$`, func() {
		for _, key := range []string{"USER", "LOGNAME", "UID", "GID", "SUDO_UID", "SUDO_GID"} {
			w.doctor.env[key] = "999999"
		}
	})
	sc.Step(`^"([^"]*)" requests a positive UID different from the process effective UID$`, func(field string) error {
		w.doctor.manifest[field] = strconv.Itoa(os.Geteuid() + 1)
		return w.approvalManifest()
	})
	sc.Step(`^approval state is "([^"]*)"$`, w.approvalState)
	sc.Step(`^a seeded matching approval with record defect "([^"]*)"$`, w.approvalDefect)
	sc.Step(`^approval storage has unsafe layout "([^"]*)"$`, w.approvalUnsafe)
	sc.Step(`^the child state location is "([^"]*)" with umask "0000"$`, func(location string) {
		w.approval.location = location
		switch location {
		case "unset XDG_STATE_HOME", "empty XDG_STATE_HOME":
			w.doctor.env["XDG_STATE_HOME"] = ""
		case "relative XDG_STATE_HOME":
			w.doctor.env["XDG_STATE_HOME"] = "relative-state"
		}
	})
	sc.Step(`^active approval B at generation "2" and a historical approval A at generation "1"$`, func() error {
		if err := w.approvalSeed("1", false); err != nil {
			return err
		}
		data := w.approval.recordBefore
		if err := w.approvalSeed("2", true); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(filepath.Dir(w.approval.record), "historical.json"), data, 0600)
	})
	sc.Step(`^the checkout requests settings A$`, func() error { return w.approvalCapture() })
	sc.Step(`^two approval checkouts named "([^"]*)" and "([^"]*)" in isolated host state$`, func(first, second string) error {
		if err := w.approvalCheckout(); err != nil {
			return err
		}
		d := w.doctor
		if err := w.writeDoctorCheckout(filepath.Join(w.dir, first)); err != nil {
			return err
		}
		if err := w.writeDoctorCheckout(filepath.Join(w.dir, second)); err != nil {
			return err
		}
		d.checkout = filepath.Join(w.dir, first)
		d.cwd = d.checkout
		w.approval.secondRoot = filepath.Join(w.dir, second)
		return w.approvalOracle()
	})
	sc.Step(`^a seeded matching approval for the first checkout$`, func() error {
		if err := w.approvalSeed("1", false); err != nil {
			return err
		}
		w.approval.firstRecord = w.approval.record
		w.approval.firstBytes = w.approval.recordBefore
		return nil
	})
	sc.Step(`^the second checkout has "([^"]*)"$`, func(copyKind string) error {
		a, d := w.approval, w.doctor
		d.checkout = a.secondRoot
		d.cwd = d.checkout
		if err := w.approvalOracle(); err != nil {
			return err
		}
		path := ""
		switch copyKind {
		case "no copied record":
		case "a copied approval inside its writable checkout":
			path = filepath.Join(d.checkout, "itos-container", "approvals", a.project, "current.json")
		case "the first record copied to the second project's host path":
			path = a.record
		default:
			return fmt.Errorf("unknown copy %q", copyKind)
		}
		if path != "" {
			if err := writeManifestFixture(path, a.firstBytes, 0600); err != nil {
				return err
			}
		}
		return w.approvalCapture()
	})
	sc.Step(`^a literal application environment value contains terminal controls and shell-looking text$`, func() error {
		w.approval.literal = "\x1b[31m\r\n\t$(touch " + filepath.Join(w.doctor.recorder, "executed") + "); `echo nope` \\\" <>&\u2028"
		w.doctor.manifest["containerEnv"] = map[string]string{"APP": w.approval.literal}
		return w.approvalManifest()
	})
	sc.Step(`^approval input has prerequisite defect "([^"]*)"$`, func(defect string) error {
		w.approval.condition = defect
		switch defect {
		case "unsupported manifest initializeCommand":
			w.doctor.manifest["initializeCommand"] = "./forbidden"
			return w.approvalManifest()
		case "hidden image label requests a user":
			w.doctor.engine.config()["Labels"] = map[string]string{"devcontainer.metadata": `[{"containerUser":"1000"}]`}
		case "cached image is absent":
			w.doctor.engine.imageStatus = 404
		case "daemon identity changes during inspection", "checkout root changes during inspection":
			w.doctor.engine.paused = true
		default:
			return fmt.Errorf("unknown prerequisite %q", defect)
		}
		return nil
	})
	sc.Step(`^doctor inspects before and after the (?:equivalent|effective) edit "([^"]*)"$`, w.approvalTwoInspections)
	sc.Step(`^doctor inspects the second checkout as JSON$`, func() error { return w.runDoctor(w.env(), "doctor", "--json") })
	sc.Step(`^bare approve runs at a terminal and receives "([^"]*)" after the review$`, func(answer string) error { return w.approvalTerminal("plain", answer, "", "") })
	sc.Step(`^bare approve runs at a terminal with no answer available$`, func() error { return w.approvalTerminal("plain", "unavailable", "", "") })
	sc.Step(`^bare approve runs with "([^"]*)" and input "y"$`, func(streams string) error { return w.approvalTerminal("plain", "y", "", streams) })
	sc.Step(`^approve runs without a terminal with the independently calculated exact fingerprint and "([^"]*)"$`, w.approvalExact)
	sc.Step(`^approve runs with the exact fingerprint without fchmodat2 and with umask "([^"]*)"$`, w.approvalLegacyKernel)
	sc.Step(`^approve runs without a terminal with fingerprint "([^"]*)" as JSON$`, w.approvalBadFingerprint)
	sc.Step(`^terminal approve shows its review, the fixture (?:changes|replaces) "([^"]*)", and then answers yes$`, func(edit string) error { return w.approvalTerminal("plain", "yes", edit, "") })
	sc.Step(`^two terminal approvals review generation "4", the first answers yes, then the second answers yes$`, w.approvalConcurrent)
	sc.Step(`^approve runs without a terminal with a well-formed candidate fingerprint as JSON$`, w.approvalPrerequisite)
	sc.Step(`^approve runs at a terminal in "([^"]*)" with "([^"]*)"$`, func(mode, consent string) error { return w.approvalTerminal(mode, consent, "", "") })
	sc.Step(`^doctor reports the independently calculated fingerprint and complete effective settings$`, w.approvalCandidate)
	sc.Step(`^doctor reports approval "([^"]*)" and names "([^"]*)"$`, w.approvalStatusGuidance)
	sc.Step(`^both inspections report matching approval and identical independently calculated fingerprints$`, func() error { return w.approvalInspections(false) })
	sc.Step(`^the second inspection reports changed approval and a different independently calculated fingerprint$`, func() error { return w.approvalInspections(true) })
	sc.Step(`^its approval differences identify "([^"]*)" with the prior and current values$`, w.approvalDifference)
	sc.Step(`^the approval candidate uses the process effective UID and GID and resolves both manifest users to that UID$`, w.approvalCandidate)
	sc.Step(`^it reports approval problem "([^"]*)" with exit (\d+)(?: and field "([^"]*)")?$`, w.approvalProblem)
	sc.Step(`^it reports prerequisite problem "([^"]*)" with exit (\d+) before requesting consent$`, func(rule string, exit int) error { return w.approvalProblem(rule, exit, "") })
	sc.Step(`^the transcript shows all effective settings and offline restrictions before exactly one y/N prompt$`, func() error { return w.approvalReview(false) })
	sc.Step(`^the transcript shows complete settings and sorted old-to-new differences before exactly one y/N prompt$`, func() error { return w.approvalReview(true) })
	sc.Step(`^it reports terminal approval refusal "([^"]*)" with exit (\d+)(?: after showing the review)?$`, w.approvalTerminalRefusal)
	sc.Step(`^it reports matching approval without prompting and exits with code 0$`, w.approvalSuccess)
	sc.Step(`^it reports matching approval for that fingerprint without a prompt and exits with code 0$`, w.approvalSuccess)
	sc.Step(`^the sole active approval is the exact candidate at generation "([^"]*)" in protected host state$`, w.approvalStored)
	sc.Step(`^the active approval bytes and generation are unchanged$`, w.approvalRecordUnchanged)
	sc.Step(`^the approval checkout and host state are unchanged$`, w.approvalSameTree)
	sc.Step(`^every existing state entry and outside target is unchanged$`, w.approvalSameTree)
	sc.Step(`^the checkout is unchanged and only approval state was written$`, w.approvalOnlyState)
	sc.Step(`^its guidance names "([^"]*)" and "([^"]*)"$`, func(first, second string) error { return w.approvalContains(first, second) })
	sc.Step(`^it never prompts or records approval$`, func() error {
		if strings.Contains(w.stdout+w.stderr, "y/N") {
			return fmt.Errorf("unexpected prompt")
		}
		return w.approvalRecordUnchanged()
	})
	sc.Step(`^no approval record or frozen input is produced$`, w.approvalNoRecord)
	sc.Step(`^its approval output follows the requested plain or JSON mode$`, w.approvalOutputMode)
	sc.Step(`^the replacement target is unchanged$`, func() error {
		if w.doctor.replacement == "" {
			return w.approvalNoRecord()
		}
		return w.doctorReplacementUnchanged()
	})
	sc.Step(`^it reports the second checkout's independent fingerprint and approval "([^"]*)"$`, func(status string) error {
		if err := w.approvalCandidate(); err != nil {
			return err
		}
		return w.approvalStatus(status)
	})
	sc.Step(`^the first checkout's approval remains unchanged$`, func() error {
		data, err := os.ReadFile(w.approval.firstRecord)
		if err != nil {
			return err
		}
		if string(data) != string(w.approval.firstBytes) {
			return fmt.Errorf("first approval changed")
		}
		return nil
	})
	sc.Step(`^its current approval generation is "([^"]*)" and its candidate fingerprint is A$`, func(g string) error {
		if err := w.approvalCandidate(); err != nil {
			return err
		}
		view, err := w.approvalView()
		if err != nil {
			return err
		}
		if view["generation"] != g {
			return fmt.Errorf("generation = %v", view["generation"])
		}
		return nil
	})
	sc.Step(`^approval storage outcome is "([^"]*)" at "([^"]*)" with exit (\d+)$`, w.approvalLocation)
	sc.Step(`^all created managed directories are owned by the effective UID with mode 0700$`, w.approvalPermissions)
	sc.Step(`^all created records and locks are owned by the effective UID with mode 0600$`, w.approvalPermissions)
	sc.Step(`^no approval appears in the checkout or another host-state location$`, w.approvalOnlyState)
	sc.Step(`^the first records generation "5" and exits with code 0$`, w.approvalFirstWinner)
	sc.Step(`^the second reports "approve.conflict" with exit 1 without another write$`, func() error { return w.approvalTerminalRefusal("approve.conflict", 1) })
	sc.Step(`^the only active record is the first process's complete canonical approval$`, func() error { return w.approvalStored("5") })
	sc.Step(`^run refuses approval "([^"]*)" with exit 1 and names "itos container approve"$`, func(reason string) error {
		if err := w.exitsWith(1); err != nil {
			return err
		}
		return w.approvalContains(reason, "itos container approve")
	})
	sc.Step(`^run reports that approval matched before refusing unimplemented offline startup with exit 3$`, func() error {
		if err := w.exitsWith(3); err != nil {
			return err
		}
		return w.approvalContains("approval matched", "offline startup is not implemented")
	})
	sc.Step(`^the approved mount boundary contains only the checkout and excludes host approval and snapshot paths$`, w.approvalBoundary)
	sc.Step(`^approval help describes terminal review, changes, y/N, remembered approval and optional exact --fingerprint$`, func() error {
		return w.approvalContains("terminal", "changes", "y/N", "settings change", "--fingerprint")
	})
	sc.Step(`^approval presentation is "([^"]*)" with exit (\d+)$`, w.approvalPresentation)
	sc.Step(`^no shell-looking text is executed$`, w.doctorNoMarker)
	sc.Step(`^any stored or JSON settings preserve the exact literal environment value$`, w.approvalLiteral)
	sc.Step(`^approval review contains "([^"]*)" controls in the "([^"]*)" environment$`, w.approvalUnicodeEnvironment)
	sc.Step(`^the terminal review escapes "([^"]*)" controls from the "([^"]*)" environment before consent$`, w.approvalUnicodeReview)
	sc.Step(`^the plain approval status escapes "([^"]*)" controls and preserves the effective settings$`, w.approvalUnicodeStatus)
	w.approvalContractSteps(sc)
}

func (w *world) approvalTwoInspections(edit string) error {
	if err := w.approvalCapture(); err != nil {
		return err
	}
	if err := w.runDoctor(w.env(), "doctor", "--json"); err != nil {
		return err
	}
	w.approval.results = append(w.approval.results, w.doctor.results[len(w.doctor.results)-1])
	if err := w.approvalEdit(edit); err != nil {
		return err
	}
	if err := w.approvalOracle(); err != nil {
		return err
	}
	if err := w.runDoctor(w.env(), "doctor", "--json"); err != nil {
		return err
	}
	w.approval.results = append(w.approval.results, w.doctor.results[len(w.doctor.results)-1])
	return nil
}
func (w *world) approvalExact(mode string) error {
	if err := w.approvalOracle(); err != nil {
		return err
	}
	if w.approval.before == nil {
		if err := w.approvalCapture(); err != nil {
			return err
		}
	}
	w.approval.mode = mode
	env, args := w.approvalMode(mode)
	args = append(args, "--fingerprint", w.approval.fingerprint)
	return w.runDoctor(env, args...)
}
func (w *world) approvalMode(mode string) ([]string, []string) {
	env, args := w.env(), []string{"approve"}
	if w.approval.location == "unset XDG_STATE_HOME" {
		var clean []string
		for _, value := range env {
			if !strings.HasPrefix(value, "XDG_STATE_HOME=") {
				clean = append(clean, value)
			}
		}
		env = clean
	}
	switch mode {
	case "--json":
		args = append(args, "--json")
	case "inherited ITOS_JSON=1":
		env = setEnv(env, "ITOS_JSON", "1")
	case "ITOS_JSON=1 with --no-json":
		env = setEnv(env, "ITOS_JSON", "1")
		args = append(args, "--no-json")
	}
	return env, args
}
func (w *world) approvalBadFingerprint(value string) error {
	if err := w.approvalCapture(); err != nil {
		return err
	}
	fingerprint := "sha256:" + strings.Repeat("0", 64)
	switch value {
	case "a SHA-256 fingerprint for another checkout":
		fingerprint = "sha256:" + approvalHash(map[string]any{"checkout": "other"})
	case "uppercase hexadecimal":
		fingerprint = "sha256:" + strings.Repeat("A", 64)
	case "a bare hexadecimal digest without sha256:":
		fingerprint = strings.Repeat("a", 64)
	case "an empty flag value":
		fingerprint = ""
	case "a short digest":
		fingerprint = "sha256:abcd"
	}
	return w.runDoctor(w.env(), "approve", "--json", "--fingerprint="+fingerprint)
}
