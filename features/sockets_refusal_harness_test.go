package features

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSocketStartupRefusalEvidence(t *testing.T) {
	const diagnostic = `run.isolation-unverified "": owned run could not be completed`
	for _, tt := range []struct {
		name     string
		exit     int
		stderr   string
		requests []offlineRequest
		created  []string
		want     string
	}{
		{name: "typed refusal", exit: 1, stderr: diagnostic},
		{name: "successful run", exit: 0, want: "missing typed socket isolation refusal"},
		{name: "wrong startup failure", exit: 75, stderr: "run.execution-unavailable", want: "not product red evidence"},
		{name: "missing diagnostic", exit: 1, want: "missing typed socket isolation refusal"},
		{name: "different diagnostic suffix", exit: 1, stderr: "run.isolation-unverified-other", want: "missing typed socket isolation refusal"},
		{name: "wrong exit", exit: 75, stderr: diagnostic, want: "missing typed socket isolation refusal"},
		{name: "failed create", exit: 1, stderr: diagnostic,
			requests: []offlineRequest{{Method: "POST", Path: "/containers/create", Status: 500}}, want: "create requests=1"},
		{name: "removed worker", exit: 1, stderr: diagnostic,
			requests: []offlineRequest{{Method: "POST", Path: "/containers/create", Status: 201}, {Method: "DELETE", Path: "/containers/owned", Status: 204}},
			created:  []string{"owned"}, want: "create requests=1 worker IDs=1"},
		{name: "missing create journal", exit: 1, stderr: diagnostic, created: []string{"owned"}, want: "worker IDs=1"},
		{name: "read-only inspection", exit: 1, stderr: diagnostic,
			requests: []offlineRequest{{Method: "GET", Path: "/info", Status: 200}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := socketRefusal(tt.exit, []byte(tt.stderr), tt.requests, tt.created)
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("refusal evidence = %v, want %q", err, tt.want)
			}
		})
	}
}

// Exercise the new per-location fixtures without the product or an engine.
// The only executable is the reviewed host nonce listener/control probe.
func TestSocketStartupListenerControls(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("trusted probe uses Linux amd64 Unix socket syscalls")
	}
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(t.TempDir(), "slice-19-probe")
	if err = buildSocketProbe(root, probe); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(removeCallerPath)
	for _, dir := range []string{"", "nested/a/b/c/d/e", ".git", ".git/nested/a/b/c/d/e"} {
		for _, op := range []string{"stream-connect", "seqpacket-connect", "dgram-sendto"} {
			t.Run(dir+"/"+op, func(t *testing.T) {
				s := socketLocalFixture(t, root, probe, "existing")
				s.config.Targets = []socketTarget{{Name: "startup", Op: op, Path: filepath.Join(dir, "socket")}}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := s.publish(ctx); err != nil {
					t.Fatal(err)
				}
				for _, edge := range []string{"before", "after"} {
					if err := s.hostControl(ctx, "startup-"+edge); err != nil {
						t.Fatal(err)
					}
				}
				if err := s.hostControlsFor([]string{"startup"}); err != nil {
					t.Fatal(err)
				}
				s.controls = s.controls[:1]
				if err := s.hostControlsFor([]string{"startup"}); err == nil {
					t.Fatal("missing after control accepted as refusal evidence")
				}
			})
		}
	}
}
