package features

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
)

// Exercise the merged recorder through HTTP without a native engine. Both
// slices need removal faults; every run needs identity-checked owned teardown.
func TestOfflineRecorderPreservesBothRemovalFaults(t *testing.T) {
	for _, mode := range []string{"normal", "recovery", "isolation"} {
		t.Run(mode, func(t *testing.T) {
			w, mutations := recoveryRecorderWorld(t)
			a := w.offline.api
			id := a.created[0]
			a.recovery, a.failRemove = mode == "recovery", mode == "recovery"
			if mode == "isolation" {
				w.offline.isolation = &isolationWorld{removeFault: true}
				a.requests = []offlineRequest{{Method: "POST", Path: "/containers/" + id + "/wait", Status: 200, Observation: map[string]any{"StatusCode": float64(7)}}}
			}
			response := httptest.NewRecorder()
			a.serve(response, httptest.NewRequest("DELETE", "http://engine/containers/"+id, nil))
			want := http.StatusServiceUnavailable
			if mode == "normal" {
				want = http.StatusNoContent
			}
			if response.Code != want || a.requests[len(a.requests)-1].Status != want {
				t.Fatalf("removal response/journal = %d/%d, want %d", response.Code, a.requests[len(a.requests)-1].Status, want)
			}
			if (len(mutations) == 1) != (mode == "normal") {
				t.Fatal("removal fault failed to intercept mutation or affected the normal control")
			}
			marker := map[string]string{"recovery": "slice-11 transient removal fault", "isolation": "SLICE_14_SYNTHETIC_REMOVE_BODY"}[mode]
			if marker != "" && !strings.Contains(response.Body.String(), marker) {
				t.Fatal("removal lost its slice-specific fault diagnostic")
			}
		})
	}
}

func TestOfflineRecorderSignalsOnlyOwnedWorker(t *testing.T) {
	w, mutations := recoveryRecorderWorld(t)
	a := w.offline.api
	owned := a.created[0]
	for _, recovery := range []bool{false, true} {
		a.recovery = recovery
		for _, target := range []string{owned, strings.Repeat("f", 64), "-1", "0", strconv.Itoa(os.Getpid())} {
			for _, signal := range []string{"SIGTERM", "SIGKILL", "SIGSTOP", "SIGKILL&signal=SIGTERM", "SIGKILL&extra=1"} {
				response := httptest.NewRecorder()
				path := "/containers/" + target + "/kill"
				a.serve(response, httptest.NewRequest("POST", "http://engine"+path+"?signal="+signal, nil))
				want := http.StatusForbidden
				if target == owned && (signal == "SIGTERM" || signal == "SIGKILL") {
					want = http.StatusNoContent
				}
				if response.Code != want {
					t.Fatalf("recovery=%t target=%s signal=%s: got %d, want %d", recovery, target, signal, response.Code, want)
				}
				select {
				case got := <-mutations:
					if want != http.StatusNoContent || got != "POST "+path+"?signal="+signal {
						t.Fatal("signal reached outside the owned worker contract", got)
					}
				default:
					if want == http.StatusNoContent {
						t.Fatal("owned signal was not forwarded")
					}
				}
			}
		}
	}
}

func TestOfflineRecorderStopRequiresOwnedIdentityAndBoundedGrace(t *testing.T) {
	w, mutations := recoveryRecorderWorld(t)
	a := w.offline.api
	owned := a.created[0]
	for _, target := range []string{owned, strings.Repeat("f", 64)} {
		for _, query := range []string{"signal=SIGTERM&t=10", "signal=SIGKILL&t=10", "signal=SIGTERM&t=-1",
			"signal=SIGTERM&t=10&t=-1", "signal=SIGTERM&signal=SIGKILL&t=10", "signal=SIGTERM&t=10&extra=1"} {
			response := httptest.NewRecorder()
			path := "/containers/" + target + "/stop?" + query
			a.serve(response, httptest.NewRequest("POST", "http://engine"+path, nil))
			want := http.StatusForbidden
			if target == owned && query == "signal=SIGTERM&t=10" {
				want = http.StatusNoContent
			}
			if response.Code != want {
				t.Fatalf("stop %s: got %d, want %d", path, response.Code, want)
			}
			select {
			case got := <-mutations:
				if want != http.StatusNoContent || got != "POST "+path {
					t.Fatal("stop escaped the owned teardown contract", got)
				}
			default:
				if want == http.StatusNoContent {
					t.Fatal("owned stop was not forwarded")
				}
			}
		}
	}
	// Membership in the created-ID list alone must never authorize teardown.
	a.identities[owned] = map[string]any{"Name": "/changed", "Labels": w.sessionLabels()}
	for _, operation := range []string{"kill?signal=SIGTERM", "stop?signal=SIGTERM&t=10", "?force=true&v=false"} {
		method, path := "POST", "/containers/"+owned+"/"+operation
		if strings.HasPrefix(operation, "?") {
			method, path = "DELETE", "/containers/"+owned+operation
		}
		response := httptest.NewRecorder()
		a.serve(response, httptest.NewRequest(method, "http://engine"+path, nil))
		if response.Code != http.StatusForbidden || len(mutations) != 0 {
			t.Fatal("changed ownership reached teardown", path, response.Code)
		}
	}
}

func recoveryRecorderWorld(t *testing.T) (*world, <-chan string) {
	t.Helper()
	var w *world
	id := strings.Repeat("a", 64)
	mutations := make(chan string, 1)
	w = offlineRecorderWorld(t, http.HandlerFunc(func(out http.ResponseWriter, request *http.Request) {
		if request.Method == "GET" && request.URL.Path == "/containers/"+id+"/json" {
			_ = json.NewEncoder(out).Encode(map[string]any{"Id": id, "Name": "/itos-container-" + w.session.token, "Image": w.doctor.imageID, "Config": map[string]any{"Labels": w.sessionLabels()}, "State": map[string]any{"Running": true}})
			return
		}
		mutations <- request.Method + " " + request.URL.RequestURI()
		out.WriteHeader(http.StatusNoContent)
	}))
	w.offline.api.created = []string{id}
	return w, mutations
}
