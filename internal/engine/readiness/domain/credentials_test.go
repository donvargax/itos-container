package domain_test

import (
	"reflect"
	"testing"
	"time"

	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/domain"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
)

func credentialDomainFixture() (*porttest.CredentialFiles, domain.Credentials, []policy.Credential) {
	f := porttest.CredentialFixture()
	// The domain consumes typed mount rows. Parser correctness is the disk
	// adapter's shared contract, never duplicated in this in-memory provider.
	f.Parser = func([]byte) ([]port.CredentialMount, bool) {
		return []port.CredentialMount{{ID: 1, Device: 1, Root: "/", Point: "/"}}, true
	}
	c := domain.Credentials{FS: f, Input: port.CredentialInput{Root: "/checkout", State: "/state", AccountState: "/state", OS: "linux", Architecture: "amd64", UID: 1000}}
	return f, c, []policy.Credential{{Kind: "claude-token", Source: "/vault/token"}}
}

func TestCredentialSourceOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, rule string
		change     func(*porttest.CredentialFiles)
	}{
		{"private", "", func(*porttest.CredentialFiles) {}},
		{"missing", "credential.missing", func(f *porttest.CredentialFiles) { delete(f.Nodes, "/vault/token") }},
		{"unavailable", "credential.unavailable", func(f *porttest.CredentialFiles) { f.OpenErrors["/vault"] = port.CredentialReadUnavailable }},
		{"symlink", "credential.symlink", func(f *porttest.CredentialFiles) { n := f.Nodes["/vault/token"]; n.Mode = 0120777; f.Nodes[n.Path] = n }},
		{"nondirectory ancestor", "credential.unavailable", func(f *porttest.CredentialFiles) { n := f.Nodes["/vault"]; n.Mode = 0100600; f.Nodes[n.Path] = n }},
		{"unsafe ancestor", "credential.ancestor", func(f *porttest.CredentialFiles) { n := f.Nodes["/vault"]; n.Mode = 0040777; f.Nodes[n.Path] = n }},
		{"regular", "credential.regular", func(f *porttest.CredentialFiles) { n := f.Nodes["/vault/token"]; n.Mode = 0010600; f.Nodes[n.Path] = n }},
		{"owner", "credential.owner", func(f *porttest.CredentialFiles) { n := f.Nodes["/vault/token"]; n.UID = 0; f.Nodes[n.Path] = n }},
		{"permission", "credential.permissions", func(f *porttest.CredentialFiles) { n := f.Nodes["/vault/token"]; n.Mode |= 1; f.Nodes[n.Path] = n }},
		{"link", "credential.links", func(f *porttest.CredentialFiles) { n := f.Nodes["/vault/token"]; n.Links = 2; f.Nodes[n.Path] = n }},
		{"size", "credential.size", func(f *porttest.CredentialFiles) { n := f.Nodes["/vault/token"]; n.Size = 65537; f.Nodes[n.Path] = n }},
		{"late link", "credential.links", func(f *porttest.CredentialFiles) {
			n := f.Nodes["/vault/token"].CredentialNode
			n.Links = 2
			f.FinalStats["/vault/token"] = n
		}},
		{"path replaced", "credential.unavailable", func(f *porttest.CredentialFiles) {
			n := f.Nodes["/vault/token"]
			n.Inode = 77
			f.FinalNodes[n.Path] = n
		}},
		{"stat unavailable", "credential.unavailable", func(f *porttest.CredentialFiles) { f.StatErrors["/vault/token"] = true }},
		{"descriptor unavailable", "credential.unavailable", func(f *porttest.CredentialFiles) { f.DescriptorErrors["/vault/token"] = true }},
		{"table unavailable", "credential.unavailable", func(f *porttest.CredentialFiles) { f.MountErrors[0] = true }},
		{"deadline", "credential.unavailable", func(f *porttest.CredentialFiles) { f.Deadline = true }},
		{"checkout unreadable", "credential.unavailable", func(f *porttest.CredentialFiles) { f.ReadErrors["/checkout"] = true }},
		{"checkout alias", "credential.location", func(f *porttest.CredentialFiles) {
			n := f.Nodes["/vault/token"]
			n.Path = "/checkout/token"
			f.Nodes[n.Path] = n
		}},
		{"state absent", "", func(f *porttest.CredentialFiles) { delete(f.Nodes, "/state") }},
		{"state created", "", func(f *porttest.CredentialFiles) {
			f.FinalNodes["/state"] = f.Nodes["/state"]
			delete(f.Nodes, "/state")
		}},
		{"depth exact", "", func(f *porttest.CredentialFiles) { f.CheckoutDepth = 64 }},
		{"depth exceeded", "credential.unavailable", func(f *porttest.CredentialFiles) { f.CheckoutDepth = 65 }},
		{"entry exact", "", func(f *porttest.CredentialFiles) { f.CheckoutEntries = 100000 }},
		{"entry exceeded", "credential.unavailable", func(f *porttest.CredentialFiles) { f.CheckoutEntries = 100001 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, c, grants := credentialDomainFixture()
			tc.change(f)
			got := c.Inspect(grants)
			var want []port.CredentialIssue
			if tc.rule != "" {
				want = []port.CredentialIssue{{Rule: tc.rule, Index: 0}}
			}
			if !reflect.DeepEqual(got.Issues, want) {
				t.Fatalf("%#v want %#v", got, want)
			}
		})
	}
}

func TestCredentialStateProjectionAndOrdering(t *testing.T) {
	for _, inverse := range []bool{false, true} {
		f, c, grants := credentialDomainFixture()
		rows := []port.CredentialMount{{ID: 1, Device: 1, Root: "/", Point: "/"}, {ID: 2, Device: 1, Root: "/state/private", Point: "/vault"}}
		if inverse {
			rows[1].Root, rows[1].Point = "/vault", "/state/private"
		} else {
			for _, path := range []string{"/vault", "/vault/token"} {
				n := f.Nodes[path]
				n.MountID = 2
				f.Nodes[path] = n
			}
		}
		f.Parser = func([]byte) ([]port.CredentialMount, bool) { return rows, true }
		if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, []port.CredentialIssue{{Rule: "credential.location", Index: 0}}) {
			t.Fatal(got)
		}
	}
	_, c, grants := credentialDomainFixture()
	grants = append(grants, policy.Credential{Kind: "github-token", Source: "/vault/token"})
	if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, []port.CredentialIssue{{Rule: "credential.duplicate", Index: 1}}) {
		t.Fatal(got)
	}
	f, c, grants := credentialDomainFixture()
	delete(f.Nodes, "/vault/token")
	grants = append(grants, policy.Credential{Kind: "github-token", Source: "/state"})
	if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, []port.CredentialIssue{{Rule: "credential.missing", Index: 0}, {Rule: "credential.location", Index: 1}}) {
		t.Fatal(got)
	}
	_, c, _ = credentialDomainFixture()
	c.FS = nil
	if got := c.Inspect(nil); len(got.Issues) != 0 {
		t.Fatal(got)
	}
}

func TestCredentialBoundaryRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, rule string
		change     func(*porttest.CredentialFiles, *port.CredentialInput)
	}{
		{"platform", "host.unsupported", func(_ *porttest.CredentialFiles, i *port.CredentialInput) { i.OS = "windows" }},
		{"relative checkout", "checkout.root", func(_ *porttest.CredentialFiles, i *port.CredentialInput) { i.Root = "relative" }},
		{"unavailable account", "state.unavailable", func(_ *porttest.CredentialFiles, i *port.CredentialInput) { i.AccountState = "" }},
		{"relative state", "state.location", func(_ *porttest.CredentialFiles, i *port.CredentialInput) { i.State = "relative" }},
		{"state in checkout", "state.path", func(_ *porttest.CredentialFiles, i *port.CredentialInput) { i.State = "/checkout/state" }},
		{"checkout absent", "checkout.unavailable", func(f *porttest.CredentialFiles, _ *port.CredentialInput) { delete(f.Nodes, "/checkout") }},
		{"checkout file", "checkout.unavailable", func(f *porttest.CredentialFiles, _ *port.CredentialInput) {
			n := f.Nodes["/checkout"]
			n.Mode = 0100600
			f.Nodes[n.Path] = n
		}},
		{"dot component", "checkout.root", func(_ *porttest.CredentialFiles, i *port.CredentialInput) { i.Root = "/checkout/../root" }},
		{"state file", "state.unsafe", func(f *porttest.CredentialFiles, _ *port.CredentialInput) {
			n := f.Nodes["/state"]
			n.Mode = 0100600
			f.Nodes[n.Path] = n
		}},
		{"state ancestor", "state.unsafe", func(f *porttest.CredentialFiles, i *port.CredentialInput) {
			n := f.Nodes["/vault"]
			n.Mode = 0040777
			f.Nodes[n.Path] = n
			i.State = "/vault/missing"
		}},
		{"state created unsafe", "state.unsafe", func(f *porttest.CredentialFiles, _ *port.CredentialInput) {
			n := f.Nodes["/state"]
			n.Mode = 0040777
			f.FinalNodes[n.Path] = n
			delete(f.Nodes, n.Path)
		}},
		{"state mode", "state.unsafe", func(f *porttest.CredentialFiles, _ *port.CredentialInput) {
			n := f.Nodes["/state"]
			n.Mode = 0040755
			f.Nodes[n.Path] = n
		}},
		{"state link", "state.path", func(f *porttest.CredentialFiles, _ *port.CredentialInput) {
			n := f.Nodes["/state"]
			n.Mode = 0120700
			f.Nodes[n.Path] = n
		}},
		{"state unavailable", "state.unavailable", func(f *porttest.CredentialFiles, _ *port.CredentialInput) {
			f.OpenErrors["/state"] = port.CredentialReadUnavailable
		}},
		{"state replaced", "state.path", func(f *porttest.CredentialFiles, _ *port.CredentialInput) {
			n := f.Nodes["/state"]
			n.Inode = 77
			f.FinalNodes[n.Path] = n
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, c, grants := credentialDomainFixture()
			tc.change(f, &c.Input)
			want := port.CredentialIssue{Rule: tc.rule, Index: -1}
			if tc.rule == "checkout.root" || tc.rule == "checkout.unavailable" {
				want.Subject = c.Input.Root
			}
			if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, []port.CredentialIssue{want}) {
				t.Fatal(got)
			}
		})
	}
}

func TestCredentialIncompleteEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*porttest.CredentialFiles)
	}{
		{"state descriptor", func(f *porttest.CredentialFiles) { f.DescriptorErrors["/state"] = true }},
		{"checkout descriptor", func(f *porttest.CredentialFiles) { f.DescriptorErrors["/checkout"] = true }},
		{"missing mount ID", func(f *porttest.CredentialFiles) { n := f.Nodes["/vault/token"]; n.MountID = 42; f.Nodes[n.Path] = n }},
		{"descriptor names absent path", func(f *porttest.CredentialFiles) { f.DescriptorPaths = map[string]string{"/vault/token": "/absent"} }},
		{"source disappears on final stat", func(f *porttest.CredentialFiles) {
			n := f.Nodes["/vault/token"].CredentialNode
			f.StatSequence = map[string][]porttest.CredentialStat{"/vault/token": {{Node: n, OK: true}, {Node: n, OK: true}, {OK: false}}}
		}},
		{"checkout child disappears", func(f *porttest.CredentialFiles) {
			f.Batches = map[string][]porttest.CredentialBatch{"/checkout": {{Names: []string{"missing"}, Done: true, OK: true}}}
		}},
		{"invalid directory entry", func(f *porttest.CredentialFiles) {
			f.Batches = map[string][]porttest.CredentialBatch{"/checkout": {{Names: []string{".."}, Done: true, OK: true}}}
		}},
		{"incomplete empty directory batch", func(f *porttest.CredentialFiles) {
			f.Batches = map[string][]porttest.CredentialBatch{"/checkout": {{OK: true}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, c, grants := credentialDomainFixture()
			tc.change(f)
			if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, []port.CredentialIssue{{Rule: "credential.unavailable", Index: 0}}) {
				t.Fatal(got)
			}
		})
	}
}

func TestCredentialStateMountIdentityCannotChangeAtSameCoordinate(t *testing.T) {
	f, c, grants := credentialDomainFixture()
	n := f.Nodes["/state"]
	n.MountID = 2
	f.FinalNodes[n.Path] = n
	reads := 0
	f.Parser = func([]byte) ([]port.CredentialMount, bool) {
		reads++
		rows := []port.CredentialMount{{ID: 1, Device: 1, Root: "/", Point: "/"}}
		if reads == 2 {
			rows = append(rows, port.CredentialMount{ID: 2, Device: 1, Root: "/state", Point: "/state"})
		}
		return rows, true
	}
	if got := c.Inspect(grants); !reflect.DeepEqual(got.Issues, []port.CredentialIssue{{Rule: "credential.unavailable", Index: 0}}) {
		t.Fatal(got)
	}
}

func TestCredentialDeadlineCannotProduceSuccessAfterExpiry(t *testing.T) {
	expired, complete := false, false
	for beforeExpiry := 1; beforeExpiry <= 160; beforeExpiry++ {
		f, c, grants := credentialDomainFixture()
		f.ClockTicks = make([]time.Duration, beforeExpiry+1)
		f.ClockTicks[beforeExpiry] = 10 * time.Second
		got := c.Inspect(grants)
		if f.ClockReads > beforeExpiry {
			expired = true
			if len(got.Issues) == 0 {
				t.Fatalf("success after deadline at observation %d", beforeExpiry)
			}
		} else if len(got.Issues) == 0 {
			complete = true
		}
	}
	if !expired || !complete {
		t.Fatal("deadline fixtures did not bracket a complete observation")
	}
}
