package disk

import (
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	rf "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"slices"
	"strconv"
	"strings"
	"testing"
)

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
		return []rp.CredentialMount{{ID: 1, Device: 1, Root: "/", Point: "/"}}, true
	}
	var grants []policy.Credential
	if kinds.Claude {
		grants = append(grants, policy.Credential{Kind: "claude-token", Source: "/vault/token"})
	}
	if kinds.GitHub {
		grants = append(grants, policy.Credential{Kind: "github-token", Source: "/vault/github"})
	}
	sources := &CheckedSources{FS: sourceMetadata{fs}, Input: rp.CredentialInput{Root: "/checkout", State: "/state", AccountState: "/state", OS: "linux", Architecture: "amd64", UID: 1000}, Grants: grants}
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
