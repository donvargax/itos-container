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

// Raw observations cross the test-process boundary. They carry identities and
// kernel text, never an expected credential verdict. The product command must
// choose the inspection scope and the production observer must judge the data.
type credentialObservation struct {
	Nodes                  map[string]credentialNode
	Mounts                 []string
	ReadErrors             map[string]bool
	MountErrors            map[int]bool
	DescriptorErrors       map[string]bool
	FinalNodes             map[string]credentialNode
	Activity               string
	Retained               int
	Deadline               bool
	Policy, Manifest, Argv string
	Calls                  map[string]int
	Completed              string
}

type credentialNode struct {
	Device, Inode, Links uint64
	UID, Mode            uint32
	Size                 int64
	MountID              int
	Path                 string
}

func (w *world) credentialObservationSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a credential observation fixture for "([^"]*)" with "([^"]*)"$`, w.credentialObservationFixture)
	sc.Step(`^credential observation includes a valid selected manifest$`, func() error {
		w.credentials.observation.Manifest = manifestBytes
		return nil
	})
	sc.Step(`^the credential observation contract runs in JSON mode$`, func() error {
		return w.runCredentialObservation("doctor --credentials-only --json")
	})
	sc.Step(`^the credential observation contract runs "([^"]*)"$`, w.runCredentialObservation)
	sc.Step(`^credential observation schedules unrelated state activity "([^"]*)"$`, func(activity string) error {
		switch activity {
		case "approval publication", "intent publication", "snapshot publication", "session record cleanup", "publication and disappearing temp entries":
			w.credentials.observation.Activity = activity
			return nil
		default:
			return fmt.Errorf("unknown state activity %q", activity)
		}
	})
	sc.Step(`^the credential observation state retains (\d+) unrelated entries$`, func(count int) error {
		w.credentials.observation.Retained = count
		return nil
	})
	sc.Step(`^credential observation has namespace evidence fault "([^"]*)"$`, w.credentialNamespaceFault)
	sc.Step(`^the credential observation policy version is (\d+) with no credentials section$`, func(version int) error {
		w.credentials.grants = nil
		return w.credentialObservationPolicy(version)
	})
	sc.Step(`^the credential observation recorded "([^"]*)" calls$`, func(operation string) error {
		if w.credentials.observation.Calls[operation] == 0 {
			return fmt.Errorf("no %s calls recorded", operation)
		}
		return nil
	})
	sc.Step(`^the credential observation recorded no source metadata checkout traversal or state mount-table calls$`, func() error {
		return w.credentialNoCalls("source metadata", "checkout traversal", "state mount-table")
	})
	sc.Step(`^the credential observation recorded no state record enumeration or metadata visits$`, func() error {
		return w.credentialNoCalls("state enumeration", "state metadata")
	})
	sc.Step(`^the credential observation completed the scheduled state activity during inspection$`, func() error {
		o := w.credentials.observation
		if o.Completed == "" || o.Completed != o.Activity || o.Calls["state mount-table"] == 0 {
			return fmt.Errorf("scheduled state activity did not run inside the mount observation")
		}
		return nil
	})
	sc.Step(`^the credential observation left all (\d+) unrelated entries unvisited$`, func(count int) error {
		if w.credentials.observation.Retained != count {
			return fmt.Errorf("fixture does not contain %d retained entries", count)
		}
		return w.credentialNoCalls("state enumeration", "state metadata")
	})
	sc.Step(`^its credential observation JSON reports a valid "([^"]*)" inspection$`, w.credentialObservationScope)
}

func (w *world) credentialNoCalls(operations ...string) error {
	for _, operation := range operations {
		if count := w.credentials.observation.Calls[operation]; count != 0 {
			return fmt.Errorf("unexpected %s calls: %d", operation, count)
		}
	}
	return nil
}

func (w *world) credentialObservationFixture(boundary, layout string) error {
	root := "/checkout"
	if boundary == "protected state" {
		root = "/state"
	} else if boundary != "checkout" {
		return fmt.Errorf("unknown protected boundary %q", boundary)
	}
	o := &credentialObservation{
		Nodes: map[string]credentialNode{}, FinalNodes: map[string]credentialNode{},
		ReadErrors: map[string]bool{}, MountErrors: map[int]bool{}, DescriptorErrors: map[string]bool{},
	}
	next := uint64(1)
	dir := func(path string) {
		o.Nodes[path] = credentialNode{Device: 1, Inode: next, Links: 1, UID: 1000, Mode: 0040700, MountID: 1, Path: path}
		next++
	}
	for _, path := range []string{"/", "/checkout", "/state", "/checkout-sources", root + "/private"} {
		dir(path)
	}
	n := o.Nodes["/"]
	n.UID, n.Mode = 0, 0040755
	o.Nodes["/"] = n
	source := "/checkout-sources/token"
	o.Nodes[source] = credentialNode{Device: 1, Inode: next, Links: 1, UID: 1000, Mode: 0100600, Size: 32, MountID: 1, Path: source}
	table := "1 0 0:1 / / rw - tmpfs tmpfs rw\n"
	switch layout {
	case "distinct outside file":
	case "unreadable descendant":
		dir("/checkout/unreadable")
		o.ReadErrors["/checkout/unreadable"] = true
	case "unavailable mount table":
		o.MountErrors[0], o.MountErrors[1] = true, true
	case "root directory bound outside", "descendant directory bound outside", "single descendant file bound outside",
		"outside directory bound into a descendant", "outside file bound into a descendant",
		"descendant on another device bound outside", "descendant with escaped mount paths bound outside", "covered state backing directory bound outside":
		base := root + "/private"
		if layout == "root directory bound outside" {
			base = root
		}
		outside := "/checkout-sources"
		if layout == "descendant with escaped mount paths bound outside" {
			outside = "/outside space"
			dir(outside)
			n := o.Nodes[source]
			delete(o.Nodes, source)
			source = outside + "/token"
			n.Path = source
			o.Nodes[source] = n
		}
		n := o.Nodes[source]
		alias := base + "/token"
		n.Path = alias
		o.Nodes[alias] = n
		if strings.Contains(layout, "directory") || layout == "root directory bound outside" || strings.Contains(layout, "another device") || strings.Contains(layout, "escaped") {
			n = o.Nodes[base]
			n.Path = outside
			o.Nodes[outside] = n
		}
		escape := func(s string) string { return strings.ReplaceAll(s, " ", `\040`) }
		mountpoint, mountroot := outside, base
		if strings.Contains(layout, "single descendant file") {
			mountpoint, mountroot = source, alias
		}
		if strings.HasPrefix(layout, "outside") {
			mountpoint, mountroot = base, outside
			if strings.Contains(layout, "file") {
				mountpoint, mountroot = alias, source
			}
		}
		device := uint64(1)
		if strings.Contains(layout, "another device") {
			device = 2
			table += "3 1 0:2 /private /state/private rw - tmpfs tmpfs rw\n"
			mountroot = "/private"
			for path, node := range o.Nodes {
				if path == base || strings.HasPrefix(path, base+"/") {
					node.Device, node.MountID = device, 3
					o.Nodes[path] = node
				}
			}
		}
		table += fmt.Sprintf("2 1 0:%d %s %s rw - tmpfs tmpfs rw\n", device, escape(mountroot), escape(mountpoint))
		for path, node := range o.Nodes {
			if path == mountpoint || strings.HasPrefix(path, mountpoint+"/") {
				node.MountID, node.Device = 2, device
				o.Nodes[path] = node
			}
		}
		if layout == "covered state backing directory bound outside" {
			table += "3 1 0:2 / /state/private rw - tmpfs tmpfs rw\n"
			for path, node := range o.Nodes {
				if path == base || strings.HasPrefix(path, base+"/") {
					node.MountID, node.Device, node.Inode = 3, 2, node.Inode+100
					o.Nodes[path] = node
				}
			}
		}
	default:
		return fmt.Errorf("unknown credential observation layout %q", layout)
	}
	o.Mounts = []string{table, table}
	w.credentials = &credentialWorld{observation: o, paths: map[string]string{"token": source}, grants: []credentialGrant{{Kind: "claude-token", Source: source}}}
	return w.credentialObservationPolicy(3)
}

func (w *world) credentialObservationPolicy(version int) error {
	p := map[string]any{"version": version, "manifest": ".devcontainer/devcontainer.json", "network": map[string]string{"mode": "offline"}}
	if len(w.credentials.grants) > 0 {
		p["credentials"] = w.credentials.grants
	}
	data, err := json.Marshal(p)
	w.credentials.observation.Policy = string(data)
	return err
}

func (w *world) credentialNamespaceFault(fault string) error {
	o := w.credentials.observation
	source := w.credentials.paths["token"]
	switch fault {
	case "mount table unreadable":
		o.MountErrors[0] = true
	case "final mount table unreadable":
		o.MountErrors[1] = true
	case "truncated final mount row":
		o.Mounts[1] = strings.TrimSuffix(o.Mounts[1], "\n")
	case "malformed mount row":
		o.Mounts[0] = "malformed\n"
	case "duplicate mount ID":
		o.Mounts[0] += o.Mounts[0]
	case "descriptor mount row missing":
		n := o.Nodes[source]
		n.MountID = 100
		o.Nodes[source] = n
	case "descriptor device disagrees":
		n := o.Nodes[source]
		n.Device = 99
		o.Nodes[source] = n
	case "source pathname now names another inode":
		n := o.Nodes[source]
		n.Inode++
		o.FinalNodes[source] = n
	case "source mount mapping changed":
		o.Mounts[1] = "1 0 0:1 /changed / rw - tmpfs tmpfs rw\n"
	case "16385 mount rows":
		var table strings.Builder
		for i := 1; i <= 16385; i++ {
			fmt.Fprintf(&table, "%d 0 0:1 / / rw - tmpfs tmpfs rw\n", i)
		}
		o.Mounts[0] = table.String()
	case "mount table exceeds 8 MiB":
		o.Mounts[0] += strings.Repeat("2 1 0:1 / /elsewhere rw - tmpfs tmpfs rw\n", 230000)
	case "mount row exceeds 65535 bytes":
		o.Mounts[0] = "1 0 0:1 / / rw " + strings.Repeat("x", 65536) + " - tmpfs tmpfs rw\n"
	case "fdinfo mount ID unavailable":
		o.DescriptorErrors[source] = true
	case "observation deadline reached":
		o.Deadline = true
	default:
		return fmt.Errorf("unknown namespace evidence fault %q", fault)
	}
	return nil
}

func (w *world) runCredentialObservation(argv string) error {
	bin, err := w.sessionProbeBinary()
	if err != nil {
		return err
	}
	o := w.credentials.observation
	o.Argv, o.Calls, o.Completed = argv, nil, ""
	data, err := json.Marshal(o)
	if err != nil {
		return err
	}
	w.stdin = string(data)
	defer func() { w.stdin = "" }()
	log := filepath.Join(w.dir, "slice-29-observation.json")
	env := setEnv(setEnv(w.env(), "SLICE_29_CONTRACT", "1"), "SLICE_29_OBSERVATIONS", log)
	if err := w.runEnv(w.dir, env, bin, "-test.run=^TestCredentialContractProcess$"); err != nil {
		return err
	}
	data, err = os.ReadFile(log)
	if err != nil {
		return err
	}
	var observed struct {
		Calls     map[string]int
		Completed string
	}
	if err := json.Unmarshal(data, &observed); err != nil {
		return err
	}
	o.Calls, o.Completed = observed.Calls, observed.Completed
	return nil
}

func (w *world) credentialObservationScope(scope string) error {
	object, err := w.json()
	if err != nil {
		return err
	}
	if object["schema"] != float64(1) || object["scope"] != scope || object["ok"] != true || !reflect.DeepEqual(object["problems"], []any{}) {
		return fmt.Errorf("unexpected %s envelope: %s", scope, w.stdout)
	}
	if scope == "policy" {
		return w.effectivePolicy(&godog.DocString{Content: w.credentials.observation.Policy})
	}
	if scope != "manifest" {
		return fmt.Errorf("unknown scope %q", scope)
	}
	want := map[string]any{
		"valid": true, "path": ".devcontainer/devcontainer.json", "unused": []any{},
		"image_metadata": map[string]any{"checked": false, "present": nil, "unused": []any{}},
		"effective": map[string]any{"image": manifestImage, "workspaceFolder": "/workspace", "name": nil,
			"containerEnv": map[string]any{}, "containerUser": nil, "remoteUser": nil},
	}
	if !reflect.DeepEqual(object["manifest"], want) {
		return fmt.Errorf("unexpected effective manifest: %s", w.stdout)
	}
	return nil
}
