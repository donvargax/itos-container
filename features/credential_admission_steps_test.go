package features

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

type credentialAdmissionFixture struct {
	request, command, component, checkpoint, boundary string
	prior                                             string
	freeze                                            map[string]any
	results                                           []map[string]any
	wants                                             []map[string]any
}

func (w *world) credentialAdmissionSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a credential admission observation fixture requesting "([^"]*)" with independent matching policy and source-checkout pins$`, func(request string) {
		w.credentialApproval = &credentialApprovalWorld{admission: &credentialAdmissionFixture{request: request}}
	})
	sc.Step(`^the admission contract runs "([^"]*)" with matching pins then with a "([^"]*)" disagreement at "([^"]*)"$`, func(command, component, checkpoint string) error {
		f := w.credentialApproval.admission
		f.command, f.component, f.checkpoint = command, component, checkpoint
		if err := w.credentialAdmissionRun(command, "", "", "", "", 0); err != nil {
			return err
		}
		return w.credentialAdmissionRun(command, component, checkpoint, "", "", 0)
	})
	sc.Step(`^the mismatching admission reports "([^"]*)" with exit 1 and no usable approval candidate$`, func(rule string) error { return w.credentialAdmissionRefusal(rule, 1) })
	sc.Step(`^the matching control succeeds with exactly the independently calculated grants$`, func() error { return w.credentialAdmissionControls(1) })
	sc.Step(`^the disagreement journal identifies the policy pin and observed checkout node at the specified checkpoint$`, func() error {
		f := w.credentialApproval.admission
		r := f.results[1]
		p, n := object(r["Pin"]), object(r["Observed"])
		key := "Inode"
		if f.component == "device" {
			key = "Device"
		}
		if fmt.Sprint(p[key]) == fmt.Sprint(n[key]) {
			return fmt.Errorf("journal did not retain the cross-pin disagreement")
		}
		return nil
	})
	sc.Step(`^the mismatching admission publishes neither approval nor frozen input and reads no source data$`, w.credentialAdmissionNoPublication)
	sc.Step(`^the mismatching admission performs no engine access or consent after detecting the disagreement$`, func() error {
		f := w.credentialApproval.admission
		c := object(f.results[len(f.results)-1]["Calls"])
		engines, prompts := float64(0), float64(0)
		if f.checkpoint == "after engine inspection" || f.checkpoint == "after terminal consent" {
			engines = 1
		}
		if f.checkpoint == "after terminal consent" {
			prompts = 1
		}
		if c["engine opens"] != engines || c["prompts"] != prompts {
			return fmt.Errorf("admission continued past the root disagreement")
		}
		return nil
	})
	sc.Step(`^the admission contract runs "([^"]*)" with the "([^"]*)" path at 4095 bytes then at 4096 bytes$`, func(command, boundary string) error {
		f := w.credentialApproval.admission
		f.command, f.boundary = command, boundary
		for _, length := range []int{4095, 4096} {
			if err := w.credentialAdmissionRun(command, "", "", boundary, "", length); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^the 4096-byte admission reports "([^"]*)" with exit 2 and both its message and fix name the boundary and the 4095-byte limit$`, func(rule string) error {
		if err := w.credentialAdmissionRefusal(rule, 2); err != nil {
			return err
		}
		f := w.credentialApproval.admission
		r := f.results[1]
		text := credentialAdmissionProblemText(r, rule)
		boundary := "root"
		if f.boundary == "selected state" {
			boundary = "state"
		}
		for _, part := range []string{"message", "fix"} {
			v := text[part]
			if !strings.Contains(v, "4095") || !strings.Contains(strings.ToLower(v), boundary) {
				return fmt.Errorf("%s does not name the %s 4095-byte bound", part, boundary)
			}
		}
		return nil
	})
	sc.Step(`^the 4095-byte control succeeds with exactly the independently calculated grants$`, func() error { return w.credentialAdmissionControls(1) })
	sc.Step(`^the overlong path is refused from its text before any boundary or source observation and before engine access or consent$`, w.credentialAdmissionNoObservation)
	sc.Step(`^the overlong-path call journal follows the specified root or state validation order$`, func() error {
		f := w.credentialApproval.admission
		c := object(f.results[1]["Calls"])
		want := float64(0)
		if f.boundary == "selected state" {
			want = 1
		}
		if c["policy pins"] != want || c["policy reads"] != want {
			return fmt.Errorf("overlong boundary validation used the wrong policy ordering")
		}
		return nil
	})
	sc.Step(`^the 4096-byte admission publishes neither approval nor frozen input and reads no source data$`, w.credentialAdmissionNoPublication)
	sc.Step(`^the admission contract runs "([^"]*)" with ext4 btrfs xfs then "([^"]*)" at "([^"]*)" in the "([^"]*)" mount observation$`, func(command, filesystem, placement, observation string) error {
		f := w.credentialApproval.admission
		f.command = command
		for _, name := range []string{"ext4", "btrfs", "xfs", filesystem} {
			if err := w.credentialAdmissionRun(command, "", "", placement+"|"+observation, name, 0); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^only ext4 btrfs and xfs succeed and the other filesystem reports "credential.filesystem" at "/credentials/0/source" with exit 3$`, func() error {
		if err := w.credentialAdmissionControls(3); err != nil {
			return err
		}
		return w.credentialAdmissionRefusal("credential.filesystem", 3)
	})
	sc.Step(`^the filesystem refusal fix names ext4 btrfs and xfs for source checkout and state paths$`, func() error {
		r := w.credentialApproval.admission.results[3]
		fix := credentialAdmissionProblemText(r, "credential.filesystem")["fix"]
		for _, name := range []string{"ext4", "btrfs", "xfs", "source", "checkout", "state"} {
			if !strings.Contains(strings.ToLower(fix), name) {
				return fmt.Errorf("filesystem fix omits %s", name)
			}
		}
		return nil
	})
	sc.Step(`^each accepted control reports exactly the independently calculated grants$`, func() error { return w.credentialAdmissionControls(3) })
	sc.Step(`^the filesystem refusal retains the requested grants without a usable approval candidate$`, func() error {
		f := w.credentialApproval.admission
		r := f.results[3]
		var obj map[string]any
		if err := json.Unmarshal([]byte(r["Stdout"].(string)), &obj); err == nil {
			if !reflect.DeepEqual(obj["credentials"], f.wants[3]["credentials"]) {
				return fmt.Errorf("filesystem refusal dropped requested grants")
			}
			return nil
		}
		for _, raw := range f.wants[3]["credentials"].([]any) {
			g := object(raw)
			if !strings.Contains(r["Stdout"].(string), g["source"].(string)) {
				return fmt.Errorf("plain refusal dropped requested source")
			}
		}
		return nil
	})
	sc.Step(`^the filesystem refusal publishes neither approval nor frozen input and reads no source data$`, w.credentialAdmissionNoPublication)
	sc.Step(`^the filesystem refusal precedes engine access and consent without enumerating state records$`, func() error {
		c := object(w.credentialApproval.admission.results[3]["Calls"])
		for _, name := range []string{"engine opens", "prompts", "state enumeration", "state metadata"} {
			if n, _ := c[name].(float64); n != 0 {
				return fmt.Errorf("filesystem refusal performed %s", name)
			}
		}
		return nil
	})
}

func (w *world) credentialAdmissionRun(command, component, checkpoint, boundary, filesystem string, length int) error {
	f := w.credentialApproval.admission
	root, state := "/checkout", "/state"
	if length != 0 {
		if boundary == "effective root" {
			root = credentialAdmissionPath(length)
		} else {
			state = credentialAdmissionPath(length)
		}
	}
	o := credentialAdmissionNodes(root, state)
	if filesystem != "" {
		credentialAdmissionFilesystem(o, boundary, filesystem, root, state)
	}
	want := credentialAdmissionSettings(f.request, root)
	policy := map[string]any{"version": 3, "manifest": "worker.json", "network": map[string]any{"mode": "offline"}}
	if grants, ok := want["credentials"]; ok {
		var requests []map[string]any
		for _, raw := range grants.([]any) {
			g := object(raw)
			requests = append(requests, map[string]any{"kind": g["kind"], "source": g["source"]})
		}
		policy["credentials"] = requests
	}
	request := map[string]any{"Argv": command, "Root": root, "State": state, "Policy": string(approvalJSON(policy)), "Manifest": `{"image":"worker.test/agent@sha256:` + strings.Repeat("a", 64) + `"}`, "Files": o, "Pin": map[string]string{"Root": root, "Device": "101", "Inode": "200", "GitDevice": "101", "GitInode": "201"}, "Component": component, "Checkpoint": checkpoint}
	if command == "preflight" {
		request["Approved"] = string(approvalJSON(credentialAdmissionSettings(f.prior, root)))
	}
	bin, err := w.sessionProbeBinary()
	if err != nil {
		return err
	}
	input, output := filepath.Join(w.dir, "slice-30-admission-input.json"), filepath.Join(w.dir, "slice-30-admission-output.json")
	if err := os.WriteFile(input, approvalJSON(request), 0600); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := []string{"-test.run=^TestCredentialAdmissionProcess$", "-test.count=1"}
	if dir := w.coverageDir(); dir != "" {
		args = append(args, "-test.gocoverdir="+dir)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = w.root
	cmd.Env = setEnv(setEnv(w.coverageEnv(os.Environ()), "SLICE_30_ADMISSION_INPUT", input), "SLICE_30_ADMISSION_OUTPUT", output)
	if data, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("admission assembly: %w\n%s", err, data)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		return err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return err
	}
	f.results = append(f.results, result)
	f.wants = append(f.wants, want)
	return nil
}

func (w *world) credentialAdmissionRefusal(rule string, exit int) error {
	f := w.credentialApproval.admission
	r := f.results[len(f.results)-1]
	if r["Exit"] != float64(exit) || !strings.Contains(r["Stdout"].(string)+r["Stderr"].(string), rule) {
		return fmt.Errorf("admission did not refuse %s with exit %d", rule, exit)
	}
	report := object(r["Report"])
	if report["fingerprint"] != "" || report["effective"] != nil {
		return fmt.Errorf("refused admission retained a usable candidate")
	}
	return nil
}

func (w *world) credentialAdmissionControls(count int) error {
	f := w.credentialApproval.admission
	if len(f.results) < count+1 {
		return fmt.Errorf("missing independent admission controls")
	}
	for i, r := range f.results[:count] {
		if r["Exit"] != float64(0) {
			return fmt.Errorf("admission positive control %d did not succeed: %s %s", i, r["Stdout"], r["Stderr"])
		}
		if strings.Contains(f.command, "--credentials-only") {
			var obj map[string]any
			if err := json.Unmarshal([]byte(r["Stdout"].(string)), &obj); err != nil {
				return err
			}
			if !reflect.DeepEqual(obj["credentials"], f.wants[i]["credentials"]) {
				return fmt.Errorf("source control grants differ")
			}
		} else if !reflect.DeepEqual(object(r["Report"])["effective"], f.wants[i]) || object(r["Report"])["fingerprint"] != "sha256:"+approvalHash(f.wants[i]) {
			return fmt.Errorf("admission control %d candidate differs from independent oracle", i)
		}
	}
	return nil
}

func credentialAdmissionProblemText(r map[string]any, rule string) map[string]string {
	var obj map[string]any
	_ = json.Unmarshal([]byte(r["Stdout"].(string)), &obj)
	problems, _ := obj["problems"].([]any)
	if len(problems) == 0 {
		problems, _ = r["Problems"].([]any)
	}
	for _, raw := range problems {
		p := object(raw)
		if p["rule"] == rule {
			return map[string]string{"message": fmt.Sprint(p["message"]), "fix": fmt.Sprint(p["fix"])}
		}
	}
	return nil
}

func (w *world) credentialAdmissionNoPublication() error {
	f := w.credentialApproval.admission
	c := object(f.results[len(f.results)-1]["Calls"])
	for _, name := range []string{"approval writes", "frozen publications", "source data reads"} {
		if n, _ := c[name].(float64); n != 0 {
			return fmt.Errorf("refusal performed %s", name)
		}
	}
	return nil
}

func (w *world) credentialAdmissionNoObservation() error {
	f := w.credentialApproval.admission
	c := object(f.results[1]["Calls"])
	for _, name := range []string{"source metadata", "checkout traversal", "state mount-table", "checkout inspections", "engine opens", "prompts"} {
		if n, _ := c[name].(float64); n != 0 {
			return fmt.Errorf("overlong path performed %s", name)
		}
	}
	return nil
}

func credentialAdmissionPath(length int) string {
	var b strings.Builder
	for b.Len() < length {
		n := min(200, length-b.Len()-1)
		if n == 0 {
			n = 1
		}
		b.WriteByte('/')
		b.WriteString(strings.Repeat("a", n))
	}
	return b.String()[:length]
}

func credentialAdmissionNodes(root, state string) *credentialObservation {
	o := &credentialObservation{Nodes: map[string]credentialNode{}, Mounts: []string{"1 0 0:1 / / rw - ext4 disk rw\n", "1 0 0:1 / / rw - ext4 disk rw\n"}}
	inode := uint64(10)
	for _, path := range []string{root, state, "/account", "/vault"} {
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		current := ""
		for _, part := range append([]string{""}, parts...) {
			current = strings.TrimSuffix(current, "/") + "/" + part
			if _, ok := o.Nodes[current]; !ok {
				o.Nodes[current] = credentialNode{Device: 101, Inode: inode, Links: 1, UID: 1000, Mode: 0040700, MountID: 1, Path: current}
				inode++
			}
		}
	}
	n := o.Nodes["/"]
	n.UID, n.Mode = 0, 0040755
	o.Nodes["/"] = n
	n = o.Nodes[root]
	n.Inode = 200
	o.Nodes[root] = n
	o.Nodes[root+"/.git"] = credentialNode{Device: 101, Inode: 201, Links: 1, UID: 1000, Mode: 0040700, MountID: 1, Path: root + "/.git"}
	for i, name := range []string{"claude-a", "claude-b", "github-a"} {
		path := "/vault/" + name
		o.Nodes[path] = credentialNode{Device: 101, Inode: uint64(300 + i), Links: 1, UID: 1000, Mode: 0100600, Size: 57, MountID: 1, Path: path}
	}
	return o
}

func credentialAdmissionFilesystem(o *credentialObservation, boundary, filesystem, root, state string) {
	placement, observation, _ := strings.Cut(boundary, "|")
	point := "/vault/claude-a"
	switch placement {
	case "source":
	case "source ancestor":
		point = "/vault"
	case "checkout root", "covered checkout row":
		point = root
	case "checkout descendant":
		point = root + "/subtree"
	case "selected state root", "absent selected state root":
		point = state
	case "selected state descendant":
		point = state + "/subtree"
	case "fixed account state root", "covered fixed state row":
		point = "/account/.local/state/itos-container"
	case "fixed state descendant":
		point = "/account/.local/state/itos-container/subtree"
	}
	covered := strings.HasPrefix(placement, "covered ")
	for i := range o.Mounts {
		typeName := "ext4"
		if observation == "first" && i == 0 || observation == "final" && i == 1 {
			typeName = filesystem
		}
		o.Mounts[i] += "2 1 0:2 / " + point + " rw - " + typeName + " disk rw\n"
		if covered {
			o.Mounts[i] += "3 1 0:3 / " + point + " rw - ext4 disk rw\n"
		}
	}
	if placement == "absent selected state root" {
		delete(o.Nodes, state)
	}
	for path, n := range o.Nodes {
		if path == point || strings.HasPrefix(path, point+"/") {
			n.MountID = 2
			if covered {
				n.MountID = 3
			}
			o.Nodes[path] = n
		}
	}
}

func credentialAdmissionSettings(request, root string) map[string]any {
	s := map[string]any{"schema": 1, "profile": "offline-v1", "host": map[string]string{"uid": "1000", "gid": "1000"}, "checkout": map[string]string{"root": root, "device": "101", "inode": "200", "git_device": "101", "git_inode": "201"}, "engine": map[string]string{"endpoint": "unix:///engine.sock", "id": "engine-A", "os": "linux", "architecture": "amd64"}, "policy": map[string]any{"version": 3, "manifest": "worker.json", "network": map[string]string{"mode": "offline"}}, "manifest": map[string]any{"name": nil, "image": "worker.test/agent@sha256:" + strings.Repeat("a", 64), "workspaceFolder": "/workspace", "containerEnv": map[string]string{}, "containerUser": "1000", "remoteUser": "1000"}, "image": map[string]string{"reference": "worker.test/agent@sha256:" + strings.Repeat("a", 64), "id": "sha256:" + strings.Repeat("b", 64), "os": "linux", "architecture": "amd64"}}
	var grants []map[string]string
	add := func(kind, name string) {
		grants = append(grants, map[string]string{"kind": kind, "source": "/vault/" + name, "target": "/run/itos-container/credentials/" + kind})
	}
	switch request {
	case "none":
	case "claude":
		add("claude-token", "claude-a")
	case "github":
		add("github-token", "github-a")
	case "both":
		add("claude-token", "claude-a")
		add("github-token", "github-a")
	case "repointed claude":
		add("claude-token", "claude-b")
	}
	if len(grants) > 0 {
		s["profile"] = "offline-credentials-v1"
		s["credentials"] = grants
	}
	return approvalClone(s)
}
