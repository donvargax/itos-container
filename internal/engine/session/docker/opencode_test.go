package docker

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestOpenCodeEnvironmentOverridesImageDefaultsAndSorts(t *testing.T) {
	got := openCodeEnvironment([]string{"ZED=1", `OPENCODE_CONFIG_CONTENT={"share":"manual"}`, "OPENCODE_DISABLE_AUTOUPDATE=false", "OPENCODE_DISABLE_MODELS_FETCH=false", "OPENCODE_CONFIG=/image/opencode.json", "ALPHA=2"})
	want := []string{"ALPHA=2", "OPENCODE_CONFIG=/image/opencode.json", `OPENCODE_CONFIG_CONTENT={"share":"disabled"}`, "OPENCODE_DISABLE_AUTOUPDATE=true", "OPENCODE_DISABLE_MODELS_FETCH=true", "ZED=1"}
	if !slices.Equal(got, want) {
		t.Fatalf("environment %q, want %q", got, want)
	}
	if got := openCodeEnvironment(nil); len(got) != 3 {
		t.Fatalf("profile alone has %d entries", len(got))
	}
}

func TestOpenCodeWorkerEnvironmentIsAppliedAndVerified(t *testing.T) {
	for _, harness := range []bool{false, true} {
		fixture := credentialSDKFixture(t)
		fixture.Binding.Home, fixture.Binding.OpenCode = true, harness
		token := strings.Repeat("a", 64)
		fixture.Record.Home = port.HomeReference{Scope: token, ID: token, Name: "itos-container-home-" + token, Created: "generation-1"}
		ctx := t.Context()
		c, failure := fixture.Engine.OpenRun(ctx, fixture.Binding)
		if failure != "" {
			t.Fatal(failure)
		}
		argv := []string{"opencode", "--standalone", "run"}
		id, failure := c.CreateRun(ctx, fixture.Record, argv)
		if failure != "" {
			t.Fatal(failure)
		}
		if failure := c.VerifyRun(ctx, id, argv); failure != "" {
			t.Fatal("the worker boundary failed", failure)
		}
		var created []string
		for _, traffic := range fixture.Traffic() {
			if _, body, ok := strings.Cut(string(traffic), "\n"); ok && strings.HasPrefix(string(traffic), "POST") && strings.Contains(string(traffic), "/containers/create") {
				var request struct{ Env []string }
				if json.Unmarshal([]byte(body), &request) != nil {
					t.Fatal("unreadable create request")
				}
				created = request.Env
			}
		}
		for _, want := range []string{`OPENCODE_CONFIG_CONTENT={"share":"disabled"}`, "OPENCODE_DISABLE_AUTOUPDATE=true", "OPENCODE_DISABLE_MODELS_FETCH=true"} {
			if slices.Contains(created, want) != harness {
				t.Fatalf("harness %v: created environment %q and %q", harness, created, want)
			}
		}
		if harness {
			fixture.Defect("opencode environment")
			if failure := c.VerifyRun(ctx, id, argv); failure != port.IsolationUnverified {
				t.Fatal("a worker without the exact profile passed inspection", failure)
			}
		}
		c.Close()
	}
}
