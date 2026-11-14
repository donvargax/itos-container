package domain_test

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
)

// These cases separate conditions that the shared fake/disk contract proves
// together. Each one differs from an accepted observation in a single respect,
// so the refusal can come from no other check.

var credentialRootRow = port.CredentialMount{ID: 1, Device: 1, Root: "/", Point: "/", Filesystem: "tmpfs"}

// credentialTables supplies one typed table for the first mount observation
// and another for the final one.
func credentialTables(f *porttest.CredentialFiles, first, final []port.CredentialMount) {
	reads := 0
	f.Parser = func([]byte) ([]port.CredentialMount, bool) {
		reads++
		if reads == 1 {
			return first, true
		}
		return final, true
	}
}

func credentialOnMount(f *porttest.CredentialFiles, id int, paths ...string) {
	for _, path := range paths {
		n := f.Nodes[path]
		n.MountID = id
		f.Nodes[path] = n
	}
}

func credentialIssue(rule string) []port.CredentialIssue {
	if rule == "" {
		return nil
	}
	return []port.CredentialIssue{{Rule: rule, Index: 0}}
}

func TestCredentialMappingMustNotChangeBetweenObservations(t *testing.T) {
	for _, tc := range []struct {
		name  string
		paths []string
	}{
		{"source", []string{"/vault", "/vault/token"}},
		{"checkout", []string{"/checkout"}},
		{"state", []string{"/state"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, c, grants := credentialDomainFixture()
			credentialOnMount(f, 2, tc.paths...)
			row := port.CredentialMount{ID: 2, Device: 1, Root: "/first", Point: tc.paths[0]}
			moved := row
			moved.Root = "/final"
			credentialTables(f, []port.CredentialMount{credentialRootRow, row}, []port.CredentialMount{credentialRootRow, moved})
			if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, credentialIssue("credential.unavailable")) {
				t.Fatalf("independent in each table but moved between them: %#v", got.Issues)
			}
		})
	}
}

// The worker's non-recursive bind exposes a checkout directory that a host
// submount hides from inventory. Only its coordinate places the source there.
func TestCredentialCheckoutCoordinateNeedsTheSameDevice(t *testing.T) {
	for device, rule := range map[uint64]string{1: "credential.location", 7: ""} {
		f, c, grants := credentialDomainFixture()
		credentialOnMount(f, 2, "/vault", "/vault/token")
		rows := []port.CredentialMount{credentialRootRow, {ID: 2, Device: device, Root: "/checkout/private", Point: "/vault"}}
		credentialTables(f, rows, rows)
		if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, credentialIssue(rule)) {
			t.Fatalf("device %d: %#v, want %q", device, got.Issues, rule)
		}
	}
}

// A match in either complete observation is exposure, even when the other
// observation places the source elsewhere.
func TestCredentialLocationInEitherObservation(t *testing.T) {
	inside := port.CredentialMount{ID: 2, Device: 1, Root: "/state/private", Point: "/vault"}
	outside := port.CredentialMount{ID: 2, Device: 1, Root: "/independent", Point: "/vault"}
	for name, rows := range map[string][2]port.CredentialMount{"first": {inside, outside}, "final": {outside, inside}} {
		f, c, grants := credentialDomainFixture()
		credentialOnMount(f, 2, "/vault", "/vault/token")
		credentialTables(f, []port.CredentialMount{credentialRootRow, rows[0]}, []port.CredentialMount{credentialRootRow, rows[1]})
		if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, credentialIssue("credential.location")) {
			t.Fatalf("alias only in the %s table: %#v", name, got.Issues)
		}
	}
}

func TestCredentialFilesystemRefusalInEitherObservation(t *testing.T) {
	merged := port.CredentialMount{ID: 2, Device: 64, Root: "/", Point: "/checkout/child", Filesystem: "overlay"}
	plain := merged
	plain.Filesystem = "tmpfs"
	for name, rows := range map[string][2]port.CredentialMount{"first": {merged, plain}, "final": {plain, merged}} {
		f, c, grants := credentialDomainFixture()
		credentialTables(f, []port.CredentialMount{credentialRootRow, rows[0]}, []port.CredentialMount{credentialRootRow, rows[1]})
		if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, credentialIssue("credential.filesystem")) {
			t.Fatalf("overlay only in the %s table: %#v", name, got.Issues)
		}
	}
}

// Descriptor reads of the source, in order: the walk before the first table,
// the recheck after it, the pathname rewalk, then the recheck after the final
// table. One missing STATX_MNT_ID anywhere keeps its own diagnostic.
func TestCredentialMountIDMissingAtOneRead(t *testing.T) {
	for name, ids := range map[string][]int{
		"before the first table":      {0, 1},
		"after the first table":       {1, 0},
		"only in the first recheck":   {1, 0, 1},
		"only after the final table":  {1, 1, 1, 0},
		"only in the final rewalk":    {1, 1, 1, 1, 0},
		"only in the checkout rewalk": nil,
	} {
		f, c, grants := credentialDomainFixture()
		f.DescriptorMountIDs = map[string][]int{"/vault/token": ids}
		if ids == nil {
			f.DescriptorMountIDs = map[string][]int{"/checkout": {1, 1, 0, 1}}
		}
		if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, credentialIssue("credential.mount_id")) {
			t.Fatalf("%s: %#v", name, got.Issues)
		}
	}
}

func TestCredentialMountIDCannotMoveToAnotherRow(t *testing.T) {
	f, c, grants := credentialDomainFixture()
	second := credentialRootRow
	second.ID = 2
	rows := []port.CredentialMount{credentialRootRow, second}
	credentialTables(f, rows, rows)
	// Both rows give the source the same coordinate; only its mount ID moved.
	f.MountIDsAfterTable = map[string]int{"/vault/token": 2}
	if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, credentialIssue("credential.unavailable")) {
		t.Fatalf("mount ID changed across the table read: %#v", got.Issues)
	}
}

// A descriptor read that fails proves nothing, even when it reports the same
// path and mount ID as a read that succeeded: once before the table and once
// after it. The fdinfo seam fails exactly one read and keeps its evidence.
func TestCredentialFailedDescriptorReadIgnoresItsEvidence(t *testing.T) {
	for _, failing := range []int{1, 2} {
		for _, path := range []string{"/vault/token", "/vault", "/checkout", "/state"} {
			f, c, grants := credentialDomainFixture()
			f.FDInfo = map[string]string{path: "mnt_id:\t1\n"}
			reads := 0
			f.FDParser = func([]byte) (int, bool) {
				reads++
				return 1, reads != failing
			}
			if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, credentialIssue("credential.unavailable")) {
				t.Fatalf("%s, failed read %d: %#v", path, failing, got.Issues)
			}
		}
	}
}

// The held pin must still be the object that was walked: same device and same
// inode, each on its own, before its mapping and again at the final stat.
func TestCredentialHeldIdentityCannotChange(t *testing.T) {
	for _, change := range []string{"inode", "device"} {
		for _, path := range []string{"/vault/token", "/vault", "/checkout", "/state"} {
			f, c, grants := credentialDomainFixture()
			n := f.Nodes[path].CredentialNode
			other := n
			if change == "inode" {
				other.Inode += 500
			} else {
				other.Device += 500
			}
			f.StatSequence = map[string][]porttest.CredentialStat{path: {{Node: other, OK: true}, {Node: n, OK: true}}}
			if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, credentialIssue("credential.unavailable")) {
				t.Fatalf("%s reported another %s before mapping: %#v", path, change, got.Issues)
			}
		}
	}
	for _, final := range []string{"failed with the pinned identity", "inode", "device"} {
		f, c, grants := credentialDomainFixture()
		n := f.Nodes["/vault/token"].CredentialNode
		last := porttest.CredentialStat{Node: n, OK: final == "inode" || final == "device"}
		switch final {
		case "inode":
			last.Node.Inode += 500
		case "device":
			last.Node.Device += 500
		}
		f.StatSequence = map[string][]porttest.CredentialStat{"/vault/token": {{Node: n, OK: true}, {Node: n, OK: true}, last}}
		if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, credentialIssue("credential.unavailable")) {
			t.Fatalf("final source stat %s: %#v", final, got.Issues)
		}
	}
}

// First publication is accepted only at the coordinate reserved while the root
// was absent. Here the new root is a mountpoint of another backing directory.
func TestCredentialStateCreatedAtAnotherCoordinate(t *testing.T) {
	f, c, grants := credentialDomainFixture()
	created := f.Nodes["/state"]
	created.MountID = 2
	f.FinalNodes["/state"] = created
	delete(f.Nodes, "/state")
	rows := []port.CredentialMount{credentialRootRow, {ID: 2, Device: 1, Root: "/elsewhere", Point: "/state"}}
	credentialTables(f, rows, rows)
	if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, credentialIssue("credential.unavailable")) {
		t.Fatalf("state published away from its reserved coordinate: %#v", got.Issues)
	}
}

// Every handle an inspection opens is closed before it returns. Repeated
// inspections of one filesystem fit the handle budget a single one needs.
func TestCredentialInspectionReleasesEveryHandle(t *testing.T) {
	inspect := func(limit, runs int) bool {
		f, c, grants := credentialDomainFixture()
		f.HandleLimit = limit
		f.MountsText = slices.Repeat(f.MountsText, runs)
		for range runs {
			if len(c.Inspect(grants).Issues) != 0 {
				return false
			}
		}
		return true
	}
	limit := 1
	for !inspect(limit, 1) {
		if limit++; limit > 64 {
			t.Fatal("no handle budget up to 64 fits one inspection")
		}
	}
	if !inspect(limit, 3) {
		t.Fatalf("a repeated inspection needs more than the first one's %d handles", limit)
	}
}

func TestCredentialAncestorRule(t *testing.T) {
	for _, tc := range []struct {
		name string
		uid  uint32
		mode uint32
		rule string
	}{
		{"own private", 1000, 0040700, ""},
		{"root-owned", 0, 0040755, ""},
		{"root-owned sticky and writable", 0, 0041777, ""},
		{"root-owned writable without sticky", 0, 0040777, "credential.ancestor"},
		{"own sticky and writable", 1000, 0041777, "credential.ancestor"},
		{"own group-writable", 1000, 0040720, "credential.ancestor"},
		{"own other-writable", 1000, 0040702, "credential.ancestor"},
		{"foreign private", 2000, 0040700, "credential.ancestor"},
	} {
		f, c, grants := credentialDomainFixture()
		n := f.Nodes["/vault"]
		n.UID, n.Mode = tc.uid, tc.mode
		f.Nodes[n.Path] = n
		if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, credentialIssue(tc.rule)) {
			t.Fatalf("%s ancestor: %#v, want %q", tc.name, got.Issues, tc.rule)
		}
	}
	// A symbolic link is reported before the unsafe ancestor that holds it.
	f, c, grants := credentialDomainFixture()
	n := f.Nodes["/vault"]
	n.UID = 2000
	f.Nodes[n.Path] = n
	n = f.Nodes["/vault/token"]
	n.Mode = 0120777
	f.Nodes[n.Path] = n
	if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, credentialIssue("credential.symlink")) {
		t.Fatalf("symlink below an unsafe ancestor: %#v", got.Issues)
	}
}

// Checkout ancestors carry no credential-source owner or permission rule.
func TestCredentialCheckoutAncestorsAreNotSourceAncestors(t *testing.T) {
	for name, change := range map[string]func(*porttest.CredentialEntry){
		"group-writable": func(n *porttest.CredentialEntry) { n.Mode = 0040775 },
		"foreign-owned":  func(n *porttest.CredentialEntry) { n.UID = 2000 },
	} {
		f, c, grants := credentialDomainFixture()
		parent := f.Nodes["/checkout"]
		parent.Path, parent.Inode = "/projects", 70
		change(&parent)
		f.Nodes[parent.Path] = parent
		checkout := f.Nodes["/checkout"]
		checkout.Path, checkout.Inode = "/projects/checkout", 71
		f.Nodes[checkout.Path] = checkout
		c.Input.Root = checkout.Path
		if got := c.Inspect(grants); len(got.Issues) != 0 {
			t.Fatalf("%s checkout parent: %#v", name, got.Issues)
		}
	}
}

func TestCredentialFileBounds(t *testing.T) {
	for _, tc := range []struct {
		name   string
		rule   string
		change func(*port.CredentialNode)
	}{
		{"empty", "credential.size", func(n *port.CredentialNode) { n.Size = 0 }},
		{"one byte", "", func(n *port.CredentialNode) { n.Size = 1 }},
		{"65536 bytes", "", func(n *port.CredentialNode) { n.Size = 65536 }},
		{"unlinked", "credential.links", func(n *port.CredentialNode) { n.Links = 0 }},
		{"group-readable", "credential.permissions", func(n *port.CredentialNode) { n.Mode = 0100640 }},
		{"other-executable", "credential.permissions", func(n *port.CredentialNode) { n.Mode = 0100601 }},
		{"owner-unreadable", "", func(n *port.CredentialNode) { n.Mode = 0100000 }},
	} {
		f, c, grants := credentialDomainFixture()
		n := f.Nodes["/vault/token"]
		tc.change(&n.CredentialNode)
		f.Nodes[n.Path] = n
		if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, credentialIssue(tc.rule)) {
			t.Fatalf("%s source: %#v, want %q", tc.name, got.Issues, tc.rule)
		}
	}
}

func TestCredentialPathShape(t *testing.T) {
	for _, root := range []string{"/checkout//nested", "/checkout/./nested", "/checkout/"} {
		_, c, grants := credentialDomainFixture()
		c.Input.Root = root
		want := []port.CredentialIssue{{Rule: "checkout.root", Index: -1, Subject: root}}
		if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, want) {
			t.Fatalf("checkout %q: %#v", root, got.Issues)
		}
	}
	_, c, grants := credentialDomainFixture()
	c.Input.Architecture = "arm64"
	if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, []port.CredentialIssue{{Rule: "host.unsupported", Index: -1}}) {
		t.Fatalf("another architecture: %#v", got.Issues)
	}
	// A descriptor pathname of exactly 4096 bytes is evidence; one more is not.
	for length, rule := range map[int]string{4096: "", 4097: "credential.unavailable"} {
		f, c, _ := credentialDomainFixture()
		n := f.Nodes["/vault/token"]
		n.Path = "/" + strings.Repeat("x", length-1)
		f.Nodes[n.Path] = n
		got := c.Inspect([]policy.Credential{{Kind: "claude-token", Source: n.Path}})
		if !reflect.DeepEqual(got.Issues, credentialIssue(rule)) {
			t.Fatalf("%d-byte source path: %#v, want %q", length, got.Issues, rule)
		}
	}
}

// An inventory is complete only when every batch was read, bounded and made of
// plain entry names. These entries would open if they were tried.
func TestCredentialInventoryRefusesWhatItCannotTrust(t *testing.T) {
	batch := func(done, ok bool, names ...string) map[string][]porttest.CredentialBatch {
		return map[string][]porttest.CredentialBatch{"/checkout": {{Names: names, Done: done, OK: ok}}}
	}
	var oversize []string
	for i := 0; i <= port.MetadataBatchSize; i++ {
		oversize = append(oversize, fmt.Sprintf("entry-%d", i))
	}
	for _, tc := range []struct {
		name    string
		batches map[string][]porttest.CredentialBatch
		node    string
		entries int
	}{
		{"failed read reported as complete", batch(true, false), "", 0},
		{"batch above the limit", batch(true, true, oversize...), "", len(oversize)},
		{"name with a separator", batch(true, true, "nested/file"), "/checkout/nested/file", 0},
		{"dot name", batch(true, true, "."), "/checkout/.", 0},
		{"empty name", batch(true, true, ""), "/checkout/", 0},
	} {
		f, c, grants := credentialDomainFixture()
		f.Batches, f.CheckoutEntries = tc.batches, tc.entries
		if tc.node != "" {
			n := f.Nodes["/checkout"]
			n.Path, n.Inode = tc.node, 80
			f.Nodes[n.Path] = n
		}
		if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, credentialIssue("credential.unavailable")) {
			t.Fatalf("%s: %#v", tc.name, got.Issues)
		}
	}
}
