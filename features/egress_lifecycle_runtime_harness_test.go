package features

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func egressLifeRuntimeJournal() []egressLifeRequest {
	return []egressLifeRequest{
		{method: "POST", path: "/volumes/create", status: 201, response: []byte(`{"Name":"anchor"}`)},
		{method: "POST", path: "/networks/create", status: 201, body: []byte(`{"Labels":{"io.itos-container.kind":"internal-network"}}`), response: []byte(`{"Id":"internal"}`)},
		{method: "POST", path: "/networks/create", status: 201, body: []byte(`{"Labels":{"io.itos-container.kind":"egress-network"}}`), response: []byte(`{"Id":"egress"}`)},
		{method: "POST", path: "/containers/create", status: 201, body: []byte(`{"Labels":{"io.itos-container.kind":"egress-proxy"}}`), response: []byte(`{"Id":"proxy"}`)},
		{method: "POST", path: "/networks/egress/connect", status: 200, body: []byte(`{"Container":"proxy"}`)},
		{method: "POST", path: "/containers/proxy/start", status: 204},
		{method: "POST", path: "/containers/create", status: 201, body: []byte(`{"Labels":{}}`), response: []byte(`{"Id":"worker"}`)},
		{method: "POST", path: "/containers/worker/attach", status: 101},
		{method: "POST", path: "/containers/worker/start", status: 204},
		{method: "POST", path: "/containers/worker/wait", status: 200},
		{method: "POST", path: "/containers/worker/kill", status: 204},
		{method: "POST", path: "/containers/proxy/wait", status: 200},
		{method: "DELETE", path: "/containers/worker", status: 204},
	}
}

func TestEgressLifecycleRuntimeRejectsFallbackAndRestarts(t *testing.T) {
	base := egressLifeRuntimeJournal()
	if err := egressLifeRuntimeRoutes(base); err != nil {
		t.Fatal(err)
	}
	for _, extra := range []egressLifeRequest{
		base[1], base[3], base[5], base[6], base[8],
		{method: "POST", path: "/containers/worker/restart"},
		{method: "POST", path: "/containers/proxy/restart"},
		{method: "POST", path: "/containers/worker/exec"},
		{method: "POST", path: "/containers/foreign/start"},
		{method: "POST", path: "/networks/foreign/connect", body: []byte(`{"Container":"worker"}`)},
	} {
		if err := egressLifeRuntimeRoutes(append(append([]egressLifeRequest(nil), base...), extra)); err == nil {
			t.Errorf("accepted %s %s", extra.method, extra.path)
		}
	}
	for _, index := range []int{0, 1, 2, 3, 5, 6, 8} {
		copy := append([]egressLifeRequest{}, base[:index]...)
		copy = append(copy, base[index+1:]...)
		if err := egressLifeRuntimeRoutes(copy); err == nil {
			t.Errorf("accepted missing original event %d", index)
		}
	}
}

func TestEgressLifecycleGraceRequiresObservedSignalsAndFullBudget(t *testing.T) {
	after := time.Now()
	term := egressLifeRequest{method: "POST", path: "/containers/worker/kill", query: "signal=SIGTERM", status: 204, at: after.Add(time.Millisecond)}
	kill := term
	kill.query, kill.at = "signal=SIGKILL", term.at.Add(10*time.Second)
	if err := egressLifeSignalOrder([]egressLifeRequest{term}, "worker", after, false); err != nil {
		t.Fatal(err)
	}
	if err := egressLifeSignalOrder([]egressLifeRequest{term, kill}, "worker", after, true); err != nil {
		t.Fatal(err)
	}
	early, failed, before, wrong := kill, term, term, term
	early.at, failed.status, before.at, wrong.query = term.at.Add(9*time.Second), 500, after.Add(-time.Second), "signal=SIGINT"
	for _, tc := range []struct {
		name    string
		journal []egressLifeRequest
		forced  bool
	}{
		{"no signal", nil, false}, {"missing kill", []egressLifeRequest{term}, true},
		{"early kill", []egressLifeRequest{term, early}, true}, {"failed term", []egressLifeRequest{failed, kill}, true},
		{"preceded loss", []egressLifeRequest{before}, false}, {"wrong signal", []egressLifeRequest{wrong}, false},
		{"kill before term", []egressLifeRequest{kill, term}, true}, {"duplicate term", []egressLifeRequest{term, term}, false},
		{"unexpected kill", []egressLifeRequest{term, kill}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := egressLifeSignalOrder(tc.journal, "worker", after, tc.forced); err == nil {
				t.Fatal("invalid termination passed")
			}
		})
	}
}

func TestEgressLifecycleRouteOracleNeedsAttemptsControlsAndAbsence(t *testing.T) {
	for _, change := range []string{"clean", "missing probe", "probe succeeded", "missing error", "received nonce", "failed control", "no controls"} {
		t.Run(change, func(t *testing.T) {
			dir := t.TempDir()
			e := &egressEndpoint{key: "runtime", dir: dir, controls: []error{nil, nil}}
			run := &egressNativeRun{steps: []egressStep{{Op: "direct", Mode: "grace", Target: "v4", Nonce: "attempt4"}, {Op: "direct", Mode: "grace", Target: "v6", Nonce: "attempt6"}}, report: egressProbeReport{Results: []egressProbeResult{{Op: "direct", Mode: "grace", Target: "v4", Error: "unreachable"}, {Op: "direct", Mode: "grace", Target: "v6", Error: "unreachable"}}}}
			receipts := []egressReceipt{{Kind: "conn", Handshake: true}, {Kind: "nonce", Nonce: "control"}}
			switch change {
			case "missing probe":
				run.report.Results = run.report.Results[:1]
			case "probe succeeded":
				run.report.Results[0].OK = true
			case "missing error":
				run.report.Results[0].Error = ""
			case "received nonce":
				receipts[1].Nonce = "attempt4"
			case "failed control":
				e.controls[0] = errors.New("no endpoint")
			case "no controls":
				e.controls = nil
			}
			var data []byte
			for _, r := range receipts {
				data = append(data, append(approvalJSON(r), '\n')...)
			}
			if err := os.WriteFile(filepath.Join(dir, "receipts.jsonl"), data, 0600); err != nil {
				t.Fatal(err)
			}
			w := &world{egress: &egressWorld{native: &egressNative{endpoints: map[string]*egressEndpoint{"runtime": e}}, lifeRuntime: &egressLifeRuntime{run: run}}}
			if err := w.egressLifeRoutesDenied("grace"); (err == nil) != (change == "clean") {
				t.Fatalf("route assertion: %v", err)
			}
		})
	}
}

// Exercise the actual trusted worker program without an engine. It must leave
// completed route receipts after TERM, and its ignoring mode must remain alive
// so the native scenario cannot mistake a voluntary exit for forced teardown.
func TestEgressLifecycleProbeContinuesAfterTermUntilKill(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("native probe targets Linux amd64")
	}
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	program := filepath.Join(t.TempDir(), "probe")
	if err := egressBuild(root, "features/testdata/slice-27/probe", program); err != nil {
		t.Fatal(err)
	}
	// Bind a local listener, complete the TCP dial, and close without TLS.
	// This yields a completed failed probe without external routes or DNS.
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	dir := t.TempDir()
	plan := filepath.Join(dir, "plan.json")
	steps := []egressStep{{Op: "wait-signal", Name: "loss"}, {Op: "direct", Mode: "grace", Name: "api.example.com", Method: "tcp4", Target: listener.Addr().String(), Nonce: "route-control"}, {Op: "hold-after-signal"}}
	if err := os.WriteFile(plan, approvalJSON(map[string]any{"Steps": steps, "Exit": 0}), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, plan)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	if !egressLifeAwaitWorker(dir, "loss", 5*time.Second) {
		t.Fatal("probe missed TERM barrier")
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	var report egressProbeReport
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		data, _ := os.ReadFile(filepath.Join(dir, "slice-27-report.json"))
		if json.Unmarshal(data, &report) == nil && len(report.Results) == 2 {
			break
		}
	}
	if len(report.Results) != 2 || report.Done || report.Results[0].Reply != "SIGTERM" || report.Results[1].OK || report.Results[1].Error == "" {
		t.Fatalf("no completed post-TERM route evidence: %+v", report)
	}
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatal("worker exited before KILL", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	var exited *exec.ExitError
	if !errors.As(err, &exited) || !strings.Contains(err.Error(), "killed") {
		t.Fatalf("probe did not require KILL: %v", err)
	}
}

func TestEgressLifecyclePublicationMustPrecedeProxyRemoval(t *testing.T) {
	for _, change := range []string{"clean", "missing deletion", "read failed", "wrong mode", "different bytes", "empty copy"} {
		t.Run(change, func(t *testing.T) {
			data := []byte("refusal\n")
			r := egressLifeRequest{method: "DELETE", path: "/containers/proxy", status: 204, logBefore: data, logMode: 0600}
			switch change {
			case "missing deletion":
				r.path = "/containers/other"
			case "read failed":
				r.logErr = os.ErrNotExist
			case "wrong mode":
				r.logMode = 0644
			case "different bytes":
				r.logBefore = []byte("old\n")
			case "empty copy":
				data, r.logBefore = nil, nil
			}
			if err := egressLifePublishedBeforeRemoval([]egressLifeRequest{r}, "proxy", data); (err == nil) != (change == "clean") {
				t.Fatalf("publication assertion: %v", err)
			}
		})
	}
}
