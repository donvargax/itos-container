package porttest

import (
	"reflect"

	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// The kernel prints "/" as the root of every nfs and nfs4 mount row, whatever
// export subdirectory the mount is rooted at. One export is mounted at "/" and
// a protected tree's private subdirectory a second time over the source's
// parent. Both rows share a device and a root, so the source projects beside
// the protected tree. No pinned identity matches either: state is never
// enumerated, and a submount hides the checkout's backing entry from inventory.
// The same rows on ext4, whose root field is a real path, name an independent file.
func credentialExportContract(t Reporter, observe func(*CredentialFiles, []policy.Credential, port.CredentialInput) port.CredentialResult, grants []policy.Credential, input port.CredentialInput) {
	input.AccountState = "/fixed"
	for _, filesystem := range []string{"nfs", "nfs4", "ext4"} {
		for _, tree := range []string{"/state", "/fixed", "/checkout"} {
			f := CredentialFixture()
			fixed := f.Nodes["/state"]
			fixed.Path, fixed.Inode = "/fixed", 90
			f.Nodes[fixed.Path] = fixed
			for path, n := range f.Nodes {
				n.Device, n.MountID = 50, 2
				f.Nodes[path] = n
			}
			private := f.Nodes["/vault"]
			private.Inode, private.MountID = 91, 3
			f.Nodes[private.Path] = private
			token := f.Nodes["/vault/token"]
			token.MountID = 3
			f.Nodes[token.Path] = token
			rows := "2 1 0:50 / / rw - " + filesystem + " server:/export rw\n" +
				"3 2 0:50 / /vault rw - " + filesystem + " server:/export" + tree + "/private rw\n"
			private.Path, private.MountID = tree+"/private", 2
			if tree == "/checkout" {
				// Inventory meets only the covering directory of another filesystem.
				private.Device, private.Inode, private.MountID = 2, 92, 4
				rows += "4 2 0:2 / /checkout/private rw - tmpfs tmpfs rw\n"
			} else {
				token.Path, token.MountID = private.Path+"/token", 2
				f.Nodes[token.Path] = token
			}
			f.Nodes[private.Path] = private
			for i := range f.MountsText {
				f.MountsText[i] += rows
			}
			var want []port.CredentialIssue
			if filesystem != "ext4" {
				want = []port.CredentialIssue{{Rule: "credential.filesystem", Index: 0}}
			}
			got := observe(f, grants, input)
			if !reflect.DeepEqual(got.Issues, want) {
				t.Errorf("%s export subdirectory of %s mounted over the source: %#v want %#v", filesystem, tree, got.Issues, want)
			}
			if f.Calls["state enumeration"] != 0 || f.Calls["state metadata"] != 0 || len(f.handles) != 0 {
				t.Errorf("%s export subdirectory of %s visited state records or retained handles: %v %d", filesystem, tree, f.Calls, len(f.handles))
			}
		}
	}
}
