package features

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Only a disposable HTTP server is used here, never a native engine. Model the
// engine's inspected representation independently of the product create body.
func offlineStartupInspection(w *world, id string) map[string]any {
	return map[string]any{
		"Id": id, "Name": "/itos-container-" + w.session.token, "Image": w.doctor.imageID,
		"Config": map[string]any{"User": w.approval.uid + ":" + w.approval.gid, "Labels": w.sessionLabels()},
		"State":  map[string]any{"Status": "created", "Running": false, "Error": ""},
		"HostConfig": map[string]any{
			"NetworkMode": "none", "ReadonlyRootfs": true,
			"CapDrop": []any{"ALL"}, "SecurityOpt": []any{"no-new-privileges=true"},
			"Mounts": []any{map[string]any{"Type": "bind", "Source": w.doctor.checkout, "Target": "/workspace",
				"BindOptions": map[string]any{"NonRecursive": true, "Propagation": "rprivate"}}},
			"Tmpfs": map[string]any{"/tmp": "rw,nosuid,nodev,mode=1777"},
		},
	}
}

func TestOfflineRecorderStartsNormalizedInspections(t *testing.T) {
	for _, options := range [][]any{{"no-new-privileges"}, {"no-new-privileges=true"}, nil,
		{"no-new-privileges=false"}, {"no-new-privileges=true", "seccomp=unconfined"}} {
		t.Run(strings.TrimSpace(string(approvalJSON(options))), func(t *testing.T) {
			id := strings.Repeat("a", 64)
			var w *world
			forwarded := make(chan struct{}, 1)
			w = offlineRecorderWorld(t, http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" && r.URL.Path == "/containers/"+id+"/json" {
					observed := offlineStartupInspection(w, id)
					object(observed["HostConfig"])["SecurityOpt"] = options
					_ = json.NewEncoder(out).Encode(observed)
					return
				}
				forwarded <- struct{}{}
				out.WriteHeader(http.StatusNoContent)
			}))
			a := w.offline.api
			a.created = []string{id}
			response := httptest.NewRecorder()
			a.serve(response, httptest.NewRequest("POST", "http://engine/containers/"+id+"/start", nil))
			want := http.StatusForbidden
			if len(options) == 1 && options[0] != "no-new-privileges=false" {
				want = http.StatusNoContent
			}
			if response.Code != want || (len(forwarded) == 1) != (want == http.StatusNoContent) {
				t.Fatalf("inspected security options %v: HTTP %d, forwarded=%d, want %d", options, response.Code, len(forwarded), want)
			}
		})
	}
}

func TestSocketStartupFailureDistinguishesRecorderRefusal(t *testing.T) {
	w := offlineRecorderWorld(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("refused request reached the engine")
	}))
	w.offline.api.serve(httptest.NewRecorder(), httptest.NewRequest("POST", "http://engine/containers/foreign/start", nil))
	w.offline.exit = 75
	s := &socketWorld{w: w, runErr: errors.New("unreached barrier")}
	err := s.completeReports()
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") || !strings.Contains(err.Error(), "recorder refused unsafe request") {
		t.Fatalf("recorder refusal lost its origin: %v", err)
	}
	if strings.Contains(err.Error(), "engine response:") {
		t.Fatal("recorder refusal was attributed to the engine")
	}
}

func TestSocketStartupFailureIncludesControllerAndEngineDiagnostics(t *testing.T) {
	id := strings.Repeat("a", 64)
	var w *world
	w = offlineRecorderWorld(t, http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			observed := offlineStartupInspection(w, id)
			object(observed["HostConfig"])["SecurityOpt"] = []any{"no-new-privileges"}
			object(observed["State"])["Error"] = "synthetic runtime start error"
			object(observed["Config"])["Env"] = []any{"UNRELATED=do-not-dump-inspection"}
			_ = json.NewEncoder(out).Encode(observed)
			return
		}
		out.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(out).Encode(map[string]any{"message": "synthetic executable unavailable"})
	}))
	a := w.offline.api
	a.created = []string{id}
	a.serve(httptest.NewRecorder(), httptest.NewRequest("POST", "http://engine/containers/"+id+"/start", nil))
	w.offline.exit, w.offline.stderr = 75, []byte("run.execution-unavailable: owned run could not be completed")
	s := &socketWorld{w: w, runErr: errors.New("probe exited before barrier/live")}
	err := s.completeReports()
	if err == nil {
		t.Fatal("early refusal passed the workload completeness check")
	}
	for _, want := range []string{"not product red evidence", "product exit 75", "barrier/live", "run.execution-unavailable",
		"POST /containers/" + id + "/start", "HTTP 500", "synthetic executable unavailable", "synthetic runtime start error"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("startup failure omitted %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "do-not-dump-inspection") {
		t.Fatal("startup diagnostics dumped unrelated container configuration")
	}
}
