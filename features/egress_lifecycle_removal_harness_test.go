package features

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestEgressLifecycleRemovalFaultOnlyRefusesNamedRequestOnce(t *testing.T) {
	var forwarded atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, _ *http.Request) {
		forwarded.Add(1)
		out.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	fault := &egressLifeRemoveFault{path: "/containers/original", body: `{"message":"synthetic fault"}`}
	recorder := &egressLifeRecorder{transport: http.DefaultTransport.(*http.Transport).Clone(), refuse: fault.refuse}
	defer recorder.transport.CloseIdleConnections()
	for i, tc := range []struct {
		method, path string
		status       int
		forwarded    int32
	}{
		{"GET", "/containers/original", 204, 1},
		{"DELETE", "/containers/other", 204, 2},
		{"DELETE", "/containers/original", 500, 2},
		{"DELETE", "/containers/original", 204, 3},
	} {
		request, err := http.NewRequestWithContext(t.Context(), tc.method, server.URL+tc.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := recorder.RoundTrip(request)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != tc.status || forwarded.Load() != tc.forwarded {
			t.Fatalf("request %d: status %d, forwarded %d, error %v", i, response.StatusCode, forwarded.Load(), err)
		}
		if tc.status == 500 && string(body) != fault.body {
			t.Fatal("injected refusal lost its body")
		}
		observed := recorder.snapshot()[i]
		if observed.status != tc.status || string(observed.response) != string(body) || observed.finished.IsZero() {
			t.Fatal("journal lost the refusal or actual retry outcome")
		}
	}
	if fault.hits != 1 {
		t.Fatal("fault did not fire exactly once")
	}
}

func TestEgressLifecycleRecorderRefusesFabricatedSuccess(t *testing.T) {
	for _, status := range []int{200, 201, 204, 302} {
		recorder := &egressLifeRecorder{refuse: func(*egressLifeRequest) *http.Response {
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("fake"))}
		}}
		request, err := http.NewRequestWithContext(context.Background(), "DELETE", "http://engine/containers/original", nil)
		if err != nil {
			t.Fatal(err)
		}
		if response, err := recorder.RoundTrip(request); err == nil || response != nil {
			t.Fatalf("fabricated status %d became native evidence", status)
		}
	}
}

func TestEgressLifecycleRemovalBoundaryRequiresFaultAndNoLaterDelete(t *testing.T) {
	base := []egressLifeRequest{{method: "DELETE", path: "/containers/worker", status: 204}, {method: "DELETE", path: "/containers/proxy", status: 500}, {method: "GET", path: "/containers/proxy/json", status: 200}}
	if err := egressLifeRemovalBoundary(base, "/containers/proxy"); err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"no fault", "wrong ID", "earlier failure", "later removal"} {
		copy := append([]egressLifeRequest(nil), base...)
		switch change {
		case "no fault":
			copy[1].status = 204
		case "wrong ID":
			copy[1].path = "/containers/other"
		case "earlier failure":
			copy[0].status = 500
		case "later removal":
			copy = append(copy, egressLifeRequest{method: "DELETE", path: "/networks/dependency", status: 204})
		}
		if err := egressLifeRemovalBoundary(copy, "/containers/proxy"); err == nil {
			t.Error("accepted", change)
		}
	}
}
