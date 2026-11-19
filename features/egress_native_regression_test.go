package features

import (
	"encoding/json"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestEgressProbeNoncesSeparateWorkerAndControls(t *testing.T) {
	iso := &egressIsolation{s: &isolationWorld{config: isolationConfig{Nonce: "09d8db7ebcb481dd1af78aab"}}}
	var worker, controls []string
	for _, phase := range []string{"first", "running"} {
		for _, kind := range []string{"quic", "direct", "forward-tcp", "forward-udp", "forward-tls", "forward-get"} {
			worker = append(worker, iso.token(kind, phase, ""))
			for _, suffix := range []string{"b", "a"} {
				controls = append(controls, iso.token(kind, phase, suffix))
			}
		}
	}
	for _, attempt := range worker {
		for _, control := range controls {
			// The receipt search also reads binary QUIC flights containing
			// the nonce, so equality alone cannot separate the traffic.
			if strings.Contains(control, attempt) || strings.Contains(attempt, control) {
				t.Errorf("worker nonce %q overlaps control nonce %q", attempt, control)
			}
		}
	}
}

func TestEgressApprovalSeedUsesCanonicalOrder(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native egress approval fixtures use Linux checkout identities")
	}
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	w := &world{root: root, dir: t.TempDir()}
	if err = w.approvalCheckout(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.closeDoctor)
	if err = w.localEgressFixture(); err != nil {
		t.Fatal(err)
	}
	names := []string{"public.example", "private.example"}
	w.approval.policyVersion, w.approval.allow = 2, names
	if err := w.approvalNetworkFile(names); err != nil {
		t.Fatal(err)
	}
	if err := w.approvalSeed("1", false); err != nil {
		t.Fatal(err)
	}
	a := w.approval
	if err = w.runDoctor(w.env(), "doctor", "--json"); err != nil {
		t.Fatal(err)
	}
	var report struct{ Approval struct{ Status string } }
	if err = json.Unmarshal([]byte(w.stdout), &report); err != nil {
		t.Fatal(err)
	}
	if w.exit != 0 || report.Approval.Status != "matching" {
		t.Errorf("doctor refused the synthetic matching approval: %s", w.report())
	}
	got := object(object(a.settings["policy"])["network"])["allow"].([]string)
	if !slices.Equal(got, []string{"private.example", "public.example"}) {
		t.Errorf("approval allowlist = %v, want canonical name order", got)
	}
	if !slices.Equal(names, []string{"public.example", "private.example"}) {
		t.Error("canonical approval construction changed the requested list")
	}
}
