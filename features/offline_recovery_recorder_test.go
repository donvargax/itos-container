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
// slices need removal faults, while only recovery permits owned-worker signals.
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

func TestOfflineRecorderSignalsOnlyOwnedRecoveryWorker(t *testing.T) {
	w, mutations := recoveryRecorderWorld(t)
	a := w.offline.api
	owned := a.created[0]
	for _, recovery := range []bool{false, true} {
		a.recovery = recovery
		for _, target := range []string{owned, strings.Repeat("f", 64), "-1", "0", strconv.Itoa(os.Getpid())} {
			for _, signal := range []string{"SIGTERM", "SIGKILL"} {
				response := httptest.NewRecorder()
				path := "/containers/" + target + "/kill"
				a.serve(response, httptest.NewRequest("POST", "http://engine"+path+"?signal="+signal, nil))
				want := http.StatusForbidden
				if recovery && target == owned {
					want = http.StatusNoContent
				}
				if response.Code != want {
					t.Fatalf("recovery=%t target=%s signal=%s: got %d, want %d", recovery, target, signal, response.Code, want)
				}
				select {
				case got := <-mutations:
					if want != http.StatusNoContent || got != "POST "+path+"?signal="+signal {
						t.Fatal("signal reached outside the owned recovery worker", got)
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
