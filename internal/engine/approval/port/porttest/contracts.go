package porttest

import (
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"strings"
	"sync"
)

// Reporter is the small test API used by fake and concrete adapter contracts.
type Reporter interface {
	Helper()
	Errorf(string, ...any)
}

// StoreContract runs unchanged against memory and the protected disk store.
func StoreContract(t Reporter, open func() (port.Store, port.Candidate, port.Candidate)) {
	t.Helper()
	store, a, b := open()
	if record, failure := store.Read(a); failure != "" || record.Generation != "" {
		t.Errorf("absent read = %#v, %s", record, failure)
	}
	if frozen, failure := store.Select(a, ""); failure != "missing" || frozen.Settings != "" {
		t.Errorf("missing selection = %#v, %s", frozen, failure)
	}
	first, failure := store.Commit(a, "")
	if failure != "" || first.Generation != "1" || first.Canonical != a.Canonical {
		t.Errorf("first commit = %#v, %s", first, failure)
		return
	}
	if same, failure := store.Commit(a, "1"); failure != "" || same != first {
		t.Errorf("same approval rewritten: %#v, %s", same, failure)
	}
	frozen, failure := store.Select(a, "1")
	if failure != "" || frozen.Settings != a.Canonical || frozen.Project != a.Project || frozen.Fingerprint != a.Fingerprint || frozen.Generation != "1" {
		t.Errorf("frozen = %#v, %s", frozen, failure)
	}
	buffer := frozen.Bytes()
	if len(buffer) > 0 {
		buffer[0] = 'X'
	}
	if frozen.Settings != a.Canonical {
		t.Errorf("mutated frozen buffer")
	}
	if _, failure := store.Select(b, "1"); failure != "changed" {
		t.Errorf("changed selection = %s", failure)
	}
	// Two writers that reviewed the same generation cannot both replace it,
	// including when both want the identical new settings.
	var wg sync.WaitGroup
	results := make(chan port.Failure, 2)
	for range 2 {
		wg.Go(func() { _, failure := store.Commit(b, "1"); results <- failure })
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for failure := range results {
		switch failure {
		case "":
			successes++
		case "approve.conflict":
			conflicts++
		default:
			t.Errorf("concurrent failure %s", failure)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Errorf("concurrent winners/conflicts = %d/%d", successes, conflicts)
	}
	if _, failure := store.Select(a, "1"); failure != "superseded" {
		t.Errorf("old generation selected: %s", failure)
	}
	next, failure := store.Select(b, "2")
	if failure != "" || next.Settings != b.Canonical || frozen.Settings != a.Canonical {
		t.Errorf("later snapshot mutated retained value")
	}
	if _, failure := store.Commit(a, "1"); failure != "approve.conflict" {
		t.Errorf("stale publication = %s", failure)
	}
}

// Ready supplies a complete deterministic inspected request, without I/O.
func Ready() readiness.Result {
	reference := "worker.test/agent@sha256:" + strings.Repeat("a", 64)
	return readiness.Result{Ready: true,
		Checkout: &readiness.CheckoutIdentity{Root: "/checkout", Device: "1", Inode: "2", GitDevice: "1", GitInode: "3"},
		Engine:   &readiness.EngineIdentity{Endpoint: "unix:///engine.sock", ID: "engine-A", OS: "linux", Architecture: "amd64"},
		Image:    &readiness.ImageIdentity{Reference: reference, ID: "sha256:" + strings.Repeat("b", 64), OS: "linux", Architecture: "amd64"},
		Policy:   &policy.Result{Policy: &policy.Policy{Version: 1, Manifest: "worker.json"}},
		Manifest: &manifest.Result{Effective: &manifest.Settings{Image: reference, WorkspaceFolder: "/workspace", ContainerEnv: map[string]string{}}}}
}

// Readiness returns controlled observations and counts inspections.
type Readiness struct {
	Result readiness.Result
	Reads  int
	Next   *readiness.Result
}

func (r *Readiness) Inspect() readiness.Result {
	r.Reads++
	if r.Reads > 1 && r.Next != nil {
		return *r.Next
	}
	return r.Result
}

// Host supplies identities as values with no environment or process access.
type Host struct{ UID, GID, Stamp string }

func (h *Host) Identity() (string, string) { return h.UID, h.GID }
func (h *Host) Socket(string) string       { return h.Stamp }

// Codec selects precomputed fixture values and records resolution inputs.
type Codec struct {
	Values   map[string]port.Candidate
	UID, GID string
}

func (c *Codec) Candidate(_ readiness.Result, uid, gid string) port.Candidate {
	c.UID, c.GID = uid, gid
	return c.Values[uid+":"+gid]
}

// ReviewContract checks consent outcomes independently of terminal allocation.
// The real adapter receives these complete/incomplete lines on its reader.
func ReviewContract(t Reporter, open func(string) port.Reviewer) {
	t.Helper()
	for _, tc := range []struct {
		line string
		yes  bool
	}{{"y\n", true}, {" YES \n", true}, {"Y\n", true}, {"\n", false}, {"n\n", false}, {"later\n", false}, {"", false}, {"yes", false}} {
		if got := open(tc.line).Review(port.View{Candidate: port.Candidate{Canonical: "{}"}}); got != tc.yes {
			t.Errorf("review %q = %v", tc.line, got)
		}
	}
}
