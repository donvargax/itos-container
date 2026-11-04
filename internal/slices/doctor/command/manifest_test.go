package command

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/cli"
	"github.com/donvargax/itos-container/internal/engine/manifest/port"
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/port/porttest"
)

func TestManifestPresentationEscapesOnlyTerminalLocations(t *testing.T) {
	for _, withJSON := range []bool{false, true} {
		var stdout, stderr bytes.Buffer
		u := &cli.UI{Stdout: &stdout, Stderr: &stderr}
		result := port.Result{Path: "config/ñ\u009b.json", Issues: []port.Issue{{Kind: port.Unsupported, Path: []string{"bad\n/~key"}}}}
		if code := PresentManifest(u, result, withJSON); code != 2 {
			t.Fatal(code)
		}
		if !strings.Contains(stderr.String(), `config/ñ\u009b.json/bad\n~1~0key:`) || strings.Contains(stderr.String(), "\u009b") {
			t.Fatal(stderr.String())
		}
		if withJSON {
			var body struct {
				Problems []manifestProblem
				Manifest manifestReport
			}
			if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body.Problems) != 1 || body.Problems[0].Path != "/bad\n~1~0key" || body.Problems[0].File != result.Path || body.Manifest.Valid || body.Manifest.Effective != nil {
				t.Fatalf("%#v", body)
			}
		} else if stdout.Len() != 0 {
			t.Fatal(stdout.String())
		}
	}
}

func TestPolicyOnlyAndPolicyFailureNeverCallManifest(t *testing.T) {
	for _, tc := range []struct {
		result     policy.Result
		policyOnly bool
		code       int
	}{
		{policy.Result{Policy: &policy.Policy{Version: 1, Manifest: "worker.json", Network: policy.Network{Mode: "offline"}}}, true, 0},
		{policy.Result{Issues: []policy.Issue{{Kind: policy.Missing}}}, false, 2},
		{policy.Result{Issues: []policy.Issue{{Kind: policy.Unreadable}}}, false, 3},
	} {
		var stdout, stderr bytes.Buffer
		policies := &porttest.Policies{Result: tc.result}
		manifests := &porttest.Manifests{}
		c := CLI{Policies: policies, Manifests: manifests, PolicyOnly: tc.policyOnly, JSON: true}
		if code := c.Run(&cli.UI{Stdout: &stdout, Stderr: &stderr}); code != tc.code {
			t.Fatal(code)
		}
		if len(manifests.Paths) != 0 || policies.Reads != 1 {
			t.Fatal(policies, manifests)
		}
	}
}

func TestCombinedScopesFailBeforeReading(t *testing.T) {
	var stdout, stderr bytes.Buffer
	policies := &porttest.Policies{}
	manifests := &porttest.Manifests{}
	c := CLI{Policies: policies, Manifests: manifests, PolicyOnly: true, ManifestOnly: true, JSON: true}
	if code := c.Run(&cli.UI{Stdout: &stdout, Stderr: &stderr}); code != 2 || policies.Reads != 0 || len(manifests.Paths) != 0 {
		t.Fatal(code, policies, manifests)
	}
}

func TestUnknownManifestFailureRemainsARefusal(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := PresentManifest(&cli.UI{Stdout: &stdout, Stderr: &stderr}, port.Result{Path: "a.json", Issues: []port.Issue{{Kind: port.None}}}, true); code != 2 {
		t.Fatal(code)
	}
	if !strings.Contains(stdout.String(), `"rule":"manifest.internal"`) {
		t.Fatal(stdout.String())
	}
}

func TestSuppliedMetadataPresentation(t *testing.T) {
	present := true
	for _, withJSON := range []bool{false, true} {
		var stdout, stderr bytes.Buffer
		result := port.Result{Path: "worker.json", Image: "requested-image", Effective: &port.Settings{Image: "requested-image", WorkspaceFolder: "/workspace", ContainerEnv: map[string]string{}}, Metadata: port.Metadata{Checked: true, Present: &present, Unused: []string{"/0/name"}}}
		u := &cli.UI{Stdout: &stdout, Stderr: &stderr}
		if code := PresentManifest(u, result, withJSON); code != 0 || stderr.Len() != 0 {
			t.Fatal(code, stderr.String())
		}
		if !withJSON && (!strings.Contains(stdout.String(), "devcontainer.metadata") || !strings.Contains(stdout.String(), "unused")) {
			t.Fatal(stdout.String())
		}
		stdout.Reset()
		result.Effective = nil
		result.Issues = []port.Issue{{Kind: port.MetadataFailure, Path: []string{"1", "initializeCommand"}}}
		if code := PresentManifest(u, result, withJSON); code != 2 {
			t.Fatal(code)
		}
		if !strings.Contains(stderr.String(), "requested-image label devcontainer.metadata/1/initializeCommand") {
			t.Fatal(stderr.String())
		}
		if withJSON {
			var body struct{ Problems []manifestProblem }
			if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body.Problems) != 1 || body.Problems[0].Rule != "manifest.metadata" || body.Problems[0].File != "" || body.Problems[0].Image != "requested-image" || body.Problems[0].Label != "devcontainer.metadata" {
				t.Fatalf("%#v", body)
			}
		}
	}
}
