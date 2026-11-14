package porttest

import (
	"reflect"
	"strings"

	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func credentialBoundaryReviewContract(t Reporter, observe func(*CredentialFiles, []policy.Credential, port.CredentialInput) port.CredentialResult, grants []policy.Credential, input port.CredentialInput) {
	input.AccountState = "/fixed"
	for _, state := range []string{"/state", "/fixed"} {
		for _, layout := range []string{"identity", "root", "descendant", "ancestor", "nested", "inverse", "sibling", "other device", "covered", "absent"} {
			f := CredentialFixture()
			n := f.Nodes["/state"]
			n.Path, n.Inode = "/fixed", 90
			f.Nodes[n.Path] = n
			point, root, device := "/checkout", state, "1"
			switch layout {
			case "identity":
				n = f.Nodes[state]
				n.Path = point
				f.Nodes[point] = n
			case "descendant":
				root += "/private"
			case "ancestor":
				root = "/"
			case "nested":
				point += "/records"
			case "inverse":
				point, root = state, "/checkout/private"
			case "sibling":
				root += "-sibling"
			case "other device":
				device = "2"
			case "covered":
				// The visible state is on another device; its hidden backing
				// subtree is still reserved and aliases the checkout.
				for i := range f.MountsText {
					f.MountsText[i] += "3 1 0:3 / " + state + " rw - tmpfs tmpfs rw\n"
				}
				n = f.Nodes[state]
				n.MountID, n.Device = 3, 3
				f.Nodes[state] = n
			case "absent":
				delete(f.Nodes, state)
			}
			if layout != "identity" {
				for i := range f.MountsText {
					f.MountsText[i] += "2 1 0:" + device + " " + root + " " + point + " rw - tmpfs tmpfs rw\n"
				}
				for path, node := range f.Nodes {
					if path == point || strings.HasPrefix(path, point+"/") {
						node.MountID = 2
						f.Nodes[path] = node
					}
				}
			}
			refuse := layout != "sibling" && layout != "other device"
			var want []port.CredentialIssue
			if refuse {
				want = []port.CredentialIssue{{Rule: "state.path", Index: -1}}
				f.ReadErrors["/checkout"] = true
			}
			got := observe(f, grants, input)
			if !reflect.DeepEqual(got.Issues, want) {
				t.Errorf("checkout aliases %s through %s: %#v want %#v", state, layout, got.Issues, want)
			}
			if refuse && f.Calls["checkout traversal"] != 0 {
				t.Errorf("enumerated checkout alias of %s through %s", state, layout)
			}
			if f.Calls["state enumeration"] != 0 || len(f.handles) != 0 {
				t.Errorf("boundary inspection enumerated state or retained handles: %v", f.Calls)
			}
		}
	}
	for _, fault := range []string{"mount ID", "descriptor"} {
		for _, filesystem := range []string{"overlay", "fuse.portal", "fuseblk"} {
			f := CredentialFixture()
			rule := "credential.unavailable"
			if fault == "mount ID" {
				rule = "credential.mount_id"
				f.MountIDUnavailable = map[string]bool{}
				for path := range f.Nodes {
					f.MountIDUnavailable[path] = true
				}
			} else {
				f.DescriptorErrors["/checkout"] = true
			}
			for i := range f.MountsText {
				f.MountsText[i] += "2 1 0:64 / /unrelated/merged rw - " + filesystem + " backing rw\n"
			}
			got := observe(f, grants, input)
			want := []port.CredentialIssue{{Rule: rule, Index: 0}}
			if !reflect.DeepEqual(got.Issues, want) {
				t.Errorf("missing %s beside unrelated %s: %#v want %#v", fault, filesystem, got.Issues, want)
			}
		}
	}
}
