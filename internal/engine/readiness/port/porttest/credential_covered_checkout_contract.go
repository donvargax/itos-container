package porttest

import (
	"fmt"
	"reflect"

	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func credentialCoveredCheckoutContract(t Reporter, observe func(*CredentialFiles, []policy.Credential, port.CredentialInput) port.CredentialResult, grants []policy.Credential, input port.CredentialInput) {
	for _, kind := range []string{"directory", "file"} {
		for _, layout := range []string{"both", "first", "final", "ancestor only", "subvolume devices", "checkout mount", "same prefix", "other device"} {
			f := CredentialFixture()
			covered, point, coverRoot := "/checkout/private", "/vault", "/"
			n := f.Nodes["/vault"]
			if kind == "file" {
				n.Path, n.Inode = covered, 89
				f.Nodes[covered] = n
				covered, point, coverRoot = covered+"/token", "/vault/token", "/replacement-token"
				n = f.Nodes[point]
			} else {
				alias := n
				alias.MountID = 2
				f.Nodes[alias.Path] = alias
			}
			// Only the covering entries can be enumerated in the host view.
			n.Path, n.Device, n.Inode, n.MountID = covered, 2, 90, 3
			f.Nodes[covered] = n
			n = f.Nodes["/vault/token"]
			n.MountID = 2
			if layout == "ancestor only" {
				n.MountID = 4
			}
			f.Nodes[n.Path] = n
			for i := range f.MountsText {
				root, device, coverParent := covered, 1, 1
				switch layout {
				case "first", "final":
					if layout == "first" && i == 1 || layout == "final" && i == 0 {
						root = "/independent"
					}
				case "checkout mount":
					root = "/project" + covered
					coverParent = 5
					f.MountsText[i] += "5 1 0:1 /project/checkout /checkout rw - tmpfs tmpfs rw\n"
					n := f.Nodes["/checkout"]
					n.MountID = 5
					f.Nodes[n.Path] = n
					if kind == "file" {
						n = f.Nodes["/checkout/private"]
						n.MountID = 5
						f.Nodes[n.Path] = n
					}
				case "same prefix":
					root = "/checkout-sibling/private"
				case "other device":
					device = 4
				}
				f.MountsText[i] += fmt.Sprintf("2 1 0:%d %s %s rw - tmpfs tmpfs rw\n3 %d 0:2 %s %s rw - tmpfs tmpfs rw\n", device, root, point, coverParent, coverRoot, covered)
				if layout == "ancestor only" {
					f.MountsText[i] += "4 2 0:1 /independent/token /vault/token rw - tmpfs tmpfs rw\n"
				}
			}
			for path, n := range f.Nodes {
				if layout == "other device" && n.MountID == 2 {
					n.Device = 4
				}
				if layout == "subvolume devices" {
					n.Device += 100
				}
				f.Nodes[path] = n
			}
			want := []port.CredentialIssue{{Rule: "credential.location", Index: 0}}
			if layout == "same prefix" || layout == "other device" || layout == "ancestor only" && kind == "file" {
				want = nil
			}
			got := observe(f, grants, input)
			if !reflect.DeepEqual(got.Issues, want) {
				t.Errorf("covered checkout %s, %s: %#v want %#v", kind, layout, got, want)
			}
			if f.Calls["checkout traversal"] == 0 || f.Calls["state enumeration"] != 0 || f.Calls["state metadata"] != 0 || len(f.handles) != 0 {
				t.Errorf("covered checkout %s, %s: unexpected visits or retained handles: %v %d", kind, layout, f.Calls, len(f.handles))
			}
		}
	}
}
