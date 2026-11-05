package features

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/cucumber/godog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (w *world) approvalContractSteps(sc *godog.ScenarioContext) {
	sc.Step(`^an approval contract fixture with effective UID "([^"]*)" and GID "([^"]*)"$`, func(uid, gid string) { w.approval = &approvalWorld{uid: uid, gid: gid} })
	sc.Step(`^a matching approval contract fixture for UID "([^"]*)" and GID "([^"]*)"$`, func(uid, gid string) { w.approval = &approvalWorld{uid: uid, gid: gid} })
	sc.Step(`^an approval store contract fixture with failure "([^"]*)"$`, func(failure string) { w.approval = &approvalWorld{condition: failure} })
	sc.Step(`^an approval contract fixture with selection condition "([^"]*)"$`, func(condition string) { w.approval = &approvalWorld{condition: condition} })
	sc.Step(`^the approval identity contract resolves the candidate$`, func() error { return w.approvalProbe("identity") })
	sc.Step(`^the approval contract resolves UID "([^"]*)" and GID "([^"]*)" with omitted manifest users$`, func(uid, gid string) error {
		w.approval.uid, w.approval.gid = uid, gid
		return w.approvalProbe("identity-change")
	})
	sc.Step(`^the approval store contract attempts to publish a consented replacement$`, func() error { return w.approvalProbe("publication") })
	sc.Step(`^frozen input A is selected and the caller mutates its inputs and returned buffers before approval B is recorded$`, func() error { return w.approvalProbe("independence") })
	sc.Step(`^the frozen-input contract selects the requested approval$`, func() error { return w.approvalProbe("selection") })
	sc.Step(`^the contract refuses with "([^"]*)" and produces no candidate or store write$`, func(failure string) error {
		out := w.approval.contract
		if out["failure"] != failure || out["writes"] != float64(0) {
			return fmt.Errorf("identity refusal = %v", out)
		}
		c, _ := out["candidate"].(map[string]any)
		if c["Canonical"] != "" {
			return fmt.Errorf("refusal produced candidate")
		}
		return nil
	})
	sc.Step(`^it resolves worker identity "([^"]*)" and reports approval "([^"]*)"$`, func(identity, status string) error {
		out := w.approval.contract
		c, _ := out["candidate"].(map[string]any)
		if out["failure"] != "" || out["status"] != status || fmt.Sprint(c["UID"])+":"+fmt.Sprint(c["GID"]) != identity {
			return fmt.Errorf("resolved identity/status = %v", out)
		}
		return nil
	})
	sc.Step(`^its canonical fingerprint binds both independently supplied host IDs$`, func() error {
		c, _ := w.approval.contract["candidate"].(map[string]any)
		expected := map[string]any{
			"schema": 1, "profile": "offline-v1", "host": map[string]string{"uid": w.approval.uid, "gid": w.approval.gid},
			"checkout": map[string]string{"root": "/checkout", "device": "1", "inode": "2", "git_device": "1", "git_inode": "3"},
			"engine":   map[string]string{"endpoint": "unix:///engine.sock", "id": "engine-A", "os": "linux", "architecture": "amd64"},
			"policy":   map[string]any{"version": 1, "manifest": "worker.json", "network": map[string]string{"mode": "offline"}},
			"manifest": map[string]any{"name": nil, "image": doctorImage, "workspaceFolder": "/workspace", "containerEnv": map[string]string{}, "containerUser": w.approval.uid, "remoteUser": w.approval.uid},
			"image":    map[string]string{"reference": doctorImage, "id": doctorImageID, "os": "linux", "architecture": "amd64"},
		}
		if c["Canonical"] != string(approvalJSON(expected)) || c["Fingerprint"] != "sha256:"+approvalHash(expected) {
			return fmt.Errorf("identity canonicalization differs from independent oracle")
		}
		return nil
	})
	sc.Step(`^publication reports "([^"]*)" and no successful approval result$`, func(failure string) error {
		out := w.approval.contract
		record, _ := out["record"].(map[string]any)
		if out["failure"] != failure || record["Generation"] != "" {
			return fmt.Errorf("publication result = %v", out)
		}
		return nil
	})
	sc.Step(`^the visible active record is "([^"]*)"$`, func(visible string) error {
		record, _ := w.approval.contract["visible"].(map[string]any)
		generation := record["Generation"]
		if generation != "1" && (visible != "a complete old or new record" || generation != "2") {
			return fmt.Errorf("wrong visible record: %v", record)
		}
		return nil
	})
	sc.Step(`^no partial record or outside-target write is visible$`, func() error {
		if w.approval.contract["writes"] != float64(0) {
			return fmt.Errorf("unexpected write")
		}
		return nil
	})
	sc.Step(`^the retained frozen input A still contains exactly its original canonical settings and approval identifiers$`, func() error { return w.approvalFrozen("first", "original", "1") })
	sc.Step(`^a fresh selection for B returns only B's independently owned canonical bytes$`, func() error { return w.approvalFrozen("second", "expected_second", "2") })
	sc.Step(`^neither selection reads checkout data after choosing its approved record$`, func() error {
		// The cmd fixture binds only canonical values and the memory store: no checkout
		// input port is supplied to Select. Successful actual results are required.
		out := w.approval.contract
		for _, key := range []string{"failure", "publish_failure", "select_failure"} {
			if out[key] != "" {
				return fmt.Errorf("selection failed: %v", out)
			}
		}
		return nil
	})
	sc.Step(`^selection refuses with "([^"]*)" and returns no frozen input$`, func(failure string) error {
		out := w.approval.contract
		frozen, _ := out["frozen"].(map[string]any)
		if out["failure"] != failure || frozen["Settings"] != "" || frozen["Project"] != "" {
			return fmt.Errorf("selection result = %v", out)
		}
		return nil
	})
}
func (w *world) approvalProbe(kind string) error {
	path := filepath.Join(w.dir, "slice-4-contract.json")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	args := []string{"test", "./cmd/itos-container", "-run", "^TestApprovalContractProbe$", "-count=1"}
	if coverdir := os.Getenv("GOCOVERDIR"); coverdir != "" {
		// go test supplies its own temporary GOCOVERDIR. Explicitly direct the
		// test binary's counters to the caller's per-ID measurement directory.
		args = append(args, "-cover", "-covermode=atomic", "-coverpkg=./...", "-args", "-test.gocoverdir="+coverdir)
	}
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = w.root
	cmd.Env = os.Environ()
	for key, value := range map[string]string{"SLICE_4_PROBE_OUTPUT": path, "SLICE_4_PROBE_KIND": kind, "SLICE_4_PROBE_UID": w.approval.uid, "SLICE_4_PROBE_GID": w.approval.gid, "SLICE_4_PROBE_CONDITION": w.approval.condition} {
		cmd.Env = setEnv(cmd.Env, key, value)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("contract fixture: %w\n%s", err, output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &w.approval.contract)
}
func (w *world) approvalFrozen(key, expected, generation string) error {
	out := w.approval.contract
	got, _ := out[key].(map[string]any)
	want, _ := out[expected].(map[string]any)
	if out["failure"] != "" || got["Generation"] != generation || got["Settings"] != want["Canonical"] || got["Project"] != want["Project"] || got["Fingerprint"] != want["Fingerprint"] || strings.HasPrefix(fmt.Sprint(got["Settings"]), "X") {
		return fmt.Errorf("frozen value differs: %v", out)
	}
	return nil
}
