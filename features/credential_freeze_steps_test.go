package features

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func (w *world) credentialFreezeSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a credential preflight contract with independently approved "([^"]*)" and requested "([^"]*)" grants$`, func(prior, current string) {
		w.credentialApproval = &credentialApprovalWorld{admission: &credentialAdmissionFixture{request: current, prior: prior, command: "preflight"}}
	})
	sc.Step(`^the real local preflight and run refusal presentation consume the fixture observations$`, func() error { return w.credentialAdmissionRun("preflight", "", "", "", "", 0) })
	sc.Step(`^the contract refuses changed approval with exit 1 naming "itos container approve" and exactly "([^"]*)"$`, func(pointers string) error {
		f := w.credentialApproval.admission
		r := f.results[0]
		if r["Exit"] != float64(1) {
			return fmt.Errorf("preflight did not refuse changed grant with exit 1")
		}
		extra := object(r["Extra"])
		text := string(approvalJSON(extra["problems"]))
		if !strings.Contains(text, "run.approval-changed") || !strings.Contains(text, "itos container approve") {
			return fmt.Errorf("missing public changed-grant guidance")
		}
		paths := regexp.MustCompile(`/[a-zA-Z]+(?:/[a-zA-Z]+)*`).FindAllString(text, -1)
		slices.Sort(paths)
		paths = slices.Compact(paths)
		if !slices.Equal(paths, strings.Fields(pointers)) {
			return fmt.Errorf("preflight grant difference paths differ")
		}
		return nil
	})
	sc.Step(`^the contract makes no engine observation approval write or frozen publication$`, func() error {
		extra := object(w.credentialApproval.admission.results[0]["Extra"])
		for _, key := range []string{"engines", "writes", "snapshots"} {
			if extra[key] != float64(0) {
				return fmt.Errorf("preflight performed %s", key)
			}
		}
		return nil
	})
	sc.Step(`^a credential freeze contract with independently seeded matching grants at generation "4"$`, func() {
		w.credentialApproval = &credentialApprovalWorld{admission: &credentialAdmissionFixture{request: "both"}}
	})
	sc.Step(`^the real service selects and publishes frozen input then the fixture replaces the request and active approval$`, w.credentialFreezeInvoke)
	sc.Step(`^the selected and persisted snapshot contain the exact original canonical settings bytes fingerprint project and generation$`, func() error {
		f := w.credentialApproval.admission
		settings := credentialAdmissionSettings("both", "/checkout")
		for _, backend := range []string{"memory", "disk"} {
			r := object(f.freeze[backend])
			if r["failure"] != "" || r["publication"] != "" {
				return fmt.Errorf("credential freeze %s refused valid canonical grants: selection=%v publication=%v", backend, r["failure"], r["publication"])
			}
			if err := credentialFrozenValue(object(r["retained"]), settings); err != nil {
				return err
			}
			if backend == "memory" {
				if err := credentialFrozenValue(object(r["persisted"]), settings); err != nil {
					return err
				}
			} else {
				stored := object(r["persisted"])
				if !reflect.DeepEqual(stored["settings"], settings) || stored["fingerprint"] != "sha256:"+approvalHash(settings) || stored["generation"] != "4" {
					return fmt.Errorf("persisted credential snapshot differs")
				}
			}
		}
		return nil
	})
	sc.Step(`^changing caller data or returned values cannot alter the stored or independently returned grants$`, func() error {
		for _, backend := range []string{"memory", "disk"} {
			r := object(w.credentialApproval.admission.freeze[backend])
			if r["second_failure"] != "" {
				return fmt.Errorf("independent selection refused")
			}
			if err := credentialFrozenValue(object(r["independent"]), credentialAdmissionSettings("both", "/checkout")); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^a new selection using generation "4" refuses as superseded without another snapshot$`, func() error {
		for _, backend := range []string{"memory", "disk"} {
			r := object(w.credentialApproval.admission.freeze[backend])
			if r["replacement_failure"] != "" || r["stale"] != "superseded" || r["count"] != float64(1) {
				return fmt.Errorf("stale generation selection or snapshot count differs")
			}
		}
		return nil
	})
	sc.Step(`^the credential freeze contract reads no source data and creates no worker or staged material$`, func() error {
		// Only the selected-value store and snapshots ports are bound by this
		// assembly. Require results from both real services, rather than a skip.
		for _, backend := range []string{"memory", "disk"} {
			if len(object(w.credentialApproval.admission.freeze[backend])) == 0 {
				return fmt.Errorf("missing freeze backend")
			}
		}
		return nil
	})
}

func credentialFrozenValue(got, settings map[string]any) error {
	project := approvalHash(map[string]any{"schema": 1, "uid": "1000", "checkout": settings["checkout"]})
	if got["Settings"] != string(approvalJSON(settings)) || got["Fingerprint"] != "sha256:"+approvalHash(settings) || got["Generation"] != "4" || got["Project"] != project {
		return fmt.Errorf("frozen grant authority differs from independent inputs")
	}
	return nil
}

func (w *world) credentialFreezeInvoke() error {
	bin, err := w.sessionProbeBinary()
	if err != nil {
		return err
	}
	input, output := filepath.Join(w.dir, "slice-30-freeze-input.json"), filepath.Join(w.dir, "slice-30-freeze-output.json")
	request := map[string]string{"Approved": string(approvalJSON(credentialAdmissionSettings("both", "/checkout"))), "Replacement": string(approvalJSON(credentialAdmissionSettings("github", "/checkout")))}
	if err := os.WriteFile(input, approvalJSON(request), 0600); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := []string{"-test.run=^TestCredentialFreezeProcess$", "-test.count=1"}
	if dir := w.coverageDir(); dir != "" {
		args = append(args, "-test.gocoverdir="+dir)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = w.root
	cmd.Env = setEnv(setEnv(w.coverageEnv(os.Environ()), "SLICE_30_ADMISSION_INPUT", input), "SLICE_30_ADMISSION_OUTPUT", output)
	if data, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("freeze assembly: %w\n%s", err, data)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &w.credentialApproval.admission.freeze)
}
