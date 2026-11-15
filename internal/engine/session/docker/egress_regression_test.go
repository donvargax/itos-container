package docker

import (
	"slices"
	"testing"

	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	st "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/moby/moby/api/types/mount"
)

func TestCreateProxyUsesLinuxDaemonExecutablePath(t *testing.T) {
	for _, tc := range []struct {
		path string
		want sp.Outcome
	}{
		{"/usr/local/bin/itos-container", ""},
		{"", sp.Refused},
		{"bin/itos-container", sp.Refused},
		{`C:\bin\itos-container.exe`, sp.Refused},
		{`\\server\share\itos-container.exe`, sp.Refused},
	} {
		t.Run(tc.path, func(t *testing.T) {
			e, b, api, _ := egressSDKFixture(t)
			e.Executable = tc.path
			c, failure := e.OpenRun(t.Context(), b)
			if failure != "" {
				t.Fatal(failure)
			}
			defer c.Close()
			id, failure := c.CreateProxy(t.Context(), st.EgressRecords(b)[5])
			if failure != tc.want {
				t.Fatalf("daemon executable %q = %s, want %s", tc.path, failure, tc.want)
			}
			if tc.want != "" {
				if id != "" || len(api.requests) != 0 {
					t.Error("invalid daemon path reached engine", id, api.requests)
				}
				return
			}
			want := []mount.Mount{{Type: mount.TypeBind, Source: tc.path, Target: "/itos-container", ReadOnly: true}}
			if !sp.HexID(id) || len(api.containerBodies) != 1 || !slices.Equal(api.containerBodies[0].HostConfig.Mounts, want) {
				t.Error("daemon executable mount changed", id, api.containerBodies)
			}
		})
	}
}

func TestCreateEgressRunWithNullEnvironment(t *testing.T) {
	e, b, api, _ := egressSDKFixture(t)
	b.Environment = "null"
	c, r := createEgressFixture(t, e, b)
	if failure := c.Start(t.Context(), r.Egress.Proxy.ID); failure != "" {
		t.Fatal(failure)
	}
	r.Stage = "intent"
	id, failure := c.CreateRun(t.Context(), r, []string{"probe"})
	if failure != "" || !sp.HexID(id) {
		t.Fatal("null overlay prevented worker creation", id, failure)
	}
	if failure := c.VerifyRun(t.Context(), id, []string{"probe"}); failure != "" {
		t.Fatal("worker verification", failure)
	}
	env := api.containerBodies[1].Env
	for _, value := range []string{"HOME=/tmp", "TMPDIR=/tmp", "HTTP_PROXY=http://172.20.0.2:3128", "HTTPS_PROXY=http://172.20.0.2:3128", "http_proxy=http://172.20.0.2:3128", "https_proxy=http://172.20.0.2:3128", "NO_PROXY=localhost,127.0.0.1,::1", "no_proxy=localhost,127.0.0.1,::1"} {
		if !slices.Contains(env, value) {
			t.Errorf("null overlay lost %q: %v", value, env)
		}
	}
}
