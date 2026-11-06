package features

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestAnchorFixtureGuard(t *testing.T) {
	t.Setenv("ITOS_CONTAINER_ENGINE_TESTS", "0")
	w := &world{}
	if err := w.anchorFixture(); err == nil || w.doctor != nil || w.anchor != nil {
		t.Fatal("anchor fixture initialized outside the CI engine tier")
	}
}

func TestAnchorProbeCompilesWithoutEngine(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	w := &world{root: root}
	if err := w.buildAnchorProbe(filepath.Join(t.TempDir(), "slice-17-probe")); err != nil {
		t.Fatal(err)
	}
	// Compile only: the reporting workload is never executed on the host.
}

func TestAnchorStartBarrierForwardsTheOriginalRequest(t *testing.T) {
	s := &anchorWorld{ready: make(chan struct{}), release: make(chan struct{})}
	a := &offlineAPI{w: &world{anchor: s}}
	r := httptest.NewRequest("POST", "/containers/owned/start", strings.NewReader("original body"))
	entry := offlineRequest{Method: "POST", Path: r.URL.Path, Observation: map[string]any{"Id": "owned"}}
	done := make(chan bool, 1)
	go func() { done <- a.anchorStart(r, entry) }()
	select {
	case <-s.ready:
	case <-time.After(time.Second):
		t.Fatal("start barrier was not reached")
	}
	select {
	case <-done:
		t.Fatal("start forwarded before fixture substitution could run")
	default:
	}
	close(s.release)
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("unchanged start was not forwarded")
		}
	case <-time.After(time.Second):
		t.Fatal("start barrier did not release")
	}
	body, err := io.ReadAll(r.Body)
	if err != nil || string(body) != "original body" || r.URL.Path != entry.Path || !reflect.DeepEqual(s.before, entry.Observation) {
		t.Fatal("barrier changed the original request or observation")
	}
}

func TestAnchorSubstitutionsPreserveOnlyTheMovedOriginal(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native race fixture uses Linux identity and symlinks")
	}
	for _, kind := range []string{"checkout root replaced by outside symlink", "checkout root replaced by real directory", "checkout ancestor replaced by symlink", "checkout ancestor replaced by directory"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			root, replacement := filepath.Join(dir, "parent", "checkout"), filepath.Join(dir, "other", "checkout")
			for _, path := range []string{root, replacement} {
				if err := os.MkdirAll(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			originalID, err := anchorIdentity(root)
			if err != nil {
				t.Fatal(err)
			}
			otherID, err := anchorIdentity(replacement)
			if err != nil {
				t.Fatal(err)
			}
			w := &world{dir: dir, doctor: &doctorWorld{checkout: root}, anchor: &anchorWorld{original: root, replacement: replacement}}
			if err = w.anchorSubstitute(kind); err != nil {
				t.Fatal(err)
			}
			movedID, err := anchorIdentity(w.anchor.original)
			if err != nil || movedID != originalID {
				t.Fatal("fixture lost the validated original inode")
			}
			replacedID, err := anchorIdentity(root)
			if err != nil || replacedID != otherID || replacedID == movedID {
				t.Fatal("old checkout pathname did not resolve to the substitute")
			}
		})
	}
}

func TestAnchorOracleRejectsEarlyRefusalAndMissingWorkload(t *testing.T) {
	s := &anchorWorld{original: t.TempDir(), replacement: t.TempDir()}
	w := &world{anchor: s, offline: &offlineRuntime{exit: 1, stderr: []byte("run.mount-refused")}}
	if w.anchorOriginal() == nil {
		t.Fatal("refusal before the submitted start passed as race safety")
	}
	s.held, s.reached = true, true
	if err := w.anchorOriginal(); err != nil {
		t.Fatal(err)
	}
	for _, code := range []int{0, 3, 70, 75} {
		w.offline.exit = code
		if w.anchorOriginal() == nil {
			t.Fatalf("missing workload with exit %d passed", code)
		}
	}
	w.offline.exit = 1
	w.offline.stdout = []byte("workload escaped before refusal")
	if w.anchorOriginal() == nil {
		t.Fatal("workload output before refusal passed")
	}
}

func TestAnchorOracleRejectsCopiesAndSubstitutedInodes(t *testing.T) {
	s := &anchorWorld{original: t.TempDir(), replacement: t.TempDir(), originalID: "1:2", nonce: "original"}
	w := &world{anchor: s, doctor: &doctorWorld{real: &doctorReal{serverVersion: "fixture"}}}
	if err := os.Mkdir(filepath.Join(s.original, "subdir"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.original, "subdir", "slice-17-edit"), []byte(s.nonce), 0600); err != nil {
		t.Fatal(err)
	}
	correct := []anchorReport{{Phase: "ready", Identity: s.originalID, Nonce: s.nonce, CWD: "/workspace/subdir"}, {Phase: "checked", Identity: s.originalID, Nonce: s.nonce, CWD: "/workspace/subdir"}}
	s.reports = correct
	if err := w.anchorExecutedOriginal(); err != nil {
		t.Fatal(err)
	}
	for _, identity := range []string{"", "1:3", "2:2"} {
		s.reports = append([]anchorReport{}, correct...)
		s.reports[0].Identity = identity // same nonce bytes cannot replace inode proof
		if w.anchorExecutedOriginal() == nil {
			t.Fatal("wrong native inode passed with matching copied contents")
		}
	}
	s.reports = nil
	if w.anchorExecutedOriginal() == nil {
		t.Fatal("unexecuted probe passed")
	}
}
