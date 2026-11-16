package features

import "testing"

func TestEgressInspectionJournalPermitsOnlyRequestedProxyPin(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	w := &world{root: root, dir: t.TempDir()}
	if err := w.approvalCheckout(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.closeDoctor)
	if err := w.approvalNetworkRequest(2, []string{"api.example.com"}); err != nil {
		t.Fatal(err)
	}
	request := doctorRequest{Method: "GET", Path: "/v1.48/images/" + busyboxFixturePin + "/json"}
	if !w.egressProxyInspection(request, true) || w.egressProxyInspection(request, false) {
		t.Fatal("proxy inspection permission ignored")
	}
	for _, bad := range []doctorRequest{
		{Method: "POST", Path: request.Path},
		{Method: "GET", Path: request.Path, Query: "platform=linux/amd64"},
		{Method: "GET", Path: "/images/" + egressProxyConfigID + "/json"},
		{Method: "GET", Path: "/images/other/json"},
		{Method: "POST", Path: "/images/create"},
	} {
		if w.egressProxyInspection(bad, true) {
			t.Fatalf("forbidden image request accepted: %+v", bad)
		}
	}
	if err := w.approvalNetworkRequest(2, nil); err != nil {
		t.Fatal(err)
	}
	if w.egressProxyInspection(request, true) {
		t.Fatal("offline inspection admitted a proxy image from an earlier allowlist fixture")
	}
}
