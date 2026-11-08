package features

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type anchorOracleTransport func(*http.Request) (*http.Response, error)

func (f anchorOracleTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func anchorJournalFixture(t *testing.T) (*world, []offlineRequest) {
	t.Helper()
	w := offlineStartupWorld(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("journal assertion attempted a new engine operation")
	}))
	a := w.offline.api
	id := strings.Repeat("a", 64)
	w.recovery = &recoveryNative{worker: id}
	v := a.volumes["itos-container-anchor-"+w.session.token]
	var err error
	v.observed, err = a.anchorInspect(v)
	if err != nil {
		t.Fatal(err)
	}
	a.created = []string{id}
	owned := w.sessionExpectedRecord("owned", id)
	intent := approvalClone(owned)
	intent["stage"], intent["container_id"] = "reserved", ""
	object(intent["anchor"])["stage"], object(intent["anchor"])["created"] = "intent", ""
	absent := approvalClone(owned)
	absent["stage"] = "absent"
	record := func(value map[string]any) sessionRecord {
		return sessionRecord{Bytes: string(approvalJSON(value)), Mode: 0600, UID: w.approval.uid, Inode: "90"}
	}
	info := offlineRequest{Method: "GET", Path: "/info", Status: 200, Observation: map[string]any{"ID": w.doctor.engineID}}
	worker := offlineStartupInspection(w, id)
	return w, []offlineRequest{
		info,
		{Method: "POST", Path: "/volumes/create", Status: 201, Body: map[string]any{"Name": v.name, "Driver": "local", "Labels": v.labels, "DriverOpts": v.options}, Record: record(intent), Observation: approvalClone(v.observed)},
		info,
		{Method: "GET", Path: "/containers/" + id + "/json", Status: 200, Observation: worker},
		{Method: "DELETE", Path: "/containers/" + id, Status: 204, Observation: worker, Record: record(owned)},
		{Method: "GET", Path: "/containers/" + id + "/json", Status: 404},
		info,
		{Method: "GET", Path: "/volumes/" + v.name, Status: 200, Observation: approvalClone(v.observed)},
		{Method: "DELETE", Path: "/volumes/" + v.name, Status: 204, Observation: approvalClone(v.observed), Record: record(absent)},
		{Method: "GET", Path: "/volumes/" + v.name, Status: 404},
	}
}

func TestAnchorJournalRequiresFreshOwnedRelease(t *testing.T) {
	w, entries := anchorJournalFixture(t)
	if err := w.lifecycleRevalidated(entries); err != nil {
		t.Fatalf("owned worker then anchor release rejected: %v", err)
	}
	for _, fault := range []string{"create-daemon", "create-record", "create-source", "release-daemon", "changed-daemon", "missing-inspect", "changed-volume", "changed-created", "missing-record", "unsafe-record", "foreign-record", "foreign-target", "force", "missing-worker-absence", "name-only-absence", "worker-reappears"} {
		t.Run(fault, func(t *testing.T) {
			w, entries := anchorJournalFixture(t)
			switch fault {
			case "create-daemon":
				entries[0].Status = 503
			case "create-record":
				entries[1].Record.Bytes = "{}"
			case "create-source":
				entries[1].Body = approvalClone(entries[1].Body)
				object(entries[1].Body["DriverOpts"])["device"] = "/replacement"
			case "release-daemon":
				entries[6].Status = 503
			case "changed-daemon":
				entries[6].Observation = map[string]any{"ID": "other"}
			case "missing-inspect":
				entries[7].Status = 503
			case "changed-volume":
				object(entries[8].Observation["Options"])["device"] = "/replacement"
			case "changed-created":
				entries[7].Observation["CreatedAt"] = "replacement"
			case "missing-record":
				entries[8].Record.Bytes = "{}"
			case "unsafe-record":
				entries[8].Record.Mode = 0666
			case "foreign-record":
				r, _ := sessionDecodeRecord(entries[8].Record.Bytes)
				r["container_id"] = strings.Repeat("f", 64)
				entries[8].Record.Bytes = string(approvalJSON(r))
			case "foreign-target":
				entries[8].Path = "/volumes/foreign"
			case "force":
				entries[8].Query = "force=true"
			case "missing-worker-absence":
				entries[5].Status = 503
			case "name-only-absence":
				entries[5].Path = "/containers/itos-container-" + w.session.token + "/json"
			case "worker-reappears":
				entries = append(entries[:8], append([]offlineRequest{entries[3]}, entries[8:]...)...)
			}
			if err := w.lifecycleRevalidated(entries); err == nil {
				t.Fatal("unsafe anchor mutation passed")
			}
		})
	}
}

func TestAnchorBoundaryUsesCapturedEvidenceAfterCleanup(t *testing.T) {
	w, _ := anchorJournalFixture(t)
	a := w.offline.api
	worker := offlineStartupInspection(w, w.recovery.worker)
	// The disposable engine now reports the volume absent. The assertion must
	// judge the observation captured at launch, without new transport calls.
	w.doctor.real.client.Transport = anchorOracleTransport(func(*http.Request) (*http.Response, error) {
		t.Error("completed-run assertion re-inspected a deleted volume")
		return &http.Response{StatusCode: 404, Body: http.NoBody}, nil
	})
	if !a.safeObserved(worker) {
		t.Fatal("verified launch boundary rejected after successful cleanup")
	}
	object(object(worker["HostConfig"])["Mounts"].([]any)[0])["Source"] = "foreign"
	if a.safeObserved(worker) {
		t.Fatal("unrelated volume accepted as the captured anchor")
	}
}

func TestAnchorRetainedRecordKeepsBothOwnedStages(t *testing.T) {
	w, entries := anchorJournalFixture(t)
	r := &isolationRun{expected: approvalClone(w.sessionExpectedRecord("absent", w.recovery.worker)), record: entries[4].Record}
	if err := isolationRecord(r, "owned"); err != nil {
		t.Fatalf("retained worker and anchor rejected: %v", err)
	}
	actual, _ := sessionDecodeRecord(r.record.Bytes)
	object(actual["anchor"])["stage"] = "absent"
	r.record.Bytes = string(approvalJSON(actual))
	if isolationRecord(r, "owned") == nil {
		t.Fatal("retained worker with a completed anchor accepted")
	}
}

func TestAnchorCompletionRequiresReleaseAndAbsence(t *testing.T) {
	w, entries := anchorJournalFixture(t)
	if err := w.anchorReleased(entries); err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"no-delete", "failed-delete", "no-confirmation", "still-present", "no-allocation"} {
		t.Run(fault, func(t *testing.T) {
			w, entries := anchorJournalFixture(t)
			switch fault {
			case "no-delete":
				entries[8].Method = "GET"
			case "failed-delete":
				entries[8].Status = 503
			case "no-confirmation":
				entries = entries[:9]
			case "still-present":
				entries[9].Status = 200
			case "no-allocation":
				entries = entries[2:]
			}
			if w.anchorReleased(entries) == nil {
				t.Fatal("incomplete anchor release accepted")
			}
		})
	}
}

func TestAnchorRecoveryOraclesDistinguishResourceTypes(t *testing.T) {
	w, entries := anchorJournalFixture(t)
	id := w.recovery.worker
	w.lifecycle = &lifecycleNative{}
	w.offline.api.requests = append(entries[:2:2], append([]offlineRequest{
		{Method: "POST", Path: "/containers/create", Status: 201},
		{Method: "POST", Path: "/containers/" + id + "/start", Status: 204},
	}, entries[2:]...)...)
	for _, check := range []func() error{w.lifecycleOneLaunch, w.recoveryOneLaunch} {
		if err := check(); err != nil {
			t.Fatal("owned anchor was confused with a worker removal", err)
		}
		w.offline.api.requests = append(w.offline.api.requests, offlineRequest{Method: "DELETE", Path: "/images/foreign", Status: 204})
		if check() == nil {
			t.Fatal("unrelated deletion accepted")
		}
		w.offline.api.requests = w.offline.api.requests[:len(w.offline.api.requests)-1]
	}
}

func TestAnchorOverlapObservesTheOwnedVolumeBoundary(t *testing.T) {
	boundary, run := mountOracleFixture()
	request := run.requests[0]
	worker := request.Observation
	w := offlineRecorderWorld(t, http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		var value map[string]any
		switch r.URL.Path {
		case "/containers/" + run.id + "/json":
			value = worker
		case "/volumes/" + request.Anchor["Name"].(string):
			value = request.Anchor
		default:
			t.Error("unexpected observation", r.URL.Path)
		}
		_ = json.NewEncoder(out).Encode(value)
	}))
	w.approval = boundary.w.approval
	w.session.token = strings.Repeat("a", 64)
	worker["Name"], worker["Image"] = "/itos-container-"+w.session.token, w.doctor.imageID
	worker["State"] = map[string]any{"Running": true}
	name := request.Anchor["Name"].(string)
	w.offline.api.volumes = map[string]*anchorVolume{name: {name: name, labels: object(request.Anchor["Labels"]), options: object(request.Anchor["Options"]), observed: approvalClone(request.Anchor)}}
	intent := filepath.Join(t.TempDir(), "intent.json")
	if err := os.WriteFile(intent, []byte(request.Record.Bytes), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := sessionReadRecord(intent)
	if err != nil {
		t.Fatal(err)
	}
	w.approval.uid = info.UID
	r := &overlapRun{w: w, worker: run.id, id: run.sessionID, intent: intent, marker: "heartbeat"}
	if err := os.WriteFile(filepath.Join(w.doctor.checkout, r.marker), []byte("1"), 0600); err != nil {
		t.Fatal(err)
	}
	if !r.active() {
		t.Fatal("running worker with its exact anchor and heartbeat was not active")
	}
	object(worker["Mounts"].([]any)[0])["Name"] = "foreign"
	if r.active() {
		t.Fatal("heartbeat authorized a substituted mount")
	}
}

func TestAnchorRecorderCapturesProtectedReleaseRecord(t *testing.T) {
	id := strings.Repeat("a", 64)
	deletes := 0
	w := offlineStartupWorld(t, http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/containers/"+id+"/json" {
			out.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method != "DELETE" || !strings.HasPrefix(r.URL.Path, "/volumes/") {
			t.Error("unexpected engine operation", r.Method, r.URL.Path)
		}
		deletes++
		out.WriteHeader(http.StatusNoContent)
	}))
	a := w.offline.api
	a.created = []string{id}
	want := w.sessionExpectedRecord("absent", id)
	object(want["anchor"])["stage"] = "owned"
	if err := os.MkdirAll(filepath.Dir(w.sessionRecordPath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w.sessionRecordPath(), approvalJSON(want), 0600); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	path := "/volumes/itos-container-anchor-" + w.session.token
	a.serve(response, httptest.NewRequest("DELETE", "http://engine"+path, nil))
	if response.Code != 204 || deletes != 1 || len(a.requests) != 1 {
		t.Fatal("owned release was not forwarded exactly once", response.Code, deletes)
	}
	e := a.requests[0]
	if !recoveryJSONEqual(e.Record.Bytes, want) || e.Record.Mode != 0600 || e.Record.Inode == "" || e.Observation["Name"] != "itos-container-anchor-"+w.session.token {
		t.Fatal("release lost its protected record or independent volume observation")
	}
}

func TestAnchorInventoryExcludesOnlyAnOwnedRetainedPair(t *testing.T) {
	id := strings.Repeat("a", 64)
	var w *world
	present := true
	w = offlineStartupWorld(t, http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/containers/"+id+"/json" {
			t.Error("unexpected independent observation", r.Method, r.URL.Path)
		}
		if !present {
			out.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(out).Encode(offlineStartupInspection(w, id))
	}))
	a := w.offline.api
	a.created = []string{id}
	name := "itos-container-anchor-" + w.session.token
	resources := map[string]string{"/volumes": `{"Name":"unrelated"}` + "\n" + `{"Name":"` + name + `"}`, "/images/json?all=1": "image"}
	filtered, err := w.recoveryWithoutRetainedAnchor(resources)
	if err != nil || filtered["/volumes"] != `{"Name":"unrelated"}` || filtered["/images/json?all=1"] != "image" {
		t.Fatal("retained pair exclusion changed unrelated resources", filtered, err)
	}
	present = false
	if _, err = w.recoveryWithoutRetainedAnchor(resources); err == nil {
		t.Fatal("leaked anchor was excluded after worker removal")
	}
}
