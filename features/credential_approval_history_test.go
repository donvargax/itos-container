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

func (w *world) credentialApprovalHistorySteps(sc *godog.ScenarioContext) {
	sc.Step(`^terminal credential approval is attempted then the manifest is removed and run is invoked with literal argv$`, func() error {
		if err := w.credentialApprovalTerminal("plain", "yes", "", ""); err != nil {
			return err
		}
		w.credentialApproval.runResult = doctorResult{stdout: w.stdout, stderr: w.stderr, exit: w.exit}
		if err := os.Remove(filepath.Join(w.doctor.checkout, ".devcontainer/devcontainer.json")); err != nil {
			return err
		}
		var err error
		w.credentialApproval.runBefore, err = w.credentialApprovalTree()
		if err != nil {
			return err
		}
		return w.runDoctor(w.env(), "run", "--", "credential-fixture-never-executed", "$(literal)")
	})
	sc.Step(`^the terminal attempt records the exact credential candidate at generation "([^"]*)"$`, func(g string) error {
		if w.credentialApproval.runResult.exit != 0 {
			return fmt.Errorf("terminal approval refused before the staged run")
		}
		return w.approvalStored(g)
	})
	sc.Step(`^the subsequent run reports "([^"]*)" at "([^"]*)" with exit (\d+)$`, w.credentialApprovalProblem)
	sc.Step(`^that run opens no manifest source data engine connection or protected state$`, func() error {
		if err := w.credentialApprovalNoAccess(); err != nil {
			return err
		}
		// An absent manifest and a policy diagnostic prove the early gate; the
		// API journal must contain only the earlier approval's inspection calls.
		if strings.Contains(w.stdout+w.stderr, "manifest.") {
			return fmt.Errorf("run inspected the removed manifest")
		}
		return w.doctorAllowedJournal(true)
	})
	sc.Step(`^that run publishes no snapshot intent session or staged credential material$`, w.credentialApprovalRunUnchanged)
	sc.Step(`^the approved record and all existing fixture resources remain unchanged by run$`, w.credentialApprovalRunUnchanged)
	sc.Step(`^the run change diagnostic lists exactly "([^"]*)"$`, func(pointers string) error {
		for _, path := range strings.Fields(pointers) {
			if !strings.Contains(w.stderr, path) {
				return fmt.Errorf("run omitted changed path %s", path)
			}
		}
		for _, bad := range []string{"/credentials/0", "/credentials/1"} {
			if strings.Contains(w.stderr, bad) {
				return fmt.Errorf("run indexed a grant difference")
			}
		}
		return nil
	})
	sc.Step(`^run makes no engine connection and publishes no approval snapshot or session$`, func() error {
		if w.doctor.engine.listener.connections.Load() != 0 {
			return fmt.Errorf("run contacted engine")
		}
		return w.credentialApprovalNoPublication()
	})
	sc.Step(`^an independently seeded stopped credential session on "([^"]*)" with both frozen grants$`, w.credentialApprovalSession)
	sc.Step(`^the current request is invalid the active approval is removed and both original sources are absent$`, func() error {
		if err := os.WriteFile(filepath.Join(w.doctor.checkout, "itos-container.yaml"), []byte("invalid: ["), 0600); err != nil {
			return err
		}
		if err := os.Remove(w.approval.record); err != nil {
			return err
		}
		for _, g := range w.credentialApproval.grants {
			if err := os.Remove(g.Source); err != nil {
				return err
			}
		}
		return w.recoveryCapture()
	})
	sc.Step(`^credential session status is invoked in plain and JSON modes$`, func() error {
		for _, args := range [][]string{{"status"}, {"status", "--json"}} {
			if err := w.runDoctor(w.env(), args...); err != nil {
				return err
			}
			w.approval.results = append(w.approval.results, doctorResult{stdout: w.stdout, stderr: w.stderr, exit: w.exit})
		}
		return nil
	})
	sc.Step(`^both status results succeed with the original fingerprint and exactly the frozen grants$`, func() error {
		if len(w.approval.results) != 2 {
			return fmt.Errorf("missing two status results")
		}
		if w.approval.results[0].exit != 0 || !strings.Contains(w.approval.results[0].stdout, w.approval.fingerprint) {
			return fmt.Errorf("plain status omitted frozen credential authority")
		}
		return w.credentialApprovalStatus(w.approval.results[1])
	})
	sc.Step(`^plain credential status shows the frozen disclosure without claiming new delivery is available$`, func() error {
		text := w.approval.results[0].stdout
		if err := w.credentialApprovalDescribe(text, false); err != nil {
			return err
		}
		if !strings.Contains(text, w.recoveryLocal.id) || strings.Contains(text, credentialDisclosure[5]) {
			return fmt.Errorf("status disclosure lacks its session or claims the approval gate")
		}
		return nil
	})
	sc.Step(`^status neither reads mutable requests approvals or source data nor changes protected records$`, w.credentialApprovalStatusUnchanged)
	sc.Step(`^the current request and active approval name different valid grants$`, func() error {
		frozen := approvalClone(w.approval.settings)
		if err := w.credentialApprovalRequest("github", "offline"); err != nil {
			return err
		}
		if err := w.approvalSeed("2", false); err != nil {
			return err
		}
		w.credentialApproval.frozen = frozen
		return w.recoveryCapture()
	})
	sc.Step(`^status is invoked then the fixture removes the frozen credentials member recomputes its hash and invokes status again$`, func() error {
		if err := w.runDoctor(w.env(), "status", "--json"); err != nil {
			return err
		}
		w.approval.results = []doctorResult{{stdout: w.stdout, stderr: w.stderr, exit: w.exit}}
		settings := approvalClone(w.credentialApproval.frozen)
		delete(settings, "credentials")
		if err := w.credentialApprovalReplaceSnapshot(settings); err != nil {
			return err
		}
		if err := w.recoveryCapture(); err != nil {
			return err
		}
		return w.runDoctor(w.env(), "status", "--json")
	})
	sc.Step(`^the first status succeeds with exactly the original frozen grants$`, func() error {
		if len(w.approval.results) != 1 {
			return fmt.Errorf("missing initial status")
		}
		return w.credentialApprovalStatus(w.approval.results[0])
	})
	sc.Step(`^the second status reports "([^"]*)" with exit (\d+) and no session entry$`, func(rule string, exit int) error {
		if err := w.exitsWith(exit); err != nil {
			return err
		}
		obj, err := w.json()
		if err != nil {
			return err
		}
		sessions, _ := obj["sessions"].([]any)
		if len(sessions) != 0 {
			return fmt.Errorf("invalid credential snapshot produced a session")
		}
		if !strings.Contains(w.stdout, rule) {
			return fmt.Errorf("missing protected snapshot refusal")
		}
		return nil
	})
	sc.Step(`^neither status reads current grants or repairs protected records$`, w.credentialApprovalStatusUnchanged)
}

func (w *world) credentialApprovalRunUnchanged() error {
	tree, err := w.credentialApprovalTree()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(tree, w.credentialApproval.runBefore) {
		return fmt.Errorf("staged run changed fixture state")
	}
	return nil
}

func (w *world) credentialApprovalSession(network string) error {
	if err := w.recoverySeed("stopped", "durable absent", "0"); err != nil {
		return err
	}
	w.credentialApproval = &credentialApprovalWorld{}
	c := &credentialWorld{sources: filepath.Join(w.doctor.host, "sources"), paths: map[string]string{}}
	w.credentials = c
	if err := os.MkdirAll(c.sources, 0700); err != nil {
		return err
	}
	for _, name := range []string{"claude-a", "github-a"} {
		c.paths[name] = filepath.Join(c.sources, name)
		if err := os.WriteFile(c.paths[name], []byte("itos-independent-historical-fixture"), 0600); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(w.doctor.host, "slice-30-read-control"), []byte("observer control"), 0600); err != nil {
		return err
	}
	if err := w.credentialApprovalRequest("both", network); err != nil {
		return err
	}
	w.credentialApproval.frozen = approvalClone(w.approval.settings)
	if err := w.credentialApprovalReplaceSnapshot(w.approval.settings); err != nil {
		return err
	}
	return w.recoveryCapture()
}

func (w *world) credentialApprovalReplaceSnapshot(settings map[string]any) error {
	a := w.approval
	fingerprint := "sha256:" + approvalHash(settings)
	record := map[string]any{"schema": 1, "project": a.project, "generation": "1", "fingerprint": fingerprint, "settings": settings}
	snapshot := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "snapshots", a.project, "1.json")
	if err := os.WriteFile(snapshot, approvalJSON(record), 0600); err != nil {
		return err
	}
	data, err := os.ReadFile(w.sessionRecordPath())
	if err != nil {
		return err
	}
	var intent map[string]any
	if err := json.Unmarshal(data, &intent); err != nil {
		return err
	}
	intent["fingerprint"] = fingerprint
	return os.WriteFile(w.sessionRecordPath(), approvalJSON(intent), 0600)
}

func (w *world) credentialApprovalStatus(r doctorResult) error {
	if r.exit != 0 {
		return fmt.Errorf("credential status refused valid history")
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &obj); err != nil {
		return err
	}
	entries, _ := obj["sessions"].([]any)
	if len(entries) != 1 {
		return fmt.Errorf("status did not return one protected session")
	}
	e := object(entries[0])
	settings := w.credentialApproval.frozen
	if e["fingerprint"] != "sha256:"+approvalHash(settings) || !reflect.DeepEqual(e["credentials"], settings["credentials"]) {
		return fmt.Errorf("status changed or omitted frozen credential grants")
	}
	return nil
}

func (w *world) credentialApprovalStatusUnchanged() error {
	if w.credentialApproval.accessError != nil {
		return w.credentialApproval.accessError
	}
	return w.recoveryUnchanged()
}
