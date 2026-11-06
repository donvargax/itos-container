package features

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"
)

type offlineRequest struct {
	Method, Path, Query string
	Body, Observation   map[string]any
	Record              sessionRecord
	Status              int
	Cancelled           bool
	At, Replied         time.Time
}

// Unlike doctor's read-only recorder and slice-9's unstarted recorder, this
// fixture supports bidirectional HTTP upgrades. It never broadens either one.
type offlineAPI struct {
	mu          sync.Mutex
	w           *world
	server      *http.Server
	requests    []offlineRequest
	created     []string
	absent      map[string]bool
	final       sessionRecord
	problem     error
	connections map[net.Conn]bool
	identities  map[string]map[string]any
	// Recovery faults and signal capabilities belong only to the slice-11
	// fixture. They do not broaden the existing offline runtime scenarios.
	recovery    bool
	failRemove  bool
	removeGate  chan struct{}
	removeReady chan struct{}
	removeHeld  bool
	lifecycle   *lifecycleFaults
}

func (w *world) startOfflineAPI() error {
	dir, err := os.MkdirTemp("/tmp", "slice-10-api-")
	if err != nil {
		return err
	}
	w.doctor.cleanup = append(w.doctor.cleanup, func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "engine.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	a := &offlineAPI{w: w, absent: map[string]bool{}, connections: map[net.Conn]bool{}, identities: map[string]map[string]any{}}
	a.server = &http.Server{Handler: http.HandlerFunc(a.serve), ReadHeaderTimeout: time.Second, ConnState: func(c net.Conn, state http.ConnState) {
		a.mu.Lock()
		defer a.mu.Unlock()
		switch state {
		case http.StateHijacked:
			a.connections[c] = true
		case http.StateClosed:
			delete(a.connections, c)
		}
	}}
	w.offline.api = a
	w.doctor.env["DOCKER_HOST"] = "unix://" + path
	go func() { _ = a.server.Serve(listener) }()
	return nil
}

func offlineHex(value string) bool {
	data, err := hex.DecodeString(value)
	return err == nil && len(data) == 32 && value == strings.ToLower(value)
}

func (a *offlineAPI) serve(out http.ResponseWriter, request *http.Request) {
	entry := offlineRequest{Method: request.Method, Path: doctorRequestPath(request.URL.Path), Query: request.URL.RawQuery, At: time.Now()}
	body, readErr := io.ReadAll(io.LimitReader(request.Body, 65537))
	if len(body) != 0 && json.Unmarshal(body, &entry.Body) != nil {
		readErr = errors.New("non-JSON offline request")
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	a.mu.Lock()
	index := len(a.requests)
	a.requests = append(a.requests, entry) // include every refused/failed request
	allowed := readErr == nil && len(body) <= 65536 && a.allowed(request, &entry)
	if !allowed {
		entry.Status = http.StatusForbidden
	}
	a.requests[index] = entry
	failRemove := allowed && a.failRemove && entry.Method == http.MethodDelete
	fault := allowed && a.removalFault(entry)
	if failRemove || fault {
		a.requests[index].Status = http.StatusServiceUnavailable
	}
	gate := a.removeGate
	hold := allowed && entry.Method == http.MethodDelete && gate != nil && !a.removeHeld
	if hold {
		a.removeHeld = true
	}
	a.mu.Unlock()
	if !allowed {
		http.Error(out, "offline fixture refused unsafe request", http.StatusForbidden)
		return
	}
	if a.lifecycle != nil {
		var cancel context.CancelFunc
		request, cancel = a.lifecycle.request(request)
		defer cancel()
		if a.lifecycle.before(out, request, entry) {
			a.mu.Lock()
			a.requests[index].Status = http.StatusServiceUnavailable
			a.requests[index].Cancelled = request.Context().Err() != nil
			a.mu.Unlock()
			return
		}
	}
	if failRemove {
		http.Error(out, "slice-11 transient removal fault", http.StatusServiceUnavailable)
		return
	}
	if fault {
		http.Error(out, "SLICE_14_SYNTHETIC_REMOVE_BODY", http.StatusServiceUnavailable)
		return
	}
	if hold {
		close(a.removeReady)
		select {
		case <-gate:
		case <-request.Context().Done():
			return
		}
	}
	// ReverseProxy relays the attach upgrade in both directions and preserves
	// half-close, rather than buffering the upgraded stream as an HTTP body.
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL.Scheme, r.Out.URL.Host, r.Out.Host = "http", "engine", "engine"
		},
		Transport:     a.w.doctor.real.transport,
		FlushInterval: -1,
		ModifyResponse: func(response *http.Response) error {
			a.mu.Lock()
			a.requests[index].Status = response.StatusCode
			a.mu.Unlock()
			if response.StatusCode == http.StatusSwitchingProtocols {
				return nil
			}
			if a.lifecycle != nil && strings.HasSuffix(entry.Path, "/wait") && response.StatusCode == http.StatusOK {
				// Acknowledged wait is a streaming subscription. Buffering its
				// entire body here would fabricate a 10 s handshake timeout for
				// quiet workloads, before any product lifetime assertion.
				response.Body = &lifecycleWaitBody{ReadCloser: response.Body, finished: func(data []byte, failure error) {
					var observed map[string]any
					_ = json.Unmarshal(data, &observed)
					a.mu.Lock()
					defer a.mu.Unlock()
					a.requests[index].Cancelled = request.Context().Err() != nil
					if failure == io.EOF && !a.requests[index].Cancelled {
						a.requests[index].Observation = observed
						a.requests[index].Replied = time.Now()
					}
				}}
				return nil
			}
			data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
			_ = response.Body.Close()
			response.Body = io.NopCloser(bytes.NewReader(data))
			if err != nil {
				return err
			}
			var observed map[string]any
			_ = json.Unmarshal(data, &observed)
			a.mu.Lock()
			if entry.Method == "POST" && entry.Path == "/containers/create" && response.StatusCode == http.StatusCreated {
				id, _ := observed["Id"].(string)
				if !offlineHex(id) {
					a.problem = errors.New("native create reply lacks a full container ID")
					a.mu.Unlock()
					return nil
				}
				a.created = append(a.created, id)
				a.identities[id] = map[string]any{"Name": "/itos-container-" + a.w.session.token, "Labels": entry.Body["Labels"]}
				// A create reply alone never counts as observed ownership.
				observed, err = a.inspect(id)
				if err != nil {
					a.problem = err
				}
			}
			if observed != nil {
				a.requests[index].Observation = observed
			}
			a.mu.Unlock()
			if a.lifecycle != nil {
				a.lifecycle.response(request, entry, response.StatusCode)
			}
			a.mu.Lock()
			a.requests[index].Cancelled = request.Context().Err() != nil
			if !a.requests[index].Cancelled {
				a.requests[index].Replied = time.Now()
			}
			a.mu.Unlock()
			return nil
		},
		ErrorHandler: func(out http.ResponseWriter, request *http.Request, _ error) {
			a.mu.Lock()
			a.requests[index].Status = http.StatusBadGateway
			a.requests[index].Cancelled = request.Context().Err() != nil
			if a.lifecycle == nil && (!a.recovery || request.Context().Err() == nil) {
				a.problem = errors.New("offline native recorder transport failed")
			}
			a.mu.Unlock()
			http.Error(out, "offline fixture transport failed", http.StatusBadGateway)
		},
	}
	proxy.ServeHTTP(out, request)
}

// Called under the journal lock. Refusals remain evidence, and are never
// mistaken for product-enforced isolation. Only owned IDs may be mutated.
func (a *offlineAPI) allowed(r *http.Request, entry *offlineRequest) bool {
	w := a.w
	if doctorAllowedRequest(doctorRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery}, w.doctor.image, true) ||
		(r.Method == "GET" && entry.Path == "/images/"+w.doctor.imageID+"/json" && r.URL.RawQuery == "") {
		return true
	}
	if r.Method == "POST" && entry.Path == "/containers/create" {
		token := strings.TrimPrefix(r.URL.Query().Get("name"), "itos-container-")
		limit := 1
		if w.offline.isolation != nil {
			limit = 2
		}
		if !offlineHex(token) || r.URL.Query().Get("name") != "itos-container-"+token || len(r.URL.Query()) != 1 || len(a.created) >= limit {
			return false
		}
		w.session.token = token
		w.session.reference = "intents/" + w.approval.project + "/" + token + ".json"
		var err error
		entry.Record, err = sessionReadRecord(w.sessionRecordPath())
		if err != nil {
			a.problem = err
		}
		return err == nil && a.safeCreate(entry.Body)
	}
	if !strings.HasPrefix(entry.Path, "/containers/") {
		return false
	}
	target := strings.TrimPrefix(entry.Path, "/containers/")
	id, operation, _ := strings.Cut(target, "/")
	if r.Method == "GET" && operation == "json" && r.URL.RawQuery == "" {
		return containsSessionID(a.created, id) || id == "itos-container-"+w.session.token
	}
	if !containsSessionID(a.created, id) || !offlineHex(id) {
		return false
	}
	if r.Method == "POST" && operation == "wait" {
		// Wait is read-only and may arrive after concurrent owned removal.
		// Relay the native response for this exact recorded full ID, including
		// not-found; an absent object cannot provide fresh mutation authority.
		return len(r.URL.Query()) == 1 && r.URL.Query().Get("condition") == "not-running"
	}
	observed, err := a.inspect(id)
	if err != nil || a.identity(observed, id) != nil {
		a.problem = fmt.Errorf("offline recorder refused a mismatched native identity for %s /containers/<owned>/%s (inspection failed: %t)", r.Method, operation, err != nil)
		return false
	}
	entry.Observation = observed
	entry.Record, err = sessionReadRecord(w.sessionRecordPath())
	if err != nil {
		a.problem = err
		return false
	}
	if r.Method == "POST" {
		switch operation {
		case "attach":
			for key, values := range r.URL.Query() {
				if (key != "stream" && key != "stdin" && key != "stdout" && key != "stderr" && key != "logs") || len(values) != 1 || (values[0] != "1" && values[0] != "0" && values[0] != "true" && values[0] != "false") {
					return false
				}
			}
			return true
		case "start":
			return r.URL.RawQuery == "" && a.safeObserved(observed)
		case "kill":
			return a.recovery && len(r.URL.Query()) == 1 &&
				(r.URL.Query().Get("signal") == "SIGTERM" || r.URL.Query().Get("signal") == "SIGKILL" ||
					(a.lifecycle != nil && r.URL.Query().Get("signal") == "SIGINT"))
		case "stop":
			return a.recovery && len(r.URL.Query()) == 2 && r.URL.Query().Get("signal") == "SIGTERM" && r.URL.Query().Get("t") == "10"
		}
	}
	if r.Method == "DELETE" && operation == "" {
		for key, values := range r.URL.Query() {
			if len(values) != 1 || (key != "force" && key != "v") ||
				(key == "v" && values[0] != "false" && values[0] != "0") ||
				(key == "force" && values[0] != "true" && values[0] != "1" && values[0] != "false" && values[0] != "0") {
				return false
			}
		}
		return true
	}
	return false
}

func (a *offlineAPI) identity(c map[string]any, id string) error {
	w := a.w
	name, labels := any("/itos-container-"+w.session.token), any(w.sessionLabels())
	if original := a.identities[id]; original != nil {
		name, labels = original["Name"], original["Labels"]
	}
	if !offlineHex(id) || c["Id"] != id || c["Name"] != name ||
		c["Image"] != w.doctor.imageID || !reflect.DeepEqual(object(c["Config"])["Labels"], labels) {
		return errors.New("offline native observation differs from selected identity")
	}
	return nil
}

func (a *offlineAPI) safeCreate(body map[string]any) bool {
	w := a.w
	host := object(body["HostConfig"])
	if body["Image"] != w.doctor.imageID || body["User"] != w.approval.uid+":"+w.approval.gid ||
		!reflect.DeepEqual(body["Labels"], w.sessionLabels()) || body["Tty"] == true || len(object(body["Volumes"])) != 0 ||
		host["NetworkMode"] != "none" || host["ReadonlyRootfs"] != true || host["Privileged"] == true || host["PublishAllPorts"] == true || host["AutoRemove"] == true ||
		!reflect.DeepEqual(host["CapDrop"], []any{"ALL"}) || !reflect.DeepEqual(host["SecurityOpt"], []any{"no-new-privileges"}) {
		return false
	}
	for _, key := range []string{"Binds", "VolumesFrom", "PortBindings", "CapAdd", "Devices", "DeviceRequests", "GroupAdd", "Links"} {
		if !offlineEmpty(host[key]) {
			return false
		}
	}
	for _, key := range []string{"PidMode", "IpcMode", "UTSMode", "UsernsMode", "CgroupnsMode"} {
		if host[key] != nil && host[key] != "" && host[key] != "private" {
			return false
		}
	}
	binds, temps := 0, 0
	mounts, _ := host["Mounts"].([]any)
	for _, item := range mounts {
		m := object(item)
		switch m["Type"] {
		case "bind":
			options := object(m["BindOptions"])
			if m["Source"] != w.doctor.checkout || m["Target"] != "/workspace" || m["ReadOnly"] == true || options["NonRecursive"] != true || (options["Propagation"] != "rprivate" && options["Propagation"] != "private") {
				return false
			}
			binds++
		case "tmpfs":
			if m["Target"] != "/tmp" || (m["Source"] != nil && m["Source"] != "") {
				return false
			}
			temps++
		default:
			return false
		}
	}
	for path := range object(host["Tmpfs"]) {
		if path != "/tmp" {
			return false
		}
		temps++
	}
	return binds == 1 && temps == 1
}

func offlineEmpty(value any) bool {
	if value == nil {
		return true
	}
	switch v := value.(type) {
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	}
	return false
}

func (a *offlineAPI) safeObserved(c map[string]any) bool {
	config := approvalClone(object(c["Config"]))
	config["Image"] = c["Image"]
	config["HostConfig"] = c["HostConfig"]
	return a.safeCreate(config)
}

func (a *offlineAPI) inspect(id string) (map[string]any, error) {
	var result map[string]any
	err := a.w.doctor.real.read("/containers/"+id+"/json", &result, 200)
	return result, err
}

func (a *offlineAPI) running() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.created) != 1 {
		return false
	}
	c, err := a.inspect(a.created[0])
	return err == nil && a.identity(c, a.created[0]) == nil && object(c["State"])["Running"] == true
}

func (a *offlineAPI) observeAfter() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, id := range a.created {
		// Explicit 404, never connection failure or an empty inventory.
		a.absent[id] = a.w.doctor.real.read("/containers/"+id+"/json", nil, 404) == nil
	}
	var err error
	a.final, err = sessionReadRecord(a.w.sessionRecordPath())
	return errors.Join(a.problem, err)
}

func (a *offlineAPI) closeConnections() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for conn := range a.connections {
		_ = conn.Close()
	}
}

func (a *offlineAPI) cleanup() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var result error
	for _, id := range a.created {
		if a.w.doctor.real.read("/containers/"+id+"/json", nil, 404) == nil {
			continue
		}
		observed, err := a.inspect(id)
		if err != nil || a.identity(observed, id) != nil {
			result = errors.Join(result, errors.New("offline harness refused cleanup without matching full identity"))
			continue
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodDelete, "http://engine/containers/"+url.PathEscape(id)+"?force=true&v=false", nil)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		response, err := a.w.doctor.real.client.Do(request)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			result = errors.Join(result, fmt.Errorf("offline harness cleanup HTTP %d", response.StatusCode))
			continue
		}
		result = errors.Join(result, a.w.doctor.real.read("/containers/"+id+"/json", nil, 404))
	}
	return result
}
