package features

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/cucumber/godog"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func (w *world) runContractSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a run preflight contract fixture with effective UID "([^"]*)" and GID "([^"]*)"$`, func(uid, gid string) { r := w.runFixture(); r.uid, r.gid = uid, gid })
	sc.Step(`^the run preflight contract receives valid explicit argv$`, func() error { return w.runProbe("identity") })
	sc.Step(`^the run preflight contract reports "([^"]*)" with exit (\d+)$`, func(rule string, exit int) error {
		return contractProblem(object(w.run.contract["identity"]), rule, exit)
	})
	sc.Step(`^the run preflight contract made no engine call, snapshot write or startup handoff$`, func() error {
		r := object(w.run.contract["identity"])
		if r["EngineCalls"] != float64(0) || w.run.contract["snapshot_count"] != float64(0) {
			return fmt.Errorf("identity refusal performed I/O: %v", r)
		}
		return contractNoHandoff(r)
	})
	sc.Step(`^a run preflight contract with independently seeded approvals A and B$`, func() { w.runFixture() })
	sc.Step(`^approval B replaces A after run inspects A but before frozen selection and a fresh run selects B$`, func() error { return w.runProbe("superseded") })
	sc.Step(`^the fresh run publishes exactly B and returns its durable frozen handoff$`, func() error {
		return w.contractEach(func(c map[string]any) error { return w.contractHandoff(c, "second", "b", "2") })
	})
	sc.Step(`^the stale run reports "([^"]*)" with exit (\d+) and approve guidance$`, func(rule string, exit int) error {
		return w.contractEach(func(c map[string]any) error {
			r := object(c["first"])
			if err := contractProblem(r, rule, exit); err != nil {
				return err
			}
			if !strings.Contains(fmt.Sprint(r["Message"])+fmt.Sprint(r["Fix"]), "itos container approve") {
				return fmt.Errorf("missing approve guidance")
			}
			return nil
		})
	})
	sc.Step(`^the stale run published no snapshot and made no startup handoff$`, func() error {
		return w.contractEach(func(c map[string]any) error {
			r := object(c["first"])
			if object(r["Visible"])["Settings"] != "" {
				return fmt.Errorf("stale selection published")
			}
			return contractNoHandoff(r)
		})
	})
	sc.Step(`^neither contract run attempted engine mutation or image acquisition$`, w.contractInspections)
	sc.Step(`^run selects A, the fixture makes "([^"]*)" before publication and handoff, and a fresh run follows with B's approved inputs$`, func(change string) error { w.run.condition = change; return w.runProbe("independence") })
	sc.Step(`^the published snapshot and startup handoff contain exactly A's canonical bytes and approval identifiers$`, func() error {
		return w.contractEach(func(c map[string]any) error { return w.contractHandoff(c, "first", "a", "1") })
	})
	sc.Step(`^the snapshot and retained handoff are unchanged by the fixture's caller-input and returned-buffer mutations$`, func() error {
		return w.contractEach(func(c map[string]any) error {
			if !reflect.DeepEqual(c["retained"], object(c["first"])["Handoff"]) {
				return fmt.Errorf("retained handoff changed")
			}
			a, b := object(c["first"]), object(c["second"])
			if !reflect.DeepEqual(a["Visible"], b["Visible"]) || a["Bytes"] != b["Bytes"] || a["Inode"] != b["Inode"] || a["Mtime"] != b["Mtime"] {
				return fmt.Errorf("A snapshot changed")
			}
			return nil
		})
	})
	sc.Step(`^no checkout settings are read after frozen selection$`, func() error {
		return w.contractEach(func(c map[string]any) error {
			if object(c["first"])["AfterReads"] != float64(0) {
				return fmt.Errorf("read checkout after selection")
			}
			return nil
		})
	})
	sc.Step(`^a fresh run of approved B publishes B without changing A$`, func() error {
		return w.contractEach(func(c map[string]any) error { return w.contractHandoff(c, "second", "b", "2") })
	})
	sc.Step(`^a run snapshot contract with publication failure "([^"]*)"$`, func(f string) { w.runFixture().condition = f })
	sc.Step(`^approved run preflight attempts publication through each store implementation and fresh preflight retries after the fault is cleared$`, func() error { return w.runProbe("fault") })
	sc.Step(`^each first run reports "([^"]*)" with exit (\d+) and returns no durable handoff$`, func(rule string, exit int) error {
		return w.contractEach(func(c map[string]any) error {
			r := object(c["first"])
			if err := contractProblem(r, rule, exit); err != nil {
				return err
			}
			return contractNoHandoff(r)
		})
	})
	sc.Step(`^each first attempt leaves final snapshot "([^"]*)" and no partial snapshot is accepted$`, func(visible string) error {
		return w.contractEach(func(c map[string]any) error {
			r := object(c["first"])
			f := object(r["Visible"])
			if visible == "absent" && r["Bytes"] != "" {
				return fmt.Errorf("unexpected visible record")
			}
			if r["Bytes"] != "" && f["Settings"] != object(c["a"])["Canonical"] {
				return fmt.Errorf("partial/conflicting record visible")
			}
			return nil
		})
	})
	sc.Step(`^each retry returns exactly A's frozen handoff only after durable snapshot publication$`, func() error {
		return w.contractEach(func(c map[string]any) error {
			if err := w.contractHandoff(c, "second", "a", "1"); err != nil {
				return err
			}
			events := object(c["second"])["Events"]
			for _, event := range []string{"file fsync", "parent directory fsync", "new ancestor fsync"} {
				if !strings.Contains(fmt.Sprint(events), event) {
					return fmt.Errorf("retry missing durability %s: %v", event, events)
				}
			}
			return nil
		})
	})
	sc.Step(`^any complete snapshot left by the first attempt is reused without changing its bytes, inode or mtime$`, func() error {
		return w.contractEach(func(c map[string]any) error {
			a, b := object(c["first"]), object(c["second"])
			if a["Bytes"] != "" && (a["Bytes"] != b["Bytes"] || a["Inode"] != b["Inode"] || a["Mtime"] != b["Mtime"]) {
				return fmt.Errorf("retry rewrote complete snapshot")
			}
			return nil
		})
	})
	sc.Step(`^active approval, prior snapshots and outside targets remain unchanged$`, func() error {
		return w.contractEach(func(c map[string]any) error {
			if !reflect.DeepEqual(c["protected_before"], c["protected_after"]) {
				return fmt.Errorf("unrelated state changed")
			}
			return nil
		})
	})
	sc.Step(`^no failed attempt made a later startup call$`, func() error {
		return w.contractEach(func(c map[string]any) error { return contractNoHandoff(object(c["first"])) })
	})
	sc.Step(`^neither attempt made an engine mutation or image acquisition call$`, w.contractInspections)
}
func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func contractProblem(r map[string]any, rule string, exit int) error {
	if r["Rule"] != rule || r["Exit"] != float64(exit) {
		return fmt.Errorf("contract expected %s/%d, got %v", rule, exit, r)
	}
	return nil
}
func contractNoHandoff(r map[string]any) error {
	h := object(r["Handoff"])
	if h["Reference"] != "" || object(h["Frozen"])["Settings"] != "" {
		return fmt.Errorf("refusal returned usable handoff: %v", h)
	}
	return nil
}
func (w *world) contractEach(check func(map[string]any) error) error {
	for _, key := range []string{"memory", "disk"} {
		c := object(w.run.contract[key])
		if c == nil {
			return fmt.Errorf("missing %s observations", key)
		}
		if err := check(c); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	return nil
}
func (w *world) contractHandoff(c map[string]any, result, candidate, generation string) error {
	r, a := object(c[result]), object(c[candidate])
	h := object(r["Handoff"])
	f := object(h["Frozen"])
	// Construct the canonical settings independently of the product codec.
	uid := strconv.Itoa(os.Geteuid())
	checkout := map[string]any{"root": a["Root"], "device": "1", "inode": "2", "git_device": "1", "git_inode": "3"}
	env := map[string]string{}
	if candidate == "b" {
		env["APP"] = "B"
	}
	settings := map[string]any{"schema": 1, "profile": "offline-v1", "host": map[string]string{"uid": uid, "gid": "1000"}, "checkout": checkout, "engine": map[string]string{"endpoint": "unix:///engine.sock", "id": "engine-A", "os": "linux", "architecture": "amd64"}, "policy": map[string]any{"version": 1, "manifest": "worker.json", "network": map[string]string{"mode": "offline"}}, "manifest": map[string]any{"name": nil, "image": doctorImage, "workspaceFolder": "/workspace", "containerEnv": env, "containerUser": uid, "remoteUser": uid}, "image": map[string]string{"reference": doctorImage, "id": doctorImageID, "os": "linux", "architecture": "amd64"}}
	project := approvalHash(map[string]any{"schema": 1, "uid": uid, "checkout": checkout})
	if r["Exit"] != float64(0) || h["Reference"] == "" || f["Settings"] != string(approvalJSON(settings)) || f["Fingerprint"] != "sha256:"+approvalHash(settings) || f["Project"] != project || f["Generation"] != generation {
		return fmt.Errorf("missing exact durable %s handoff: %v", candidate, r)
	}
	if !reflect.DeepEqual(r["Published"], h["Frozen"]) {
		return fmt.Errorf("snapshot differs from handoff")
	}
	return nil
}
func (w *world) contractInspections() error {
	return w.contractEach(func(c map[string]any) error {
		for _, key := range []string{"first", "second"} {
			r := object(c[key])
			if r["EngineCalls"] != float64(1) {
				return fmt.Errorf("%s did not freshly inspect", key)
			}
		}
		return nil
	})
}
func (w *world) runProbe(kind string) error {
	bin, err := w.sessionProbeBinary()
	if err != nil {
		return err
	}
	path := filepath.Join(w.dir, "slice-8-contract.json")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	args := []string{"-test.run=^TestRunPreflightContractProbe$", "-test.count=1"}
	if cover := w.coverageDir(); cover != "" {
		args = append(args, "-test.gocoverdir="+cover)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = w.root
	cmd.Env = w.coverageEnv(os.Environ())
	for key, value := range map[string]string{"SLICE_8_PROBE_OUTPUT": path, "SLICE_8_PROBE_KIND": kind, "SLICE_8_PROBE_CONDITION": w.run.condition, "SLICE_8_PROBE_UID": w.run.uid, "SLICE_8_PROBE_GID": w.run.gid} {
		cmd.Env = setEnv(cmd.Env, key, value)
	}
	data, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("contract fixture: %w\n%s", err, data)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &w.run.contract)
}
