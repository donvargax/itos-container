package features

import (
	"context"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

func TestAnchorRecoveryStepsDefined(t *testing.T) {
	definitions := &lifecycleDefinitions{t: t}
	(&world{}).anchorRecoverySteps(definitions)
	suite := godog.TestSuite{Name: "slice-17-anchor-recovery-definitions", Options: &godog.Options{Paths: []string{"anchors.feature"}, Tags: "@engine", Output: io.Discard}}
	features, err := suite.RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	cases := 0
	for _, f := range features {
		for _, scenario := range f.Pickles {
			cases++
			for _, step := range scenario.Steps {
				matches := 0
				for _, pattern := range definitions.patterns {
					if pattern.MatchString(step.Text) {
						matches++
					}
				}
				if matches != 1 {
					t.Errorf("%q has %d definitions", step.Text, matches)
				}
			}
		}
	}
	if cases != 7 {
		t.Fatalf("native anchor recovery rows = %d, want 7", cases)
	}
}

func TestAnchorRecoveryGuard(t *testing.T) {
	t.Setenv("ITOS_CONTAINER_ENGINE_TESTS", "0")
	for _, mode := range []string{"loss", "retry", "create", "start", "foreign", "pair"} {
		w := &world{}
		if err := w.anchorsFixture(mode); err == nil || w.doctor != nil || w.anchors != nil {
			t.Fatalf("%s fixture initialized outside CI engine tier", mode)
		}
	}
}

func TestAnchorRecoveryProbeCompilesWithoutExecution(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := recoveryBuildHelper(root, filepath.Join(t.TempDir(), "slice-17-recovery-probe"), anchorRecoveryProbeSource); err != nil {
		t.Fatal(err)
	}
	if err := recoveryBuildHelper(root, filepath.Join(t.TempDir(), "slice-17-recovery-trace"), recoveryTraceSource); err != nil {
		t.Fatal(err)
	}
}

func TestAnchorRecoveryResponseBarrier(t *testing.T) {
	for _, operation := range []string{"create", "start"} {
		t.Run(operation, func(t *testing.T) {
			s := &anchorRecoveryWorld{mode: operation, ready: make(chan struct{}), release: make(chan struct{})}
			a := &offlineAPI{w: &world{anchors: s}}
			path := "/containers/create"
			if operation == "start" {
				path = "/containers/owned/start"
			}
			q := offlineRequest{Method: "POST", Path: path}
			r := httptest.NewRequest("POST", path, nil)
			a.anchorsResponse(r, q, 500)
			select {
			case <-s.ready:
				t.Fatal("failed response became a successful barrier")
			default:
			}
			done := make(chan struct{})
			go func() { a.anchorsResponse(r, q, 201); close(done) }()
			select {
			case <-s.ready:
			case <-time.After(time.Second):
				t.Fatal("successful response did not reach barrier")
			}
			select {
			case <-done:
				t.Fatal("response escaped before controller kill")
			default:
			}
			close(s.release)
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("barrier did not release")
			}
			a.anchorsResponse(r, q, 201) // fresh recovery is not held again
		})
	}
}

func TestAnchorRecoveryReleaseFaultAndCancelledBarrier(t *testing.T) {
	s := &anchorRecoveryWorld{failDelete: true}
	a := &offlineAPI{w: &world{anchors: s}, requests: make([]offlineRequest, 1)}
	q := offlineRequest{Method: "DELETE", Path: "/volumes/owned"}
	r := httptest.NewRequest("DELETE", q.Path, nil)
	out := httptest.NewRecorder()
	if !a.anchorsBefore(out, r, q, 0) || out.Code != 503 || a.requests[0].Status != 503 {
		t.Fatal("volume-only fault was not recorded")
	}
	if a.anchorsBefore(out, r, offlineRequest{Method: "GET", Path: q.Path}, 0) {
		t.Fatal("volume inspection was faulted")
	}
	if a.anchorsBefore(out, r, offlineRequest{Method: "DELETE", Path: "/containers/owned"}, 0) {
		t.Fatal("worker removal was faulted")
	}
	s.failDelete = false
	s.mode = "pair"
	s.ready = make(chan struct{})
	s.release = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !a.anchorsBefore(out, r.WithContext(ctx), q, 0) {
		t.Fatal("cancelled barrier forwarded deletion")
	}
	if !s.parkLost {
		t.Fatal("a park the product abandoned was not recorded")
	}
}

func TestAnchorRecoveryReleasedBarrierIsNotLost(t *testing.T) {
	s := &anchorRecoveryWorld{mode: "pair", ready: make(chan struct{}), release: make(chan struct{})}
	a := &offlineAPI{w: &world{anchors: s}, requests: make([]offlineRequest, 1)}
	q := offlineRequest{Method: "DELETE", Path: "/volumes/owned"}
	r := httptest.NewRequest("DELETE", q.Path, nil)
	close(s.release)
	if a.anchorsBefore(httptest.NewRecorder(), r, q, 0) || s.parkLost {
		t.Fatal("a released park was dropped or recorded as lost")
	}
}

func TestAnchorRecoveryOraclesRejectMissingAndSubstitutedEvidence(t *testing.T) {
	w := &world{anchor: &anchorWorld{originalID: "1:2", nonce: "original", records: map[string]sessionRecord{"/protected/anchor": {}}}, recovery: &recoveryNative{}, anchors: &anchorRecoveryWorld{}}
	if _, err := w.anchorsOwned(); err == nil {
		t.Fatal("missing native anchor passed")
	}
	p := anchorRecoveryProbe{Sequence: 2, Identity: "1:2", Nonce: "original", Denied: map[string]bool{"/workspace/slice-17-replacement": true, "/workspace/slice-17-nested/marker": true, "/protected/anchor": true}, Controls: map[string]bool{"before": true, "after": true}}
	if err := w.anchorsProbeCheck(p); err != nil {
		t.Fatal(err)
	}
	for _, identity := range []string{"", "1:3", "2:2"} {
		p.Identity = identity
		if w.anchorsProbeCheck(p) == nil {
			t.Fatal("copied bytes with wrong inode passed")
		}
	}
	p.Identity = "1:2"
	delete(p.Denied, "/protected/anchor")
	if w.anchorsProbeCheck(p) == nil {
		t.Fatal("missing protected-record attempt passed")
	}
	if anchorsProblem(nil, "recovery.not-owned", 1) == nil {
		t.Fatal("unexecuted refusal passed")
	}
}

func TestAnchorRecoveryStatusRequiresAnchorField(t *testing.T) {
	id := strings.Repeat("a", 64)
	w := &world{recovery: &recoveryNative{id: id}, approval: &approvalWorld{project: "project", fingerprint: "fingerprint"}}
	p := &recoveryProcess{done: make(chan struct{})}
	close(p.done)
	_, _ = p.out.Write([]byte(`{"schema":1,"ok":true,"problems":[],"sessions":[{"id":"` + id + `","project":"project","fingerprint":"fingerprint","state":"running","resource":"running","controller":"absent","cleanup":"pending","workload_exit":null,"stop_candidate":true,"retry":["itos","container","stop","` + id + `"]}]}`))
	if _, err := w.anchorsEntry(p, 0); err == nil || !strings.Contains(err.Error(), "account for the original anchor") {
		t.Fatalf("worker-only result did not fail the anchor assertion: %v", err)
	}
}
