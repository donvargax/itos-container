package features

import "testing"

func TestEgressInspectionJournalPermitsOnlyRequestedProxyPin(t *testing.T) {
	// This journal predicate needs only the selected policy and fixture marker,
	// not a Linux checkout identity or a Unix HTTP listener.
	w := &world{approval: &approvalWorld{allow: []string{"api.example.com"}},
		doctor: &doctorWorld{engine: &doctorAPI{egress: &localEgressAPI{}}}}
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
	w.approval.allow = nil
	if w.egressProxyInspection(request, true) {
		t.Fatal("offline inspection admitted a proxy image from an earlier allowlist fixture")
	}
}
