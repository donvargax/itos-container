package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"
)

type sessionRequest struct {
	Method, Path, Query string
	Body                map[string]any
	Response            map[string]any
	Record              sessionRecord
	Events              []sessionEvent
	Status              int
}

type sessionAPI struct {
	mu       sync.Mutex
	w        *world
	server   *http.Server
	requests []sessionRequest
	created  []string
	observed map[string]map[string]any
	local    map[string]map[string]any
	absent   map[string]bool
	problem  error
	attached net.Conn
}

func (w *world) startSessionAPI() error {
	dir, err := os.MkdirTemp("/tmp", "slice-9-api-")
	if err != nil {
		return err
	}
	w.doctor.cleanup = append(w.doctor.cleanup, func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "engine.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	a := &sessionAPI{w: w, observed: map[string]map[string]any{}, local: map[string]map[string]any{}, absent: map[string]bool{}}
	a.server = &http.Server{Handler: http.HandlerFunc(a.serve), ReadHeaderTimeout: time.Second}
	w.session.api = a
	w.doctor.env["DOCKER_HOST"] = "unix://" + path
	go func() { _ = a.server.Serve(listener) }()
	return nil
}

func (a *sessionAPI) serve(out http.ResponseWriter, request *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	entry := sessionRequest{Method: request.Method, Path: doctorRequestPath(request.URL.Path), Query: request.URL.RawQuery}
	body, err := io.ReadAll(io.LimitReader(request.Body, 65537))
	if err != nil || len(body) > 65536 {
		a.problem = errors.New("session request body unavailable or too large")
		out.WriteHeader(400)
		return
	}
	if len(body) != 0 {
		if err = json.Unmarshal(body, &entry.Body); err != nil {
			a.problem = errors.New("session request body is not an object")
		}
	}
	if (entry.Method == "POST" && entry.Path == "/containers/create") || entry.Method == "DELETE" || (a.w.session.offline && entry.Method == "POST") {
		entry.Record, err = sessionReadRecord(a.w.sessionRecordPath())
		if err != nil {
			a.problem = err
		}
	}
	entry.Events, err = sessionReadEvents(a.w.session.journal)
	if err != nil {
		a.problem = err
	}
	// Every request, even a refused start/acquisition, is recorded before dispatch.
	a.requests = append(a.requests, entry)
	index := len(a.requests) - 1
	if a.w.session.offline && strings.HasPrefix(entry.Path, "/containers/") {
		a.serveOffline(out, request, entry, index)
		return
	}
	if !a.allowed(request, entry) {
		a.requests[index].Status = http.StatusForbidden
		http.Error(out, "session fixture refuses request "+doctorCanary, http.StatusForbidden)
		return
	}
	status, headers, response, err := a.respond(request, body)
	if err != nil {
		a.problem = err
		status, response = 502, []byte(`{"message":"session fixture observation failed"}`)
	}
	a.requests[index].Status = status
	if request.Method == "POST" && entry.Path == "/containers/create" && status == 201 {
		var created struct {
			ID string `json:"Id"`
		}
		if json.Unmarshal(response, &created) != nil || created.ID == "" {
			a.problem = errors.New("engine create reply lacked independent ID")
		} else {
			a.created = append(a.created, created.ID)
			observed, err := a.inspect(created.ID)
			if err != nil {
				a.problem = err
			} else {
				a.observed[created.ID] = observed
			}
		}
		if a.w.session.fault == "empty create ID" {
			response = []byte(`{"Id":"","Warnings":[]}`)
		}
		if a.w.session.fault == "lost create response" || a.w.session.fault == "delayed create" {
			conn, _, err := out.(http.Hijacker).Hijack()
			if err != nil {
				a.problem = err
			} else {
				_ = conn.Close()
			}
			return
		}
	}
	if request.Method == "GET" && strings.HasPrefix(entry.Path, "/containers/") && status == 404 {
		id := strings.TrimSuffix(strings.TrimPrefix(entry.Path, "/containers/"), "/json")
		if a.w.doctor.real != nil {
			if err := a.w.doctor.real.read("/containers/"+id+"/json", nil, 404); err != nil {
				a.problem = err
			} else {
				a.absent[id] = true
			}
		} else {
			a.absent[id] = a.local[id] == nil
		}
	}
	if entry.Method == "DELETE" && a.w.session.removal == "response lost" {
		conn, _, hijackErr := out.(http.Hijacker).Hijack()
		if hijackErr != nil {
			a.problem = hijackErr
		} else {
			_ = conn.Close()
		}
		return
	}
	if entry.Method == "GET" && strings.HasPrefix(entry.Path, "/containers/") && a.w.session.inspection == "replace socket" {
		a.w.session.inspection = ""
		path := strings.TrimPrefix(a.w.doctor.env["DOCKER_HOST"], "unix://")
		if e := os.Rename(path, path+".old"); e != nil {
			a.problem = e
		} else {
			listener, e := net.Listen("unix", path)
			if e != nil {
				a.problem = e
			} else {
				go func() { _ = a.server.Serve(listener) }()
			}
		}
	}
	for _, name := range []string{"Content-Type", "API-Version", "Docker-Experimental", "OSType"} {
		if value := headers.Get(name); value != "" {
			out.Header().Set(name, value)
		}
	}
	out.WriteHeader(status)
	_, _ = out.Write(response)
}

func (a *sessionAPI) allowed(r *http.Request, entry sessionRequest) bool {
	w := a.w
	if doctorAllowedRequest(doctorRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery}, w.doctor.image, true) {
		return true
	}
	if entry.Method == "GET" && entry.Path == "/images/"+w.doctor.imageID+"/json" && r.URL.RawQuery == "" {
		return true
	}
	name := "itos-container-" + w.session.token
	if entry.Path == "/containers/create" && r.Method == "POST" {
		return len(r.URL.Query()) == 1 && r.URL.Query().Get("name") == name && w.sessionRequestProblem(entry.Body) == nil
	}
	if !strings.HasPrefix(entry.Path, "/containers/") {
		return false
	}
	id := strings.TrimPrefix(entry.Path, "/containers/")
	if r.Method == "GET" && strings.HasSuffix(id, "/json") && r.URL.RawQuery == "" {
		id = strings.TrimSuffix(id, "/json")
		return id == name || containsSessionID(a.created, id) || (w.doctor.real == nil && id == w.session.seedID)
	}
	if r.Method != "DELETE" || (!containsSessionID(a.created, id) && (w.doctor.real != nil || id != w.session.seedID)) {
		return false
	}
	for key, values := range r.URL.Query() {
		if (key != "force" && key != "v") || len(values) != 1 || (values[0] != "false" && values[0] != "0") {
			return false
		}
	}
	if w.doctor.real == nil {
		return true
	}
	// Independently re-inspect immediately before forwarding removal. This is a
	// harness safety check; only product requests count toward behavior evidence.
	observed, err := a.inspect(id)
	if err != nil || w.sessionIdentityProblem(observed, id) != nil {
		return false
	}
	a.observed[id] = observed
	return true
}

func containsSessionID(ids []string, id string) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

func (a *sessionAPI) respond(r *http.Request, body []byte) (int, http.Header, []byte, error) {
	if a.w.doctor.real == nil {
		return a.respondLocal(r)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, r.Method, "http://engine"+r.URL.RequestURI(), bytes.NewReader(body))
	if err != nil {
		return 0, nil, nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := a.w.doctor.real.transport.RoundTrip(request)
	if err != nil {
		return 0, nil, nil, err
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	return response.StatusCode, response.Header, data, err
}

func (a *sessionAPI) respondLocal(r *http.Request) (int, http.Header, []byte, error) {
	path := doctorRequestPath(r.URL.Path)
	header := http.Header{"Content-Type": []string{"application/json"}, "API-Version": []string{"1.56"}, "OSType": []string{"linux"}}
	base := a.w.doctor.engine
	var value any
	s := a.w.session
	compound := false
	if s.fault == "owned-record file fsync then unavailable" {
		events, _ := sessionReadEvents(s.journal)
		for _, e := range events {
			if e.Stage == "owned" && e.Operation == "file fsync" && e.Failed {
				compound = true
			}
		}
	}
	if compound {
		return 503, header, []byte(`{"message":"unavailable"}`), nil
	}
	switch path {
	case "/_ping":
		return 200, header, []byte("OK"), nil
	case "/version":
		value = base.version
	case "/info":
		value = base.info
	case "/images/" + a.w.doctor.image + "/json":
		value = base.image
	case "/images/" + a.w.doctor.imageID + "/json":
		image := approvalClone(base.image)
		if s.mode == "image" {
			switch s.condition {
			case "declared volume":
				object(image["Config"])["Volumes"] = map[string]any{"/data": map[string]any{}}
			case "different image config ID":
				image["Id"] = "sha256:" + strings.Repeat("e", 64)
			case "missing Config":
				delete(image, "Config")
			case "unavailable":
				return 503, header, []byte(`{"message":"unavailable"}`), nil
			case "malformed response":
				return 200, header, []byte(`{"broken"`), nil
			}
		}
		value = image
	case "/containers/create":
		id := approvalHash(map[string]any{"fixture-created": a.w.session.token})
		labels := a.w.sessionLabels()
		a.local[id] = map[string]any{"Id": id, "Name": "/itos-container-" + a.w.session.token, "Image": a.w.doctor.imageID,
			"Config": map[string]any{"Labels": labels}, "State": map[string]any{"Status": "created", "Running": false, "Pid": float64(0), "StartedAt": "0001-01-01T00:00:00Z"}}
		return 201, header, approvalJSON(map[string]any{"Id": id, "Warnings": []string{}}), nil
	default:
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/json")
		if r.Method == "DELETE" {
			if s.following != "still present" {
				delete(a.local, id)
			}
			return 204, header, nil, nil
		}
		condition := s.inspection
		removed := false
		for _, q := range a.requests {
			if q.Method == "DELETE" {
				removed = true
			}
		}
		if removed {
			condition = s.following
		}
		if s.fault == "delayed create" {
			condition = s.following
		}
		switch condition {
		case "unavailable":
			return 503, header, []byte(`{"message":"unavailable"}`), nil
		case "explicit not-found":
			return 404, header, []byte(`{"message":"absent"}`), nil
		case "malformed response":
			return 200, header, []byte(`{"broken"`), nil
		}
		value = a.local[id]
		if id == "itos-container-"+a.w.session.token {
			for _, candidate := range a.local {
				if candidate["Name"] == "/"+id {
					value = candidate
				}
			}
		}
		if value != nil && !reflect.ValueOf(value).IsNil() {
			c := approvalClone(value.(map[string]any))
			// Name lookup stays matching in the contradictory-observations case.
			if s.condition != "name inspection and ID inspection disagree" || id != "itos-container-"+s.token {
				sessionMutateInspection(c, condition)
			}
			value = c
		}
		if value == nil || reflect.ValueOf(value).IsNil() {
			return 404, header, []byte(`{"message":"No such container"}`), nil
		}
	}
	return 200, header, approvalJSON(value), nil
}

func (a *sessionAPI) inspect(id string) (map[string]any, error) {
	if a.w.doctor.real == nil {
		if a.local[id] == nil {
			return nil, errors.New("fixture container absent")
		}
		return approvalClone(a.local[id]), nil
	}
	var observed map[string]any
	err := a.w.doctor.real.read("/containers/"+id+"/json", &observed, 200)
	return observed, err
}

func (a *sessionAPI) cleanupObserved() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.w.doctor.real == nil {
		return nil
	}
	var result error
	for _, id := range a.created {
		if err := a.w.doctor.real.read("/containers/"+id+"/json", nil, 404); err == nil {
			continue
		}
		observed, err := a.inspect(id)
		if err == nil {
			err = a.w.sessionIdentityProblem(observed, id)
		}
		if err != nil {
			result = errors.Join(result, errors.New("session harness cannot safely remove observed resource"))
			continue
		}
		request := &http.Request{Method: "DELETE", URL: &url.URL{Path: "/containers/" + id}}
		status, _, _, err := a.respond(request.WithContext(context.Background()), nil)
		if err != nil || status != 204 {
			result = errors.Join(result, errors.New("session harness removal failed"))
			continue
		}
		result = errors.Join(result, a.w.doctor.real.read("/containers/"+id+"/json", nil, 404))
	}
	return result
}

func sessionReadEvents(path string) ([]sessionEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	decoder := json.NewDecoder(file)
	var events []sessionEvent
	for {
		var event sessionEvent
		if err := decoder.Decode(&event); errors.Is(err, io.EOF) {
			return events, nil
		} else if err != nil {
			return nil, fmt.Errorf("incomplete session observation journal: %w", err)
		}
		events = append(events, event)
	}
}
