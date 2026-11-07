package features

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

func TestLifecycleNativeStepsDefined(t *testing.T) {
	// Parse and register only: no Before/Given/When or native setup executes.
	// LIFECYCLE-15 deliberately remains slice-15's unimplemented scenario.
	definitions := &lifecycleDefinitions{t: t}
	(&world{}).lifecycleNativeSteps(definitions)
	(&world{}).offlineIsolationSteps(definitions)
	suite := godog.TestSuite{
		Name:    "slice-12-native-definitions",
		Options: &godog.Options{Paths: []string{"lifecycle.feature"}, Tags: "@slice-12 && @engine", Output: io.Discard},
	}
	features, err := suite.RetrieveFeatures()
	if err != nil {
		t.Fatal(err)
	}
	cases := 0
	for _, feature := range features {
		for _, scenario := range feature.Pickles {
			cases++
			for _, step := range scenario.Steps {
				matches := 0
				for _, expression := range definitions.patterns {
					if expression.MatchString(step.Text) {
						matches++
					}
				}
				if matches != 1 {
					t.Errorf("%q matches %d lifecycle definitions, expected exactly one", step.Text, matches)
				}
			}
		}
	}
	if cases != 18 {
		t.Fatalf("native outline rows = %d, expected 18", cases)
	}
}

func TestLifecycleProbeCompiles(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	// Compilation only. The trusted probe must never execute on the host.
	if err := recoveryBuildHelper(root, filepath.Join(t.TempDir(), "slice-12-probe"), lifecycleProbeSource); err != nil {
		t.Fatal(err)
	}
}

type lifecycleDefinitions struct {
	t        *testing.T
	patterns []*regexp.Regexp
}

func (d *lifecycleDefinitions) Step(expression, function any) {
	d.t.Helper()
	pattern := regexp.MustCompile(expression.(string))
	fn := reflect.TypeOf(function)
	if fn.Kind() != reflect.Func || fn.NumIn() != pattern.NumSubexp() {
		d.t.Fatalf("step %s has an incompatible definition", expression)
	}
	d.patterns = append(d.patterns, pattern)
}

func TestLifecycleRecorderSignalsRemainOwned(t *testing.T) {
	w, mutations := recoveryRecorderWorld(t)
	a := w.offline.api
	a.recovery, a.lifecycle = true, newLifecycleFaults("handle", "", 0)
	owned := a.created[0]
	for _, target := range []string{owned, strings.Repeat("f", 64), "itos-container-" + w.session.token, "-1", "0"} {
		response := httptest.NewRecorder()
		a.serve(response, httptest.NewRequest("POST", "http://engine/containers/"+target+"/kill?signal=SIGINT", nil))
		want := http.StatusForbidden
		if target == owned {
			want = http.StatusNoContent
		}
		if response.Code != want {
			t.Fatalf("signal target %q returned %d, expected %d", target, response.Code, want)
		}
		select {
		case got := <-mutations:
			if want != http.StatusNoContent || got != "POST /containers/"+owned+"/kill?signal=SIGINT" {
				t.Fatalf("unsafe signal escaped recorder: %s", got)
			}
		default:
			if want == http.StatusNoContent {
				t.Fatal("owned original signal was never delivered")
			}
		}
	}
}

func TestLifecycleRecorderRelaysWaitAfterOwnedRemoval(t *testing.T) {
	id := strings.Repeat("a", 64)
	w := offlineRecorderWorld(t, http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		// Native absence is an observation, not evidence of substituted identity.
		if r.URL.Path != "/containers/"+id+"/wait" && r.URL.Path != "/containers/"+id+"/json" {
			t.Error("unexpected request reached native transport", r.Method, r.URL.Path)
		}
		http.Error(out, `{"message":"already removed"}`, http.StatusNotFound)
	}))
	a := w.offline.api
	a.created, a.recovery, a.lifecycle = []string{id}, true, newLifecycleFaults("streams", "", 0)
	for _, target := range []string{id, strings.Repeat("f", 64), "itos-container-" + w.session.token} {
		response := httptest.NewRecorder()
		a.serve(response, httptest.NewRequest("POST", "http://engine/containers/"+target+"/wait?condition=not-running", nil))
		want := http.StatusForbidden
		if target == id {
			want = http.StatusNotFound
		}
		if response.Code != want || a.problem != nil {
			t.Fatalf("late read-only wait: status %d, want %d, recorder failure %v", response.Code, want, a.problem)
		}
	}
	// The same absent native observation still cannot authorize a mutation.
	for _, methodPath := range []struct{ method, path string }{
		{"POST", "/kill?signal=SIGTERM"}, {"POST", "/start"}, {"DELETE", ""},
	} {
		response := httptest.NewRecorder()
		a.serve(response, httptest.NewRequest(methodPath.method, "http://engine/containers/"+id+methodPath.path, nil))
		if response.Code != http.StatusForbidden || a.problem == nil {
			t.Fatal("absent native identity authorized a mutation", methodPath, response.Code)
		}
	}
}

func TestLifecycleRecorderDelaysOnlySignalAcknowledgement(t *testing.T) {
	w, mutations := recoveryRecorderWorld(t)
	a := w.offline.api
	// Fixture delay alone is shortened for this harness test. Product budgets
	// and the native scenario's zero/six-second examples are never overridden.
	a.recovery, a.lifecycle = true, newLifecycleFaults("ignore", "", 50*time.Millisecond)
	done := make(chan struct{})
	response := httptest.NewRecorder()
	go func() {
		defer close(done)
		a.serve(response, httptest.NewRequest("POST", "http://engine/containers/"+a.created[0]+"/kill?signal=SIGTERM", nil))
	}()
	select {
	case <-mutations:
	case <-time.After(time.Second):
		t.Fatal("native delivery was held instead of its acknowledgement")
	}
	select {
	case <-done:
		t.Fatal("acknowledgement was not held after delivery")
	default:
	}
	<-done
	if response.Code != 204 || len(a.requests) != 1 || a.requests[0].Replied.Sub(a.requests[0].At) < 50*time.Millisecond {
		t.Fatal("delayed acknowledgement was not recorded with its original request time")
	}
}

func TestLifecycleRemovalFaultStaysUnavailableAfterBarrierRelease(t *testing.T) {
	f := newLifecycleFaults("cleanup-signals", "", 0)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	r := httptest.NewRequest(http.MethodDelete, "http://engine/containers/owned", nil).WithContext(ctx)
	e := offlineRequest{Method: http.MethodDelete, Path: "/containers/owned"}
	response := httptest.NewRecorder()
	done := make(chan bool, 1)
	go func() { done <- f.before(response, r, e) }()
	select {
	case <-f.ready:
	case <-ctx.Done():
		t.Fatal("removal never reached the signal overlap barrier")
	}
	select {
	case <-done:
		t.Fatal("removal completed before signals could overlap it")
	default:
	}
	f.releaseBarrier()
	select {
	case refused := <-done:
		if !refused || response.Code != http.StatusServiceUnavailable || ctx.Err() != nil {
			t.Fatal("released removal did not report unavailability before call expiry")
		}
	case <-ctx.Done():
		t.Fatal("removal waited for its call timeout after the overlap ended")
	}
	// Repeated attempts cannot reach the engine merely because the barrier
	// ended. Only explicit restoration for the fresh stop permits forwarding.
	response = httptest.NewRecorder()
	if !f.before(response, r, e) || response.Code != http.StatusServiceUnavailable {
		t.Fatal("barrier release accidentally restored engine removal")
	}
	f.restore()
	if f.before(httptest.NewRecorder(), r, e) {
		t.Fatal("fresh recovery could not reach removal after restoration")
	}
}

func TestLifecycleRecorderStreamsWaitAndDisconnectsOnlyController(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		t.Run(map[bool]string{false: "healthy", true: "disconnect"}[disconnect], func(t *testing.T) {
			id := strings.Repeat("a", 64)
			release := make(chan struct{})
			var w *world
			w = offlineRecorderWorld(t, http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/json") {
					_ = json.NewEncoder(out).Encode(map[string]any{"Id": id, "Name": "/itos-container-" + w.session.token, "Image": w.doctor.imageID, "Config": map[string]any{"Labels": w.sessionLabels()}, "State": map[string]any{"Running": true}})
					return
				}
				out.WriteHeader(200)
				out.(http.Flusher).Flush()
				select {
				case <-release:
					_, _ = io.WriteString(out, `{"StatusCode":17}`)
				case <-r.Context().Done():
				}
			}))
			a := w.offline.api
			a.created, a.recovery, a.lifecycle = []string{id}, true, newLifecycleFaults("disconnect", "", 0)
			transport := doctorTransport(strings.TrimPrefix(w.doctor.env["DOCKER_HOST"], "unix://"))
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
			r, err := http.NewRequestWithContext(context.Background(), "POST", "http://engine/containers/"+id+"/wait?condition=not-running", nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.Do(r)
			if err != nil {
				t.Fatalf("wait acknowledgement was buffered behind workload lifetime: %v", err)
			}
			if disconnect {
				a.lifecycle.disconnect()
			} else {
				close(release)
			}
			data, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if !disconnect && (readErr != nil || string(data) != `{"StatusCode":17}`) {
				t.Fatalf("healthy wait lost its observed exit: %q %v", data, readErr)
			}
			if disconnect && readErr == nil {
				t.Fatal("lost transport became a complete wait response")
			}
			// This uses the independent native-fixture transport after the fault;
			// it would fail if disconnect had stopped the observer or its server.
			if _, err := a.inspect(id); err != nil {
				t.Fatalf("controller disconnect damaged independent observation: %v", err)
			}
			a.mu.Lock()
			defer a.mu.Unlock()
			entry := a.requests[0]
			if disconnect && (!entry.Cancelled || entry.Observation["StatusCode"] != nil) {
				t.Fatal("cancelled wait fabricated an authoritative exit")
			}
			if !disconnect && (entry.Cancelled || entry.Observation["StatusCode"] != float64(17) || entry.Replied.IsZero()) {
				t.Fatal("healthy wait omitted its authoritative observed exit")
			}
		})
	}
}

func TestLifecycleCrashBarriersObserveAcceptedTransition(t *testing.T) {
	for _, item := range []struct {
		barrier, method, path string
		status                int
	}{
		{"after accepted create before reply", "POST", "/containers/create", 201},
		{"after accepted start before reply", "POST", "/containers/" + strings.Repeat("a", 64) + "/start", 204},
		{"after removal before durable absence update", "GET", "/containers/" + strings.Repeat("a", 64) + "/json", 404},
	} {
		t.Run(item.barrier, func(t *testing.T) {
			f := newLifecycleFaults("crash", item.barrier, 0)
			if item.status == 404 {
				f.response(httptest.NewRequest("DELETE", "http://engine/containers/id", nil), offlineRequest{Method: "DELETE"}, 204)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := httptest.NewRequest(item.method, "http://engine"+item.path, nil).WithContext(ctx)
			done := make(chan struct{})
			go func() {
				defer close(done)
				f.response(r, offlineRequest{Method: item.method, Path: item.path}, item.status)
			}()
			select {
			case <-f.ready:
			case <-done:
				t.Fatal("accepted transition did not hold the response")
			case <-time.After(time.Second):
				t.Fatal("barrier was unreachable")
			}
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("controller death stranded a recorder handler")
			}
		})
	}
}

func TestLifecycleReceiptsRejectWrongObservations(t *testing.T) {
	for _, fault := range []string{"healthy", "wrong code", "missing receipt", "repeated receipt", "wrong signal", "missing wait", "cancelled wait", "fixture-only status"} {
		t.Run(fault, func(t *testing.T) {
			root := t.TempDir()
			now := time.Now()
			w := &world{doctor: &doctorWorld{checkout: root}, lifecycle: &lifecycleNative{
				run: &recoveryProcess{exit: 17}, ready: now, signalled: now.Add(time.Millisecond),
				journalAtReturn: []offlineRequest{{Path: "/containers/id/wait", Status: 200, Replied: now, Observation: map[string]any{"StatusCode": float64(17)}}},
			}}
			receipt := "SIGTERM\n"
			switch fault {
			case "wrong code":
				w.lifecycle.run.exit = 75
			case "missing receipt":
				receipt = ""
			case "repeated receipt":
				receipt += receipt
			case "wrong signal":
				receipt = "SIGINT\n"
			case "missing wait":
				w.lifecycle.journalAtReturn = nil
			case "cancelled wait":
				w.lifecycle.journalAtReturn[0].Cancelled = true
			case "fixture-only status":
				w.lifecycle.journalAtReturn[0].Replied = time.Time{}
			}
			if err := os.WriteFile(filepath.Join(root, "slice-12-receipts"), []byte(receipt), 0600); err != nil {
				t.Fatal(err)
			}
			if err := w.lifecycleReceipts("SIGTERM", 17); (err == nil) != (fault == "healthy") {
				t.Fatalf("assertion accepted wrong evidence or rejected control: %v", err)
			}
		})
	}
}

func TestLifecyclePipeBackpressureIsMeasured(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native pipe-capacity observation requires Linux")
	}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = read.Close(); _ = write.Close() }()
	if lifecycleFullPipe(read) {
		t.Fatal("empty pipe was treated as backpressure")
	}
	done := make(chan struct{})
	go func() { defer close(done); _, _ = write.Write(make([]byte, 4<<20)) }()
	deadline := time.Now().Add(time.Second)
	for !lifecycleFullPipe(read) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !lifecycleFullPipe(read) {
		t.Fatal("filled unread pipe did not report real backpressure")
	}
	_ = read.Close()
	<-done
}

func TestLifecyclePipeInspectionKeepsReadDeadline(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native pipe-capacity observation requires Linux")
	}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = read.Close(); _ = write.Close() }()
	if lifecycleFullPipe(read) {
		t.Fatal("empty pipe was treated as backpressure")
	}
	if err := read.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal("pipe inspection disabled the collection deadline", err)
	}
	// A writer retained after controller exit must not strand collection.
	guard := time.AfterFunc(250*time.Millisecond, func() { _ = write.Close() })
	defer guard.Stop()
	if _, err := io.ReadAll(read); !os.IsTimeout(err) {
		t.Fatalf("open writer did not reach the read deadline: %v", err)
	}
}

func TestLifecycleBoundRequiresRecordedOrigin(t *testing.T) {
	w := &world{lifecycle: &lifecycleNative{returned: time.Now()}}
	if err := w.lifecycleBound(time.Time{}); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing cancellation origin became an elapsed duration: %v", err)
	}
}
