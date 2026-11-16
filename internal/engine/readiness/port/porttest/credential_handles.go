package porttest

import (
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func credentialHandleContract(t Reporter, observe func(*CredentialFiles, []policy.Credential, port.CredentialInput) port.CredentialResult, input port.CredentialInput) {
	f := CredentialFixture()
	f.HandleLimit = 1
	want := port.CredentialIssue{Rule: "checkout.unavailable", Index: -1, Subject: input.Root}
	if got := observe(f, []policy.Credential{{Kind: "claude-token", Source: "/vault/token"}}, input); len(got.Issues) != 1 || got.Issues[0] != want {
		t.Errorf("exhausted handles: %#v, want %#v", got, want)
	}
	if len(f.handles) != 0 {
		t.Errorf("exhausted handles retained %d handles", len(f.handles))
	}
	for _, limit := range []int{128, 1024} {
		f := CredentialFixture()
		f.HandleLimit = limit
		parent := "/vault"
		for i := 0; i < 30; i++ {
			parent += "/d"
			n := f.Nodes["/vault"]
			n.Path, n.Inode = parent, uint64(1000+i)
			f.Nodes[parent] = n
		}
		grants := []policy.Credential{}
		for i, kind := range []string{"claude-token", "github-token"} {
			n := f.Nodes["/vault/token"]
			n.Path, n.Inode = parent+"/"+kind, uint64(2000+i)
			f.Nodes[n.Path] = n
			grants = append(grants, policy.Credential{Kind: kind, Source: n.Path})
		}
		if got := observe(f, grants, input); len(got.Issues) != 0 {
			t.Errorf("deep sources with %d handles: %#v", limit, got)
		}
		if len(f.handles) != 0 {
			t.Errorf("deep sources retained %d handles after inspection", len(f.handles))
		}
	}
}
