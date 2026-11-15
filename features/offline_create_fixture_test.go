package features

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// A disposable in-memory engine for the recorder's create accounting. Every
// connection is a net.Pipe, so a testing/synctest bubble's clock governs each
// wait. No native engine, product process or approval command takes part.
//
// Like the real engine it finishes a create it has read whether or not anyone
// still waits for the answer, and refuses to delete a volume a worker holds.
type createEngine struct {
	mu       sync.Mutex
	requests []string                  // "METHOD request-URI", in arrival order
	workers  map[string]map[string]any // full ID to inspection document
	volumes  map[string]map[string]any // name to inspection document
	arrived  chan struct{}             // one token per create read off the wire
	release  chan struct{}             // closed to let the engine finish creates
	created  chan struct{}             // one token per create the engine finished
	once     sync.Once
	drop     bool                                             // finish the create, then drop the connection unanswered
	shortID  bool                                             // answer a create with a 12-character ID
	override func(method, uri string) (status int, value any) // status 0 falls through
}

func newCreateEngine() *createEngine {
	return &createEngine{workers: map[string]map[string]any{}, volumes: map[string]map[string]any{},
		arrived: make(chan struct{}, 16), release: make(chan struct{}), created: make(chan struct{}, 16)}
}

func (e *createEngine) finish() { e.once.Do(func() { close(e.release) }) }

func (e *createEngine) transport() *http.Transport {
	return &http.Transport{DisableKeepAlives: true, DialContext: func(context.Context, string, string) (net.Conn, error) {
		near, far := net.Pipe()
		go e.serve(far)
		return near, nil
	}}
}

func (e *createEngine) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	request, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return
	}
	// Read the body before any wait: a server that has not cannot notice a closed
	// connection, and the real engine accepts the create once it has the body.
	body, _ := io.ReadAll(request.Body)
	uri := request.URL.RequestURI()
	e.mu.Lock()
	e.requests = append(e.requests, request.Method+" "+uri)
	e.mu.Unlock()
	status, value := e.handle(request, body)
	if status == 0 {
		return
	}
	data := []byte{}
	if status != http.StatusNoContent {
		data = approvalJSON(value)
	}
	_, _ = fmt.Fprintf(conn, "HTTP/1.1 %d %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		status, http.StatusText(status), len(data), data)
}

func (e *createEngine) handle(r *http.Request, body []byte) (int, any) {
	if e.override != nil {
		if status, value := e.override(r.Method, r.URL.RequestURI()); status != 0 {
			return status, value
		}
	}
	missing := map[string]any{"message": "No such object: private-engine-text"}
	path := r.URL.Path
	switch {
	case r.Method == "POST" && path == "/containers/create":
		e.arrived <- struct{}{}
		<-e.release
		var create map[string]any
		_ = json.Unmarshal(body, &create)
		e.mu.Lock()
		id := strings.Repeat(string(rune('a'+len(e.workers))), 64)
		e.workers[id] = map[string]any{"Id": id, "Name": "/" + r.URL.Query().Get("name"), "Image": create["Image"],
			"Config": map[string]any{"Labels": create["Labels"]}, "State": map[string]any{"Status": "created", "Running": false}}
		drop, short := e.drop, e.shortID
		e.mu.Unlock()
		e.created <- struct{}{}
		if drop {
			return 0, nil
		}
		if short {
			id = id[:12]
		}
		return http.StatusCreated, map[string]any{"Id": id}
	case r.Method == "GET" && path == "/containers/json":
		e.mu.Lock()
		defer e.mu.Unlock()
		holders := []any{}
		for id := range e.workers {
			holders = append(holders, map[string]any{"Id": id})
		}
		return http.StatusOK, holders
	case strings.HasPrefix(path, "/containers/"):
		reference := strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/json")
		e.mu.Lock()
		defer e.mu.Unlock()
		for id, worker := range e.workers {
			if reference != id && "/"+reference != worker["Name"] {
				continue
			}
			if r.Method == "DELETE" {
				delete(e.workers, id)
				return http.StatusNoContent, nil
			}
			return http.StatusOK, worker
		}
		return http.StatusNotFound, missing
	case strings.HasPrefix(path, "/volumes/"):
		name := strings.TrimPrefix(path, "/volumes/")
		e.mu.Lock()
		defer e.mu.Unlock()
		volume := e.volumes[name]
		if volume == nil {
			return http.StatusNotFound, missing
		}
		if r.Method != "DELETE" {
			return http.StatusOK, volume
		}
		// Every worker of this fixture mounts the one volume.
		if len(e.workers) != 0 {
			return http.StatusConflict, map[string]any{"message": "remove private-volume: volume is in use - [private-container]"}
		}
		delete(e.volumes, name)
		return http.StatusNoContent, nil
	}
	return http.StatusBadRequest, map[string]any{"message": "unexpected fixture request"}
}

// The engine finishes a create it has read even when nobody waits for the
// answer; a test that judges the aftermath waits for that first.
func (e *createEngine) awaitWorker(t *testing.T) string {
	t.Helper()
	select {
	case <-e.created:
	case <-time.After(5 * time.Second):
		t.Fatal("the disposable engine never finished the create")
	}
	return e.worker()
}

func (e *createEngine) journal() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.requests...)
}

func (e *createEngine) worker() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id := range e.workers {
		return id
	}
	return ""
}

// A hand-made world whose recorder forwards to that engine: plain, recovery or
// lifecycle, the three kinds of native fixture built on offlineFixture.
func createRecorderWorld(t *testing.T, kind string) (*world, *createEngine) {
	t.Helper()
	dir := t.TempDir()
	w := &world{dir: dir,
		doctor: &doctorWorld{checkout: filepath.Join(dir, "checkout"), image: "fixture@sha256:" + strings.Repeat("1", 64),
			imageID: "sha256:" + strings.Repeat("2", 64), env: map[string]string{"XDG_STATE_HOME": filepath.Join(dir, "state")}},
		approval: &approvalWorld{project: strings.Repeat("3", 64), fingerprint: strings.Repeat("4", 64), uid: "1000", gid: "1000"},
		session:  &sessionWorld{},
		offline:  &offlineRuntime{exit: -1},
	}
	e := newCreateEngine()
	transport := e.transport()
	w.doctor.real = &doctorReal{transport: transport, client: &http.Client{Transport: transport, Timeout: time.Second}}
	a := &offlineAPI{w: w, absent: map[string]bool{}, connections: map[net.Conn]bool{}, identities: map[string]map[string]any{}}
	w.offline.api = a
	switch kind {
	case "plain":
	case "recovery":
		a.recovery, w.recovery = true, &recoveryNative{mode: "loss"}
	case "lifecycle":
		a.recovery, w.recovery = true, &recoveryNative{mode: "lifecycle"}
		w.lifecycle = &lifecycleNative{mode: "handle", fault: newLifecycleFaults("handle", "", 0)}
		a.lifecycle = w.lifecycle.fault
	default:
		t.Fatalf("unknown recorder kind %q", kind)
	}
	t.Cleanup(func() {
		// An engine still holding a create answers into a closed connection.
		e.finish()
		a.awaitCreates()
		transport.CloseIdleConnections()
	})
	return w, e
}

// The anchor volume a worker of this fixture mounts, recorded as the product's
// own volume create would have left it.
func (e *createEngine) anchor(w *world, token string) *anchorVolume {
	name := "itos-container-anchor-" + token
	labels := map[string]any{"io.itos-container.intent": token, "io.itos-container.kind": "checkout-anchor"}
	options := map[string]any{"type": "none", "o": "bind", "device": "/proc/123/fd/7"}
	observed := map[string]any{"Name": name, "Driver": "local", "Scope": "local", "CreatedAt": "2026-11-13T00:00:00Z", "Labels": labels, "Options": options}
	e.mu.Lock()
	e.volumes[name] = observed
	e.mu.Unlock()
	v := &anchorVolume{name: name, labels: approvalClone(labels), options: approvalClone(options), observed: approvalClone(observed)}
	w.offline.api.volumes = map[string]*anchorVolume{name: v}
	return v
}

// A create the recorder authorizes: the hardened request of the product, with
// the original direct bind so that no volume has to exist first.
func createRequest(ctx context.Context, w *world, token string) *http.Request {
	body := map[string]any{
		"Image": w.doctor.imageID, "User": w.approval.uid + ":" + w.approval.gid, "Tty": false,
		"Labels": map[string]any{"io.itos-container.intent": token, "io.itos-container.project": w.approval.project, "io.itos-container.fingerprint": w.approval.fingerprint},
		"Env":    []any{"SYNTHETIC_PRIVATE_FIELD=private-environment"},
		"HostConfig": map[string]any{
			"NetworkMode": "none", "ReadonlyRootfs": true, "CapDrop": []any{"ALL"}, "SecurityOpt": []any{"no-new-privileges"},
			"Mounts": []any{
				map[string]any{"Type": "bind", "Source": w.doctor.checkout, "Target": "/workspace", "BindOptions": map[string]any{"NonRecursive": true, "Propagation": "rprivate"}},
				map[string]any{"Type": "tmpfs", "Target": "/tmp"},
			},
		},
	}
	request := httptest.NewRequest("POST", "http://engine/containers/create?name=itos-container-"+token, strings.NewReader(string(approvalJSON(body))))
	request.Header.Set("Content-Type", "application/json")
	return request.WithContext(ctx)
}

// One create driven through serve, the handler of the recorder's server, as a
// controller's request arrives there.
type createCall struct {
	token    string
	cancel   context.CancelFunc
	response *httptest.ResponseRecorder
	returned chan struct{}
}

func startCreate(t *testing.T, w *world, e *createEngine, token string) *createCall {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	c := &createCall{token: token, cancel: cancel, response: httptest.NewRecorder(), returned: make(chan struct{})}
	t.Cleanup(cancel)
	go func() {
		defer close(c.returned)
		w.offline.api.serve(c.response, createRequest(ctx, w, token))
	}()
	select {
	case <-e.arrived:
	case <-c.returned:
		// An engine that answers at once can have the handler return before this
		// select runs; its arrival token is then already waiting.
		select {
		case <-e.arrived:
		default:
			t.Fatalf("recorder answered HTTP %d before forwarding the create", c.response.Code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("create never reached the engine")
	}
	return c
}

// The controller leaves: its request ends, as a closed connection ends it in
// the recorder's server, and the handler returns.
func (c *createCall) abandon(t *testing.T) {
	t.Helper()
	c.cancel()
	c.wait(t)
}

func (c *createCall) wait(t *testing.T) {
	t.Helper()
	select {
	case <-c.returned:
	case <-time.After(5 * time.Second):
		t.Fatal("recorder handler never returned")
	}
}

func (c *createCall) pending() bool {
	select {
	case <-c.returned:
		return false
	default:
		return true
	}
}

// The journal entry of the only create, read under the recorder lock.
func createEntry(t *testing.T, a *offlineAPI) offlineRequest {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()
	var found []offlineRequest
	for _, q := range a.requests {
		if q.Method == "POST" && q.Path == "/containers/create" {
			found = append(found, q)
		}
	}
	if len(found) != 1 {
		t.Fatalf("journal holds %d creates, want 1", len(found))
	}
	return found[0]
}

func createListed(a *offlineAPI) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.created...)
}

func createProblem(a *offlineAPI) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.problem == nil {
		return ""
	}
	return a.problem.Error()
}

const (
	createAbandonedWords = "offline native recorder create abandoned: the controller left before the reply"
	createTransportWords = "offline native recorder transport failed"
)

var createTokens = []string{strings.Repeat("5", 64), strings.Repeat("6", 64), strings.Repeat("7", 64), strings.Repeat("8", 64), strings.Repeat("9", 64)}
