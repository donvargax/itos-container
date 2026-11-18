package disk

import (
	"context"
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	rf "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
)

type credentialSourceHandles struct {
	rp.CredentialFS
	live map[rp.CredentialHandle]bool
}

func (s *credentialSourceHandles) Open(parent rp.CredentialHandle, name string) (rp.CredentialHandle, rp.CredentialNode, rp.CredentialRead) {
	h, node, status := s.CredentialFS.Open(parent, name)
	if status == rp.CredentialReadOK {
		s.live[h] = true
	}
	return h, node, status
}

func (s *credentialSourceHandles) Close(h rp.CredentialHandle) {
	s.CredentialFS.Close(h)
	delete(s.live, h)
}

type credentialSourceReadPaths struct {
	*credentialSourceHandles
	accessible bool
}

func (s credentialSourceReadPaths) ReadPath(h rp.CredentialHandle) (string, bool) {
	return "/proc/self/fd/" + strconv.FormatUint(uint64(h), 10), s.accessible
}

func checkedSourceFixture() (*CheckedSources, *rf.CredentialFiles, *credentialSourceHandles) {
	fs := rf.CredentialFixture()
	github := fs.Nodes["/vault/token"]
	github.Path, github.Inode = "/vault/github", 6
	fs.Nodes[github.Path] = github
	fs.MountsText = append(fs.MountsText, fs.MountsText...)
	fs.MountsText = append(fs.MountsText, fs.MountsText...)
	fs.Parser = func([]byte) ([]rp.CredentialMount, bool) {
		return []rp.CredentialMount{{ID: 1, Device: 1, Root: "/", Point: "/", Filesystem: "ext4"}}, true
	}
	handles := &credentialSourceHandles{CredentialFS: fs, live: map[rp.CredentialHandle]bool{}}
	sources := &CheckedSources{
		FS:     credentialSourceReadPaths{handles, true},
		Input:  rp.CredentialInput{Root: "/checkout", State: "/state", AccountState: "/state", OS: "linux", Architecture: "amd64", UID: 1000, PolicyDevice: "1", PolicyInode: "2"},
		Grants: []policy.Credential{{Kind: "claude-token", Source: "/vault/token"}},
	}
	return sources, fs, handles
}

func TestCredentialCheckedSourcesRefusals(t *testing.T) {
	for _, fault := range []string{"cancelled", "missing filesystem", "no grants", "duplicate kind", "unknown kind", "inspection", "read paths unavailable", "descriptor inaccessible"} {
		t.Run(fault, func(t *testing.T) {
			sources, fs, handles := checkedSourceFixture()
			defer sources.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			want := port.CredentialRefused
			switch fault {
			case "cancelled":
				cancel()
			case "missing filesystem":
				sources.FS = nil
			case "no grants":
				sources.Grants = nil
			case "duplicate kind":
				sources.Grants = append(sources.Grants, sources.Grants[0])
			case "unknown kind":
				sources.Grants[0].Kind = "unknown"
			case "inspection":
				delete(fs.Nodes, "/vault/token")
			case "read paths unavailable":
				sources.FS = handles
				want = port.CredentialUnavailable
			case "descriptor inaccessible":
				sources.FS = credentialSourceReadPaths{handles, false}
			}
			if failure := sources.Revalidate(ctx); failure != want {
				t.Fatal("unsafe source capability did not refuse", failure)
			}
			if len(sources.Checked()) != 0 || len(handles.live) != 0 {
				t.Fatal("failed source inspection retained descriptor access")
			}
		})
	}
}

func TestCredentialCheckedSourcesRejectChangedGrantSet(t *testing.T) {
	sources, _, handles := checkedSourceFixture()
	defer sources.Close()
	if failure := sources.Revalidate(t.Context()); failure != "" {
		t.Fatal("initial checked source acquisition failed", failure)
	}
	original := sources.Checked()
	originalHandles := len(handles.live)
	if len(original) != 1 || originalHandles != 1 {
		t.Fatal("initial acquisition did not retain exactly the selected source")
	}
	sources.Grants = append(sources.Grants, policy.Credential{Kind: "github-token", Source: "/vault/github"})
	if failure := sources.Revalidate(t.Context()); failure != port.CredentialRefused {
		t.Fatal("revalidation replaced the original checked grant set", failure)
	}
	if !slices.Equal(sources.Checked(), original) || len(handles.live) != originalHandles {
		t.Fatal("refused revalidation leaked new descriptors or lost original access")
	}
	sources.Close()
	if len(handles.live) != 0 || len(sources.Checked()) != 0 {
		t.Fatal("closing the refused capability retained descriptor access")
	}
}

type retainedSourceMetadata struct {
	credentialSourceReadPaths
	handle   rp.CredentialHandle
	stat     rf.CredentialStat
	observed bool
}

func (s *retainedSourceMetadata) Stat(h rp.CredentialHandle) (rp.CredentialNode, bool) {
	if h == s.handle {
		s.observed = true
		return s.stat.Node, s.stat.OK
	}
	return s.CredentialFS.Stat(h)
}

func TestCredentialCheckedSourcesRecheckRetainedMetadata(t *testing.T) {
	for _, fault := range []string{"healthy", "held metadata unavailable", "held metadata unavailable with value", "held metadata changed", "new source identity", "kind changed"} {
		t.Run(fault, func(t *testing.T) {
			sources, fs, handles := checkedSourceFixture()
			defer sources.Close()
			metadata := &retainedSourceMetadata{credentialSourceReadPaths: credentialSourceReadPaths{handles, true}}
			sources.FS = metadata
			if failure := sources.Revalidate(t.Context()); failure != "" {
				t.Fatal("initial source observation failed", failure)
			}
			original, originalHandles := sources.Checked(), maps.Clone(handles.live)
			if len(original) != 1 || len(originalHandles) != 1 {
				t.Fatal("initial observation did not retain exactly one source")
			}
			for h := range originalHandles {
				metadata.handle = h
			}
			// New descriptors complete the usual observation first. The retained
			// descriptor's final stat can independently fail or observe a change
			// after that observation; neither result may be replaced by it.
			metadata.stat = rf.CredentialStat{Node: original[0].Node, OK: true}
			switch fault {
			case "held metadata unavailable":
				metadata.stat = rf.CredentialStat{}
			case "held metadata unavailable with value":
				metadata.stat.OK = false
			case "held metadata changed":
				metadata.stat.Node.Size++
			case "new source identity":
				node := fs.Nodes[sources.Grants[0].Source]
				node.Inode++
				fs.Nodes[node.Path] = node
			case "kind changed":
				sources.Grants[0].Kind = "github-token"
			}
			want := port.CredentialRefused
			if fault == "healthy" {
				want = ""
			}
			if failure := sources.Revalidate(t.Context()); failure != want {
				t.Fatal("revalidation did not require unchanged, available retained metadata and grants", failure)
			}
			if !metadata.observed {
				t.Fatal("fixture did not reach the retained descriptor recheck")
			}
			if !slices.Equal(sources.Checked(), original) || !maps.Equal(handles.live, originalHandles) {
				t.Fatal("revalidation leaked new handles or replaced the original capability")
			}
			sources.Close()
			if len(sources.Checked()) != 0 || len(handles.live) != 0 {
				t.Fatal("closing the capability retained descriptor access")
			}
		})
	}
}

type sourceMetadata struct{ *rf.CredentialFiles }

func (s sourceMetadata) ReadPath(h rp.CredentialHandle) (string, bool) {
	return "/proc/self/fd/" + strconv.FormatUint(uint64(h), 10), true
}
func TestCredentialCheckedSourcesRetainValidatedInode(t *testing.T) {
	for _, kinds := range []port.CredentialKinds{{Claude: true}, {GitHub: true}, {Claude: true, GitHub: true}} {
		t.Run(strings.Join(kinds.List(), "+"), func(t *testing.T) {
			testCredentialCheckedSources(t, kinds)
		})
	}
}

func testCredentialCheckedSources(t *testing.T, kinds port.CredentialKinds) {
	t.Helper()
	fs := rf.CredentialFixture()
	github := fs.Nodes["/vault/token"]
	github.Path, github.Inode = "/vault/github", 6
	fs.Nodes[github.Path] = github
	fs.MountsText = append(fs.MountsText, fs.MountsText...)
	fs.MountsText = append(fs.MountsText, fs.MountsText...)
	fs.Parser = func([]byte) ([]rp.CredentialMount, bool) {
		return []rp.CredentialMount{{ID: 1, Device: 1, Root: "/", Point: "/", Filesystem: "ext4"}}, true
	}
	var grants []policy.Credential
	if kinds.Claude {
		grants = append(grants, policy.Credential{Kind: "claude-token", Source: "/vault/token"})
	}
	if kinds.GitHub {
		grants = append(grants, policy.Credential{Kind: "github-token", Source: "/vault/github"})
	}
	sources := &CheckedSources{FS: sourceMetadata{fs}, Input: rp.CredentialInput{Root: "/checkout", State: "/state", AccountState: "/state", OS: "linux", Architecture: "amd64", UID: 1000, PolicyDevice: "1", PolicyInode: "2"}, Grants: grants}
	defer sources.Close()
	if sources.Kinds() != kinds {
		t.Fatal("checked sources changed the requested kinds")
	}
	if fail := sources.Revalidate(t.Context()); fail != "" {
		t.Fatal("checked source acquisition refused", fail)
	}
	first := sources.Checked()
	if len(first) != len(grants) {
		t.Fatal("checked source set incomplete")
	}
	for index, grant := range grants {
		if first[index].Kind != grant.Kind || first[index].Node != fs.Nodes[grant.Source].CredentialNode || !strings.HasPrefix(first[index].ReadPath, "/proc/self/fd/") {
			t.Fatal("checked descriptor lost the selected kind or inode")
		}
	}
	if fail := sources.Revalidate(t.Context()); fail != "" {
		t.Fatal("unchanged revalidation refused", fail)
	}
	if again := sources.Checked(); !slices.Equal(again, first) {
		t.Fatal("revalidation replaced the original descriptor")
	}
	n := fs.Nodes[grants[0].Source]
	n.Inode++
	fs.Nodes[n.Path] = n
	if fail := sources.Revalidate(t.Context()); fail == "" {
		t.Fatal("replacement source admitted")
	}
	sources.Close()
	if len(sources.Checked()) != 0 {
		t.Fatal("closed capability retained access")
	}
}
