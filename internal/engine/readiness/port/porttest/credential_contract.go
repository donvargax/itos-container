package porttest

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// CredentialFixture is a private source on the same device as two independent
// protected roots. All observations are raw metadata, not credential decisions.
func CredentialFixture() *CredentialFiles {
	f := &CredentialFiles{Nodes: map[string]CredentialEntry{}, FinalNodes: map[string]CredentialEntry{}, ReadErrors: map[string]bool{}, MountErrors: map[int]bool{}, DescriptorErrors: map[string]bool{}, OpenErrors: map[string]port.CredentialRead{}, StatErrors: map[string]bool{}, FinalStats: map[string]port.CredentialNode{}}
	for i, path := range []string{"/", "/checkout", "/state", "/vault", "/vault/token"} {
		n := CredentialEntry{CredentialNode: port.CredentialNode{Device: 1, Inode: uint64(i + 1), Links: 1, UID: 1000, Mode: 0040700, Size: 32}, MountID: 1, Path: path}
		if path == "/" {
			n.UID, n.Mode = 0, 0040755
		}
		if path == "/vault/token" {
			n.Mode = 0100600
		}
		f.Nodes[path] = n
	}
	f.MountsText = []string{"1 0 0:1 / / rw - tmpfs tmpfs rw\n", "1 0 0:1 / / rw - tmpfs tmpfs rw\n"}
	return f
}

// CredentialContract judges the same validator against each observation
// implementation. The factory supplies the real domain; this test support stays
// below it and has no I/O or dependency on a production domain or adapter.
func CredentialContract(t Reporter, observe func(*CredentialFiles, []policy.Credential, port.CredentialInput) port.CredentialResult) {
	t.Helper()
	input := port.CredentialInput{Root: "/checkout", State: "/state", AccountState: "/state", OS: "linux", Architecture: "amd64", UID: 1000}
	grants := []policy.Credential{{Kind: "claude-token", Source: "/vault/token"}}
	type testCase struct {
		name, rule string
		change     func(*CredentialFiles)
	}
	node := func(path string, change func(*port.CredentialNode)) func(*CredentialFiles) {
		return func(f *CredentialFiles) { n := f.Nodes[path]; change(&n.CredentialNode); f.Nodes[path] = n }
	}
	cases := []testCase{
		{"independent", "", func(*CredentialFiles) {}},
		{"source missing", "credential.missing", func(f *CredentialFiles) { delete(f.Nodes, "/vault/token") }},
		{"inaccessible component", "credential.unavailable", func(f *CredentialFiles) { f.OpenErrors["/vault"] = port.CredentialReadUnavailable }},
		{"source metadata unavailable", "credential.unavailable", func(f *CredentialFiles) { f.StatErrors["/vault/token"] = true }},
		{"symlink", "credential.symlink", node("/vault/token", func(n *port.CredentialNode) { n.Mode = 0120777 })},
		{"symlink before unsafe ancestor", "credential.symlink", func(f *CredentialFiles) {
			n := f.Nodes["/vault"]
			n.UID = 2000
			f.Nodes["/vault"] = n
			n = f.Nodes["/vault/token"]
			n.Mode = 0120777
			f.Nodes["/vault/token"] = n
		}},
		{"foreign ancestor", "credential.ancestor", node("/vault", func(n *port.CredentialNode) { n.UID = 2000 })},
		{"writable ancestor", "credential.ancestor", node("/vault", func(n *port.CredentialNode) { n.Mode = 0040720 })},
		{"root sticky ancestor", "", node("/vault", func(n *port.CredentialNode) { n.UID, n.Mode = 0, 0041777 })},
		{"user sticky ancestor", "credential.ancestor", node("/vault", func(n *port.CredentialNode) { n.Mode = 0041777 })},
		{"nonregular", "credential.regular", node("/vault/token", func(n *port.CredentialNode) { n.Mode = 0010600 })},
		{"foreign owner", "credential.owner", node("/vault/token", func(n *port.CredentialNode) { n.UID = 2000 })},
		{"root owner", "credential.owner", node("/vault/token", func(n *port.CredentialNode) { n.UID = 0 })},
		{"permissions", "credential.permissions", node("/vault/token", func(n *port.CredentialNode) { n.Mode = 0100601 })},
		{"unreadable contents", "", node("/vault/token", func(n *port.CredentialNode) { n.Mode = 0100000 })},
		{"links", "credential.links", node("/vault/token", func(n *port.CredentialNode) { n.Links = 2 })},
		{"unlinked", "credential.links", node("/vault/token", func(n *port.CredentialNode) { n.Links = 0 })},
		{"empty", "credential.size", node("/vault/token", func(n *port.CredentialNode) { n.Size = 0 })},
		{"one byte", "", node("/vault/token", func(n *port.CredentialNode) { n.Size = 1 })},
		{"65536 bytes", "", node("/vault/token", func(n *port.CredentialNode) { n.Size = 65536 })},
		{"too large", "credential.size", node("/vault/token", func(n *port.CredentialNode) { n.Size = 65537 })},
		{"late hard link", "credential.links", func(f *CredentialFiles) {
			n := f.Nodes["/vault/token"].CredentialNode
			n.Links = 2
			f.FinalStats["/vault/token"] = n
		}},
		{"replacement source path", "credential.unavailable", func(f *CredentialFiles) { n := f.Nodes["/vault/token"]; n.Inode = 99; f.FinalNodes[n.Path] = n }},
		{"absent state", "", func(f *CredentialFiles) { delete(f.Nodes, "/state") }},
		{"safe first state publication", "", func(f *CredentialFiles) { f.FinalNodes["/state"] = f.Nodes["/state"]; delete(f.Nodes, "/state") }},
		{"unreadable checkout", "credential.unavailable", func(f *CredentialFiles) { f.ReadErrors["/checkout"] = true }},
		{"unreadable unrelated state", "", func(f *CredentialFiles) { f.ReadErrors["/state/other"] = true }},
		{"retained history", "", func(f *CredentialFiles) { f.Retained = 100001 }},
		{"checkout entry limit", "", func(f *CredentialFiles) { f.CheckoutEntries = 100000 }},
		{"checkout entry overflow", "credential.unavailable", func(f *CredentialFiles) { f.CheckoutEntries = 100001 }},
		{"checkout depth limit", "", func(f *CredentialFiles) { f.CheckoutDepth = 64 }},
		{"checkout depth overflow", "credential.unavailable", func(f *CredentialFiles) { f.CheckoutDepth = 65 }},
		{"deadline reached", "credential.unavailable", func(f *CredentialFiles) { f.Deadline = true }},
		{"before deadline", "", func(f *CredentialFiles) { f.Elapsed = 10*time.Second - time.Nanosecond }},
		{"after deadline", "credential.unavailable", func(f *CredentialFiles) { f.Elapsed = 10*time.Second + time.Nanosecond }},
		{"fdinfo exact", "", func(f *CredentialFiles) {
			f.FDInfo = map[string]string{"/vault/token": "mnt_id: 1\n" + strings.Repeat("x", 64*1024-len("mnt_id: 1\n"))}
		}},
		{"fdinfo overflow", "credential.unavailable", func(f *CredentialFiles) {
			f.FDInfo = map[string]string{"/vault/token": "mnt_id: 1\n" + strings.Repeat("x", 64*1024-len("mnt_id: 1\n")+1)}
		}},
		{"known match with unavailable subtree", "credential.location", func(f *CredentialFiles) {
			n := f.Nodes["/vault/token"]
			n.Path = "/checkout/token"
			f.Nodes[n.Path] = n
			d := f.Nodes["/checkout"]
			d.Path, d.Inode = "/checkout/unreadable", 90
			f.Nodes[d.Path] = d
			f.ReadErrors[d.Path] = true
		}},
		{"repeated directory identities", "credential.location", func(f *CredentialFiles) {
			d := f.Nodes["/checkout"]
			d.Inode = 90
			for _, path := range []string{"/checkout/a", "/checkout/b"} {
				d.Path = path
				f.Nodes[path] = d
			}
			n := f.Nodes["/vault/token"]
			n.Path = "/checkout/b/token"
			f.Nodes[n.Path] = n
		}},
		{"disappearing checkout descendant", "credential.unavailable", func(f *CredentialFiles) {
			d := f.Nodes["/checkout"]
			d.Path, d.Inode = "/checkout/vanishing", 90
			f.Nodes[d.Path] = d
			f.OpenErrors[d.Path] = port.CredentialReadMissing
		}},
		{"missing first table", "credential.unavailable", func(f *CredentialFiles) { f.MountErrors[0] = true }},
		{"missing final table", "credential.unavailable", func(f *CredentialFiles) { f.MountErrors[1] = true }},
		{"missing descriptor", "credential.unavailable", func(f *CredentialFiles) { f.DescriptorErrors["/vault/token"] = true }},
		{"changed mapping", "credential.unavailable", func(f *CredentialFiles) { f.MountsText[1] = "1 0 0:1 /changed / rw - tmpfs tmpfs rw\n" }},
		{"unrelated mount churn", "", func(f *CredentialFiles) { f.MountsText[1] += "2 1 0:2 / /unrelated rw - tmpfs tmpfs rw\n" }},
	}
	for _, elapsed := range []time.Duration{10*time.Second - time.Nanosecond, 10 * time.Second, 10*time.Second + time.Nanosecond} {
		rule := ""
		if elapsed >= 10*time.Second {
			rule = "credential.unavailable"
		}
		cases = append(cases, testCase{fmt.Sprintf("final source stat at %s", elapsed), rule, func(f *CredentialFiles) {
			n := f.Nodes["/vault/token"].CredentialNode
			f.StatSequence = map[string][]CredentialStat{"/vault/token": {{Node: n, OK: true}, {Node: n, OK: true}, {Node: n, OK: true, Elapsed: elapsed}}}
		}})
	}
	for _, activity := range []string{"approval publication", "intent publication", "snapshot publication", "session record cleanup", "publication and disappearing temp entries"} {
		cases = append(cases, testCase{activity, "", func(f *CredentialFiles) { f.Activity = activity }})
	}
	for _, boundary := range []string{"/checkout", "/state"} {
		for _, layout := range []string{"root", "descendant", "file", "inverse directory", "inverse file", "different device", "covered", "covered root"} {
			if boundary == "/checkout" && (layout == "covered" || layout == "covered root") {
				continue
			}
			cases = append(cases, testCase{boundary + " " + layout, "credential.location", func(f *CredentialFiles) { credentialAlias(f, boundary, layout) }})
		}
	}
	for _, tc := range cases {
		f := CredentialFixture()
		tc.change(f)
		got := observe(f, grants, input)
		var want []port.CredentialIssue
		if tc.rule != "" {
			want = []port.CredentialIssue{{Rule: tc.rule, Index: 0}}
		}
		if !reflect.DeepEqual(got.Issues, want) {
			t.Errorf("%s: issues %#v, want %#v", tc.name, got.Issues, want)
		}
		if f.Calls["state enumeration"] != 0 || f.Calls["state metadata"] != 0 {
			t.Errorf("%s visited unrelated state records: %v", tc.name, f.Calls)
		}
		if f.Activity != "" && f.Completed != f.Activity {
			t.Errorf("%s did not execute scheduled activity", tc.name)
		}
	}
	f := CredentialFixture()
	got := observe(f, append(grants, policy.Credential{Kind: "github-token", Source: "/vault/token"}), input)
	if !reflect.DeepEqual(got.Issues, []port.CredentialIssue{{Rule: "credential.duplicate", Index: 1}}) {
		t.Errorf("duplicate: %#v", got)
	}
	f = CredentialFixture()
	if got := observe(f, nil, input); len(got.Issues) != 0 || len(f.Calls) != 0 {
		t.Errorf("no grants observed sources: %#v %v", got, f.Calls)
	}
	credentialAccountStateContract(t, observe, grants, input)
	credentialTopologyContract(t, observe, grants, input)
	for _, over := range []bool{false, true} {
		f := CredentialFixture()
		n := f.Nodes["/vault/token"]
		delete(f.Nodes, n.Path)
		n.Path = "/" + strings.Repeat("x", 4095)
		if over {
			n.Path += "x"
		}
		f.Nodes[n.Path] = n
		mountRoot := "/" + strings.Repeat("r", 4095)
		table := "1 0 0:1 " + mountRoot + " / rw - tmpfs tmpfs rw\n"
		f.MountsText = []string{table, table}
		got := observe(f, []policy.Credential{{Kind: "claude-token", Source: n.Path}}, input)
		if !over && len(got.Issues) != 0 {
			t.Errorf("4096-byte descriptor and 8192-byte coordinate: %#v", got)
		}
		if over && !reflect.DeepEqual(got.Issues, []port.CredentialIssue{{Rule: "credential.unavailable", Index: 0}}) {
			t.Errorf("descriptor overflow: %#v", got)
		}
	}
}

func credentialAlias(f *CredentialFiles, root, layout string) {
	base := root + "/private"
	dir := f.Nodes[root]
	dir.Path, dir.Inode = base, 20
	f.Nodes[base] = dir
	source := f.Nodes["/vault/token"]
	alias := source
	alias.Path = base + "/token"
	f.Nodes[alias.Path] = alias
	point, mountRoot, device := "/vault", base, 1
	switch layout {
	case "root":
		mountRoot = root
		n := f.Nodes[root]
		n.Path = "/vault"
		f.Nodes[n.Path] = n
	case "descendant":
		n := dir
		n.Path = "/vault"
		f.Nodes[n.Path] = n
	case "file":
		point = "/vault/token"
		mountRoot = alias.Path
	case "inverse directory":
		point = base
		mountRoot = "/vault"
	case "inverse file":
		point = alias.Path
		mountRoot = "/vault/token"
	case "different device":
		device = 2
		mountRoot = "/private"
		f.MountsText[0] += "3 1 0:2 /private " + base + " rw - tmpfs tmpfs rw\n"
		for path, n := range f.Nodes {
			if path == base || strings.HasPrefix(path, base+"/") {
				n.Device, n.MountID = 2, 3
				f.Nodes[path] = n
			}
		}
	case "covered root":
		base = root
		fallthrough
	case "covered":
		f.MountsText[0] += "3 1 0:2 / " + base + " rw - tmpfs tmpfs rw\n"
		for path, n := range f.Nodes {
			if path == base || strings.HasPrefix(path, base+"/") {
				n.Device, n.MountID, n.Inode = 2, 3, n.Inode+100
				f.Nodes[path] = n
			}
		}
	}
	f.MountsText[0] += fmt.Sprintf("2 1 0:%d %s %s rw - tmpfs tmpfs rw\n", device, mountRoot, point)
	for path, n := range f.Nodes {
		if path == point || strings.HasPrefix(path, point+"/") {
			n.Device, n.MountID = uint64(device), 2
			f.Nodes[path] = n
		}
	}
	f.MountsText[1] = f.MountsText[0]
}

func credentialTopologyContract(t Reporter, observe func(*CredentialFiles, []policy.Credential, port.CredentialInput) port.CredentialResult, grants []policy.Credential, input port.CredentialInput) {
	for _, limit := range []string{"rows", "line", "table"} {
		for _, over := range []bool{false, true} {
			f := CredentialFixture()
			var b strings.Builder
			count, length := 1, 0
			switch limit {
			case "rows":
				count = 16384
			case "line":
				length = 65535
			case "table":
				count, length = 128, 65535
			}
			if over && (limit == "rows" || limit == "table") {
				count++
			}
			if over && limit == "line" {
				length++
			}
			for i := 1; i <= count; i++ {
				point := "/"
				if i > 1 {
					point = fmt.Sprintf("/unrelated/%d", i)
				}
				line := fmt.Sprintf("%d 0 0:1 / %s rw ", i, point)
				end := "- tmpfs tmpfs rw"
				if length > 0 {
					line += "x:" + strings.Repeat("a", length-len(line)-len(end)-3) + " "
				}
				b.WriteString(line + end + "\n")
			}
			f.MountsText = []string{b.String(), b.String()}
			got := observe(f, grants, input)
			if !over && len(got.Issues) != 0 {
				t.Errorf("exact %s limit: %#v", limit, got)
			}
			if over && !reflect.DeepEqual(got.Issues, []port.CredentialIssue{{Rule: "credential.unavailable", Index: 0}}) {
				t.Errorf("%s overflow: %#v", limit, got)
			}
		}
	}
}

func credentialAccountStateContract(t Reporter, observe func(*CredentialFiles, []policy.Credential, port.CredentialInput) port.CredentialResult, grants []policy.Credential, input port.CredentialInput) {
	input.AccountState = "/fixed"
	for _, layout := range []string{"absent", "created", "unsafe", "replaced", "direct", "alias", "inverse", "covered"} {
		f := CredentialFixture()
		requested := grants
		fixed := f.Nodes["/state"]
		fixed.Path, fixed.Inode = "/fixed", 90
		if layout != "absent" {
			f.Nodes[fixed.Path] = fixed
		}
		rule := ""
		switch layout {
		case "created":
			f.FinalNodes[fixed.Path] = fixed
			delete(f.Nodes, fixed.Path)
		case "unsafe":
			fixed.Mode = 0040777
			f.Nodes[fixed.Path] = fixed
			rule = "state.unsafe"
		case "replaced":
			fixed.Inode++
			f.FinalNodes[fixed.Path] = fixed
			rule = "state.path"
		case "direct":
			n := f.Nodes["/vault/token"]
			n.Path = "/fixed/token"
			f.Nodes[n.Path] = n
			requested = []policy.Credential{{Kind: "claude-token", Source: n.Path}}
			rule = "credential.location"
		case "alias", "inverse", "covered":
			layoutName := "descendant"
			if layout == "inverse" {
				layoutName = "inverse file"
			}
			if layout == "covered" {
				layoutName = "covered root"
			}
			credentialAlias(f, "/fixed", layoutName)
			rule = "credential.location"
		}
		got := observe(f, requested, input)
		var want []port.CredentialIssue
		if rule != "" {
			index := 0
			if strings.HasPrefix(rule, "state.") {
				index = -1
			}
			want = []port.CredentialIssue{{Rule: rule, Index: index}}
		}
		if !reflect.DeepEqual(got.Issues, want) {
			t.Errorf("fixed account state %s: %#v want %#v", layout, got.Issues, want)
		}
	}
}
