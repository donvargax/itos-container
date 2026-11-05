package command

import (
	"bytes"
	"encoding/json"
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
