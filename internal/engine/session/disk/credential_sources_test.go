package disk

import (
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	rf "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	"strconv"
	"testing"
)

type sourceMetadata struct{ *rf.CredentialFiles }

func (s sourceMetadata) ReadPath(h rp.CredentialHandle) (string, bool) {
	return "/proc/self/fd/" + strconv.FormatUint(uint64(h), 10), true
}
func TestCredentialCheckedSourcesRetainValidatedInode(t *testing.T) {
	fs := rf.CredentialFixture()
	fs.MountsText = append(fs.MountsText, fs.MountsText...)
	fs.MountsText = append(fs.MountsText, fs.MountsText...)
	fs.Parser = func([]byte) ([]rp.CredentialMount, bool) {
		return []rp.CredentialMount{{ID: 1, Device: 1, Root: "/", Point: "/"}}, true
	}
	sources := &CheckedSources{FS: sourceMetadata{fs}, Input: rp.CredentialInput{Root: "/checkout", State: "/state", AccountState: "/state", OS: "linux", Architecture: "amd64", UID: 1000}, Grants: []policy.Credential{{Kind: "claude-token", Source: "/vault/token"}}}
	if fail := sources.Revalidate(t.Context()); fail != "" {
		t.Fatal("checked source acquisition refused", fail)
	}
	first := sources.Checked()
	if len(first) != 1 || first[0].Node.Inode == 0 {
		t.Fatal("checked descriptor missing")
	}
	if fail := sources.Revalidate(t.Context()); fail != "" {
		t.Fatal("unchanged revalidation refused", fail)
	}
	if again := sources.Checked(); len(again) != 1 || again[0] != first[0] {
		t.Fatal("revalidation replaced the original descriptor")
	}
	n := fs.Nodes["/vault/token"]
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
