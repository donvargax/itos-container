package features

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// One recording daemon serves several projects and concurrent same-root runs.
// No local fixture executes a workload. Native execution has its own recorder.
type overlapEngine struct {
	mu          sync.Mutex
	metadata    *doctorAPI
	server      *http.Server
	endpoint    string
	requests    []overlapRequest
	workers     map[string]*overlapWorker
	volumes     map[string]map[string]any
	volumeRoots map[string]string
	condition   string
	faultRoot   string
	createGate  chan struct{}
	preflight   *overlapRendezvous
}

type overlapRequest struct {
	method, path, root, token string
	pid, status               int
}

type overlapWorker struct {
	value       map[string]any
	root, token string
	exited      chan struct{}
	attached    net.Conn
	removed     bool
}

// The replies are held independently of the journal mutex. A blocked create or
// preflight must not serialize the very interleaving the scenario is measuring.
type overlapRendezvous struct {
	mu      sync.Mutex
	arrived map[int]bool
	release chan struct{}
}

func newOverlapRendezvous() *overlapRendezvous {
	return &overlapRendezvous{arrived: map[int]bool{}, release: make(chan struct{})}
}

func (b *overlapRendezvous) wait(ctx context.Context, pid int) {
	b.mu.Lock()
	b.arrived[pid] = true
	b.mu.Unlock()
	select {
	case <-b.release:
	case <-ctx.Done():
	}
}

func (b *overlapRendezvous) count() int { b.mu.Lock(); defer b.mu.Unlock(); return len(b.arrived) }

func newOverlapEngine(w *world, id string) (*overlapEngine, error) {
	dir, err := os.MkdirTemp("/tmp", "slice-16-api-")
	if err != nil {
		return nil, err
	}
	w.doctor.cleanup = append(w.doctor.cleanup, func() { _ = os.RemoveAll(dir) })
	return newOverlapEngineAt(w, id, filepath.Join(dir, "engine.sock"))
}

func newOverlapEngineAt(w *world, id, path string) (*overlapEngine, error) {
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	a := &overlapEngine{metadata: newDoctorAPI(w.doctor.image), endpoint: "unix://" + path, workers: map[string]*overlapWorker{}, volumes: map[string]map[string]any{}, volumeRoots: map[string]string{}}
	a.metadata.info["ID"] = id
	a.metadata.config()["User"] = w.approval.uid + ":" + w.approval.gid
	a.server = &http.Server{Handler: http.HandlerFunc(a.serve), ReadHeaderTimeout: time.Second, ConnContext: func(ctx context.Context, c net.Conn) context.Context {
		return context.WithValue(ctx, runPeerKey{}, runPeerPID(c))
	}}
	go func() { _ = a.server.Serve(listener) }()
	return a, nil
}

func (a *overlapEngine) serve(out http.ResponseWriter, r *http.Request) {
	path := doctorRequestPath(r.URL.Path)
	pid, _ := r.Context().Value(runPeerKey{}).(int)
	if !strings.HasPrefix(path, "/containers/") && !strings.HasPrefix(path, "/volumes/") {
		if path == "/images/"+doctorImageID+"/json" {
			_ = json.NewEncoder(out).Encode(a.metadata.image)
			return
		}
		if strings.HasPrefix(path, "/images/") && a.preflight != nil {
			a.preflight.wait(r.Context(), pid)
		}
		a.metadata.serve(out, r)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	q := overlapRequest{method: r.Method, path: path, pid: pid, status: 200}
	index := len(a.requests)
	a.requests = append(a.requests, q)
	status := 200
	var reply any
	hijacked := false
	defer func() {
		a.requests[index].status = status
		if hijacked {
			return
		}
		out.Header().Set("Content-Type", "application/json")
		out.WriteHeader(status)
		if reply != nil {
			_ = json.NewEncoder(out).Encode(reply)
		}
	}()
	if strings.HasPrefix(path, "/volumes/") {
		var body map[string]any
		if r.Method == "POST" {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		status, reply = localVolumeReply(a.volumes, r.Method, path, body)
		if r.Method == "POST" && status == 201 {
			name, _ := body["Name"].(string)
			source, _ := object(body["DriverOpts"])["device"].(string)
			root, err := os.Readlink(source)
			if err != nil {
				status = 400
				return
			}
			a.volumeRoots[name] = root
		}
		return
	}
	if path == "/containers/create" && r.Method == "POST" {
		var body map[string]any
		if json.NewDecoder(http.MaxBytesReader(out, r.Body, 65536)).Decode(&body) != nil {
			status = 400
			return
		}
		host := object(body["HostConfig"])
		mounts, _ := host["Mounts"].([]any)
		root := ""
		for _, m := range mounts {
			if object(m)["Type"] == "volume" {
				name, _ := object(m)["Source"].(string)
				root = a.volumeRoots[name]
			}
		}
		token := strings.TrimPrefix(r.URL.Query().Get("name"), "itos-container-")
		if root == "" || !offlineHex(token) {
			status = 400
			return
		}
		id := approvalHash(map[string]any{"slice-16-worker": token})
		delete(body, "HostConfig")
		worker := &overlapWorker{root: root, token: token, exited: make(chan struct{}), value: map[string]any{
			"Id": id, "Name": "/itos-container-" + token, "Image": body["Image"], "Config": body, "HostConfig": host,
			"Mounts":          localAnchorMount(host),
			"State":           map[string]any{"Status": "created", "Running": false, "Pid": 0, "StartedAt": "0001-01-01T00:00:00Z"},
			"NetworkSettings": map[string]any{"Networks": map[string]any{"none": map[string]any{}}},
		}}
		a.workers[id] = worker
		a.requests[index].root, a.requests[index].token = root, token
		if a.createGate != nil {
			gate := a.createGate
			a.mu.Unlock()
			select {
			case <-gate:
			case <-r.Context().Done():
			}
			a.mu.Lock()
		}
		if root == a.faultRoot && a.condition == "lost create reply with its worker initially invisible" {
			conn, _, err := out.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
				hijacked = true
			}
			return
		}
		status, reply = 201, map[string]any{"Id": id}
		return
	}
	parts := strings.Split(strings.TrimPrefix(path, "/containers/"), "/")
	id := parts[0]
	c := a.workers[id]
	if strings.HasPrefix(id, "itos-container-") {
		for _, worker := range a.workers {
			if worker.token == strings.TrimPrefix(id, "itos-container-") {
				c = worker
				break
			}
		}
	}
	if c != nil {
		a.requests[index].root, a.requests[index].token = c.root, c.token
		if c.root == a.faultRoot {
			switch a.condition {
			case "controller absent with resource observation unavailable":
				status = 503
				return
			case "lost create reply with its worker initially invisible":
				status = 404
				return
			}
		}
	}
	if c == nil || c.removed {
		status = 404
		return
	}
	switch {
	case len(parts) == 2 && parts[1] == "json" && r.Method == "GET":
		reply = approvalClone(c.value)
	case len(parts) == 2 && parts[1] == "attach" && r.Method == "POST":
		conn, buf, err := out.(http.Hijacker).Hijack()
		if err != nil {
			status = 500
			return
		}
		c.attached, hijacked = conn, true
		_, _ = buf.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
		_ = buf.Flush()
		status = 101
	case len(parts) == 2 && parts[1] == "start" && r.Method == "POST":
		c.value["State"] = map[string]any{"Status": "running", "Running": true, "Pid": 123, "StartedAt": "2026-01-01T00:00:00Z"}

		status = 204
	case len(parts) == 2 && parts[1] == "wait" && r.Method == "POST":
		if c.root == a.faultRoot && (strings.HasPrefix(a.condition, "workload exited") || a.condition == "resource removed with session completion fsync failed") {
			c.exit()
		}
		// Flush acknowledgement so SDK wait doesn't retain its startup deadline.
		out.WriteHeader(200)
		out.(http.Flusher).Flush()
		hijacked = true
		a.mu.Unlock()
		select {
		case <-c.exited:
		case <-r.Context().Done():
		}
		a.mu.Lock()
		if r.Context().Err() == nil {
			_ = json.NewEncoder(out).Encode(map[string]int{"StatusCode": 7})
		}
	case len(parts) == 2 && (parts[1] == "kill" || parts[1] == "stop") && r.Method == "POST":
		c.exit()
		status = 204
	case len(parts) == 1 && r.Method == "DELETE":
		if c.root == a.faultRoot && a.condition == "workload exited with removal refused" {
			status = 503
			return
		}
		if c.root == a.faultRoot && a.condition == "workload exited with successful DELETE but worker still present" {
			status = 204
			return
		}
		c.exit()
		c.removed = true
		status = 204
	default:
		status = 403
	}
}

func (c *overlapWorker) exit() {
	select {
	case <-c.exited:
	default:
		close(c.exited)
	}
	c.value["State"] = map[string]any{"Status": "exited", "Running": false, "Pid": 0, "StartedAt": "2026-01-01T00:00:00Z"}
	if c.attached != nil {
		_ = c.attached.Close()
	}
}

func (a *overlapEngine) close() {
	_ = a.server.Close()
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, c := range a.workers {
		if c.attached != nil {
			_ = c.attached.Close()
		}
	}
}

func (a *overlapEngine) journal() []overlapRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]overlapRequest{}, a.requests...)
}

func overlapAwait(label string, timeout time.Duration, ready func() bool) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		if ready() {
			return nil
		}
		select {
		case <-timer.C:
			return fmt.Errorf("bounded overlap observation timed out: %s", label)
		case <-tick.C:
		}
	}
}
