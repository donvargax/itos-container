package command

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/cli"
	sockets "github.com/donvargax/itos-container/internal/engine/checkoutsockets/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/port/porttest"
)

// inspected is a readiness result whose checkout and engine inspections
// succeeded, as detection requires.
func inspected(issues ...port.Issue) port.Result {
	return port.Result{
		EngineDeadline: time.Now().Add(5 * time.Second),
		Checkout:       &port.CheckoutIdentity{Root: "/checkout"},
		Engine:         &port.EngineIdentity{ID: "engine-A"},
		Issues:         issues,
		Ready:          len(issues) == 0,
	}
}

type doctorBody struct {
	OK         bool
	Inspection map[string]any
	Problems   []readinessProblem
}

func runSockets(t *testing.T, result port.Result, selection sockets.Selection, withJSON bool) (code int, stdout, stderr string, fake *porttest.Sockets, body doctorBody) {
	t.Helper()
	var out, errs bytes.Buffer
	fake = &porttest.Sockets{Selection: selection}
	c := CLI{Readiness: &porttest.Readiness{Result: result}, Sockets: fake, JSON: withJSON}
	code = c.Run(&cli.UI{Stdout: &out, Stderr: &errs})
	if withJSON {
		if err := json.Unmarshal(out.Bytes(), &body); err != nil {
			t.Fatal(err, out.String())
		}
	}
	return code, out.String(), errs.String(), fake, body
}

func TestDoctorNamesTheConfirmedBackend(t *testing.T) {
	for _, c := range []struct {
		backend sockets.Backend
		name    string
		line    string
	}{
		{sockets.Landlock, "landlock", "checkout socket backend: landlock"},
		{sockets.GVisor, "gvisor", "checkout socket backend: gvisor"},
		{sockets.Neither, "neither", "checkout socket backend: neither; late host sockets reachable"},
	} {
		result := inspected()
		code, _, _, fake, body := runSockets(t, result, sockets.Selection{Backend: c.backend}, true)
		if code != 0 || !body.OK || body.Inspection["ready"] != true || len(body.Problems) != 0 {
			t.Fatalf("%s: exit %d, %+v", c.name, code, body)
		}
		if got := body.Inspection["checkout_sockets"]; !jsonEqual(got, map[string]any{"backend": c.name}) {
			t.Fatalf("%s: checkout_sockets = %#v", c.name, got)
		}
		if len(fake.Checkouts) != 1 || fake.Checkouts[0] != "/checkout" || fake.Ended[0] || fake.Deadline.IsZero() {
			t.Fatalf("%s: detection ran %+v, want once for the validated checkout under the bound", c.name, fake)
		}
		if len(fake.Engines) != 1 || fake.Engines[0] != *result.Engine {
			t.Fatalf("%s: detection used engine identities %+v, want %v", c.name, fake.Engines, *result.Engine)
		}
		code, stdout, _, _, _ := runSockets(t, inspected(), sockets.Selection{Backend: c.backend}, false)
		lines := strings.Split(stdout, "\n")
		if code != 0 || !contains(lines, c.line) || !contains(lines, "runtime isolation is not established") || !contains(lines, "inspection ready") {
			t.Fatalf("%s: exit %d, %q", c.name, code, stdout)
		}
	}
}

func TestDoctorUsesTheEngineInspectionDeadlineForSocketDetection(t *testing.T) {
	result := inspected()
	result.EngineDeadline = time.Now().Add(time.Second)
	code, _, _, fake, _ := runSockets(t, result, sockets.Selection{Backend: sockets.Landlock}, true)
	if code != 0 || !fake.Deadline.Equal(result.EngineDeadline) {
		t.Fatalf("exit %d, detection deadline %v, want %v", code, fake.Deadline, result.EngineDeadline)
	}

	result.EngineDeadline = time.Now().Add(-time.Second)
	code, _, _, _, body := runSockets(t, result, sockets.Selection{Backend: sockets.Landlock}, true)
	if code != 75 || body.OK || body.Inspection["ready"] != false || body.Inspection["checkout_sockets"] != nil || len(body.Problems) != 1 || body.Problems[0].Rule != "doctor.engine.timeout" {
		t.Fatalf("expired engine inspection reported exit %d and %+v", code, body)
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func contains(lines []string, line string) bool {
	for _, l := range lines {
		if l == line {
			return true
		}
	}
	return false
}

func TestPlainBackendLineFollowsTheImageLine(t *testing.T) {
	result := inspected()
	result.Image = &port.ImageIdentity{Reference: "img", ID: "sha256:x"}
	_, stdout, _, _, _ := runSockets(t, result, sockets.Selection{Backend: sockets.Landlock}, false)
	if !strings.Contains(stdout, "image: img (sha256:x)\ncheckout socket backend: landlock\n") {
		t.Fatalf("backend line is not right after the image line: %q", stdout)
	}
}

func TestDetectionFailureNamesNoBackendAndRefusesReadiness(t *testing.T) {
	for _, c := range []struct {
		failure sockets.Failure
		rule    string
		exit    int
		field   string
	}{
		{sockets.ABIQuery, "doctor.sockets.kernel", 75, "landlock_abi"},
		{sockets.ABIInvalid, "doctor.sockets.kernel", 75, "landlock_abi"},
		{sockets.Unsupported, "doctor.sockets.kernel", 75, "landlock_abi"},
		{sockets.NoProbe, "doctor.sockets.kernel", 75, "landlock_abi"},
		{sockets.RuntimeInspect, "doctor.sockets.runtimes", 75, "runtimes"},
		{sockets.RuntimeRegistration, "doctor.sockets.runsc", 1, "runtimes.runsc"},
		{sockets.Cancelled, "doctor.engine.timeout", 75, "DOCKER_HOST"},
		{sockets.None, "doctor.sockets.kernel", 75, "landlock_abi"}, // the zero selection confirms nothing
	} {
		// A failure never carries a backend, even a misreported one.
		code, _, stderr, _, body := runSockets(t, inspected(), sockets.Selection{Backend: sockets.Landlock, Failure: c.failure}, true)
		if c.failure == sockets.None {
			code, _, stderr, _, body = runSockets(t, inspected(), sockets.Selection{}, true)
		}
		if code != c.exit || body.OK || body.Inspection["ready"] != false {
			t.Fatalf("%v: exit %d, %+v", c.failure, code, body)
		}
		if _, present := body.Inspection["checkout_sockets"]; present {
			t.Fatalf("%v: a failed detection reports a backend: %+v", c.failure, body.Inspection)
		}
		if len(body.Problems) != 1 {
			t.Fatalf("%v: problems %+v", c.failure, body.Problems)
		}
		p := body.Problems[0]
		if p.Rule != c.rule || p.Exit != c.exit || p.Field != c.field || p.Message == "" || p.Fix == "" {
			t.Fatalf("%v: problem %+v", c.failure, p)
		}
		if !strings.Contains(stderr, p.Message) {
			t.Fatalf("%v: stderr %q lacks the message", c.failure, stderr)
		}
	}
}

func TestPlainDetectionFailureIsNotReady(t *testing.T) {
	code, stdout, stderr, _, _ := runSockets(t, inspected(), sockets.Selection{Failure: sockets.RuntimeRegistration}, false)
	if code != 1 || strings.Contains(stdout, "inspection ready") || strings.Contains(stdout, "checkout socket backend") || !strings.Contains(stderr, "runsc") {
		t.Fatalf("exit %d\nstdout %q\nstderr %q", code, stdout, stderr)
	}
}

func TestDetectionFollowsImageProblemsAndTheExitIsTheHighest(t *testing.T) {
	result := inspected(port.Issue{Kind: port.ImageMissing, Subject: "img"})
	code, _, _, fake, body := runSockets(t, result, sockets.Selection{Failure: sockets.RuntimeInspect}, true)
	if code != 75 || len(fake.Checkouts) != 1 || len(body.Problems) != 2 || body.Problems[0].Rule != "doctor.image.missing" || body.Problems[1].Rule != "doctor.sockets.runtimes" {
		t.Fatalf("exit %d, %+v", code, body.Problems)
	}
	// Whatever the image outcome, the backend is still named.
	code, _, _, fake, body = runSockets(t, result, sockets.Selection{Backend: sockets.GVisor}, true)
	if code != 3 || len(fake.Checkouts) != 1 || body.Inspection["checkout_sockets"] == nil {
		t.Fatalf("exit %d, %+v", code, body)
	}
}

func TestNoDetectionAfterACheckoutHostOrEngineRefusal(t *testing.T) {
	noEngine := inspected()
	noEngine.Engine = nil
	noCheckout := inspected()
	noCheckout.Checkout = nil
	for name, result := range map[string]port.Result{
		"no checkout":       noCheckout,
		"no engine":         noEngine,
		"engine changed":    inspected(port.Issue{Kind: port.EngineChanged, Subject: "DOCKER_HOST"}),
		"checkout changed":  inspected(port.Issue{Kind: port.CheckoutChanged, Subject: "/checkout"}),
		"engine timeout":    inspected(port.Issue{Kind: port.EngineTimeout, Subject: "DOCKER_HOST"}),
		"host unsupported":  inspected(port.Issue{Kind: port.HostUnsupported, Subject: "host"}),
		"image then engine": inspected(port.Issue{Kind: port.ImageInspect, Subject: "img"}, port.Issue{Kind: port.EngineChanged, Subject: "DOCKER_HOST"}),
	} {
		_, _, _, fake, body := runSockets(t, result, sockets.Selection{Backend: sockets.Landlock}, true)
		if len(fake.Checkouts) != 0 {
			t.Fatalf("%s: detection ran", name)
		}
		if _, present := body.Inspection["checkout_sockets"]; present {
			t.Fatalf("%s: a backend is reported without detection", name)
		}
		for _, p := range body.Problems {
			if strings.HasPrefix(p.Rule, "doctor.sockets.") {
				t.Fatalf("%s: sockets problem %+v", name, p)
			}
		}
	}
}

func TestEveryImageOutcomeStillDetects(t *testing.T) {
	for _, kind := range []port.Failure{port.ImageMissing, port.ImageInspect, port.ImageDigest, port.ImagePlatform, port.ImageUser, port.EgressImage, port.EgressImageMissing} {
		_, _, _, fake, _ := runSockets(t, inspected(port.Issue{Kind: kind, Subject: "img"}), sockets.Selection{Backend: sockets.Neither}, true)
		if len(fake.Checkouts) != 1 {
			t.Fatalf("image failure %d skipped detection", kind)
		}
	}
}

func TestWithoutADetectorDoctorReportsNoBackend(t *testing.T) {
	// The same inspection names a backend only when a detector is assembled.
	_, _, _, _, body := runSockets(t, inspected(), sockets.Selection{Backend: sockets.Landlock}, true)
	if !jsonEqual(body.Inspection["checkout_sockets"], map[string]any{"backend": "landlock"}) {
		t.Fatal("a detector must produce the backend", body.Inspection)
	}
	var out, errs bytes.Buffer
	c := CLI{Readiness: &porttest.Readiness{Result: inspected()}, JSON: true}
	if code := c.Run(&cli.UI{Stdout: &out, Stderr: &errs}); code != 0 || strings.Contains(out.String(), "checkout_sockets") {
		t.Fatal(code, out.String())
	}
}
