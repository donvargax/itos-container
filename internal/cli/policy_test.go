package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/policy/port"
)

func TestPolicyDiagnosticExitPrecedence(t *testing.T) {
	for _, tc := range []struct {
		issues []port.Issue
		exit   int
	}{
		{[]port.Issue{{Kind: port.Missing}}, 2},
		{[]port.Issue{{Kind: port.Type}, {Kind: port.Unreadable}}, 3},
		{[]port.Issue{{Kind: port.Unreadable}, {Kind: port.Type}}, 3},
		{[]port.Issue{{Kind: port.None}}, 70},
	} {
		var stdout, stderr bytes.Buffer
		ui := UI{Stdout: &stdout, Stderr: &stderr}
		if code := ui.Policy(port.Result{Issues: tc.issues}, true); code != tc.exit {
			t.Fatalf("code %d", code)
		}
		var output struct {
			OK       bool
			Problems []policyProblem
			Policy   any
			Scope    string
			Schema   int
		}
		if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
			t.Fatal(err)
		}
		if output.OK || output.Policy != nil || output.Scope != "policy" || output.Schema != 1 || len(output.Problems) != len(tc.issues) {
			t.Fatalf("%#v", output)
		}
	}
}

func TestPolicyPlainFailureAndPointers(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ui := UI{Stdout: &stdout, Stderr: &stderr}
	code := ui.Policy(port.Result{Issues: []port.Issue{{Kind: port.UnknownKey, Path: []string{"a~/b", "1"}}}}, false)
	if code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "itos-container: itos-container.yaml/a~0~1b/1:") ||
		!strings.Contains(stderr.String(), "remove fields") {
		t.Fatalf("%d %q %q", code, stdout.String(), stderr.String())
	}
}
