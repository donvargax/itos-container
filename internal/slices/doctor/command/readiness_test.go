package command

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/cli"
	manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"
	policy "github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/port/porttest"
)

func TestOrdinaryDoctorUsesReadinessAndPreservesDiagnostics(t *testing.T) {
	for _, withJSON := range []bool{false, true} {
		var stdout, stderr bytes.Buffer
		r := &porttest.Readiness{Result: port.Result{
			Policy:   &policy.Result{Issues: []policy.Issue{{Kind: policy.Unreadable}}},
			Manifest: &manifest.Result{Path: "worker.json", Issues: []manifest.Issue{{Kind: manifest.Missing}}},
			Issues:   []port.Issue{{Kind: port.ImageDigest, Subject: "requested"}, {Kind: port.EngineTimeout, Subject: "DOCKER_HOST"}},
		}}
		c := CLI{Readiness: r, JSON: withJSON}
		if code := c.Run(&cli.UI{Stdout: &stdout, Stderr: &stderr}); code != 75 || r.Reads != 1 {
			t.Fatal(code, r.Reads)
		}
		if !strings.Contains(stderr.String(), "worker.json") {
			t.Fatal(stderr.String())
		}
		if withJSON {
			var body struct {
				OK         bool
				Inspection inspectionReport
				Problems   []struct {
					Rule string
					Exit int
				}
				Manifest manifestReport
			}
			if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.OK || body.Inspection.Ready || body.Manifest.Valid || len(body.Problems) != 4 || body.Problems[0].Exit != 3 || body.Problems[1].Exit != 2 || body.Problems[2].Exit != 1 || body.Problems[3].Exit != 75 {
				t.Fatal(stdout.String())
			}
		}
	}
}

func TestUnknownReadinessFailureCannotSucceed(t *testing.T) {
	p := readinessDiagnostic(port.Issue{Kind: port.Failure(255), Subject: "inspection"})
	if p.Exit != 70 || p.Rule != "doctor.internal" || p.Field != "inspection" || p.Message == "" || p.Fix == "" {
		t.Fatal(p)
	}
}

func TestUnsupportedHostReadinessDiagnostic(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := &porttest.Readiness{Result: port.Result{
		Issues: []port.Issue{{Kind: port.HostUnsupported, Subject: "host"}},
	}}
	c := CLI{Readiness: r, JSON: true}
	if code := c.Run(&cli.UI{Stdout: &stdout, Stderr: &stderr}); code != 1 || r.Reads != 1 {
		t.Fatalf("unsupported host returned exit %d after %d inspections", code, r.Reads)
	}
	var body struct {
		Schema     int
		OK         *bool
		Inspection map[string]any
		Problems   []readinessProblem
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Schema != 1 || body.OK == nil || *body.OK || !reflect.DeepEqual(body.Inspection, map[string]any{"ready": false}) || len(body.Problems) != 1 {
		t.Fatalf("unsupported host must report refusal without uninspected identities: %s", stdout.String())
	}
	p := body.Problems[0]
	if p.Rule != "doctor.host.unsupported" || p.Exit != 1 || p.Field != "host" || p.Path != nil || p.Image != "" || p.Message == "" || p.Fix == "" {
		t.Fatalf("incomplete unsupported-host diagnostic: %+v", p)
	}
}
