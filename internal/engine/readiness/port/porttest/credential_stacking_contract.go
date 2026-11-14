package porttest

import (
	"reflect"
	"strings"

	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func credentialStackingContract(t Reporter, observe func(*CredentialFiles, []policy.Credential, port.CredentialInput) port.CredentialResult, grants []policy.Credential, input port.CredentialInput) {
	input.AccountState = "/fixed"
	// nfsd is the NFS server's own control filesystem, not an NFS mount.
	for _, filesystem := range []string{"overlay", "fuse", "fuseblk", "fuse.bindfs", "fuse.sshfs", "fuseblk.ntfs", "nfs", "nfs4", "9p", "nfsd", "ext4"} {
		for _, point := range []string{"/", "/checkout", "/checkout/private", "/checkout/private/token", "/state", "/state/private", "/fixed", "/fixed/private", "/vault", "/vault/token", "/unrelated", "/checkout-sibling", "/state-sibling", "/fixed-sibling"} {
			for _, observation := range []string{"both", "first", "final"} {
				f := CredentialFixture()
				fixed := f.Nodes["/state"]
				fixed.Path, fixed.Inode = "/fixed", 90
				f.Nodes[fixed.Path] = fixed
				for i := range f.MountsText {
					kind := filesystem
					if observation == "first" && i == 1 || observation == "final" && i == 0 {
						kind = "tmpfs"
					}
					f.MountsText[i] += "2 1 0:64 / " + point + " rw - " + kind + " backing rw\n"
				}
				for path, n := range f.Nodes {
					if path == point || strings.HasPrefix(path, strings.TrimSuffix(point, "/")+"/") {
						n.MountID, n.Device = 2, 64
						f.Nodes[path] = n
					}
				}
				var want []port.CredentialIssue
				if filesystem != "ext4" && filesystem != "nfsd" && point != "/unrelated" && !strings.HasSuffix(point, "-sibling") {
					want = []port.CredentialIssue{{Rule: "credential.filesystem", Index: 0}}
				}
				got := observe(f, grants, input)
				if !reflect.DeepEqual(got.Issues, want) {
					t.Errorf("%s at %s in %s observation: %#v want %#v", filesystem, point, observation, got.Issues, want)
				}
				if f.Calls["state enumeration"] != 0 || f.Calls["state metadata"] != 0 || len(f.handles) != 0 {
					t.Errorf("stacking observation visited state records or retained handles: %v %d", f.Calls, len(f.handles))
				}
			}
		}
	}
	for _, layout := range []string{"covered checkout", "covered state", "covered account state", "absent state", "absent account state", "source ancestor", "known identity", "source symlink", "unsafe ancestor"} {
		f := CredentialFixture()
		fixed := f.Nodes["/state"]
		fixed.Path, fixed.Inode = "/fixed", 90
		f.Nodes[fixed.Path] = fixed
		point, rule := "/checkout", "credential.filesystem"
		switch layout {
		case "covered state", "absent state":
			point = "/state"
		case "covered account state", "absent account state":
			point = "/fixed"
		case "source ancestor":
			point = "/vault"
			n := f.Nodes[point]
			n.MountID, n.Device = 2, 64
			f.Nodes[point] = n
		case "known identity":
			n := f.Nodes["/vault/token"]
			n.Path = "/checkout/token"
			f.Nodes[n.Path] = n
			rule = "credential.location"
		case "source symlink":
			n := f.Nodes["/vault/token"]
			n.Mode = 0120777
			f.Nodes[n.Path] = n
			rule = "credential.symlink"
		case "unsafe ancestor":
			n := f.Nodes["/vault"]
			n.Mode = 0040777
			f.Nodes[n.Path] = n
			rule = "credential.ancestor"
		}
		if strings.HasPrefix(layout, "absent") {
			delete(f.Nodes, point)
			point += "/private"
		}
		for i := range f.MountsText {
			f.MountsText[i] += "2 1 0:64 / " + point + " rw - overlay backing rw\n"
			if layout == "source ancestor" {
				// A normal file bind covers the source itself; its ancestor still
				// sits on the overlay and must prevent an exclusion proof.
				f.MountsText[i] += "3 2 0:1 /vault/token /vault/token rw - tmpfs tmpfs rw\n"
				n := f.Nodes["/vault/token"]
				n.MountID = 3
				f.Nodes[n.Path] = n
			}
			if strings.HasPrefix(layout, "covered") {
				// No held boundary descriptor uses the hidden row's ID 2;
				// the full table must still reserve its backing.
				f.MountsText[i] += "3 1 0:1 " + point + " " + point + " rw - tmpfs tmpfs rw\n"
				if n, exists := f.Nodes[point]; exists {
					n.MountID = 3
					f.Nodes[point] = n
				}
			}
		}
		got := observe(f, grants, input)
		want := []port.CredentialIssue{{Rule: rule, Index: 0}}
		if !reflect.DeepEqual(got.Issues, want) {
			t.Errorf("stacking %s: %#v want %#v", layout, got.Issues, want)
		}
	}
}
