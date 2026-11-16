package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// This recorder forwards every successful operation to the real daemon. Its
// journal is observation only, including the stored bytes visible immediately
// before the product removes a proxy. Independent fixture traffic bypasses it.
type egressLifeRecorder struct {
	mu        sync.Mutex
	transport *http.Transport
	server    *http.Server
	requests  []*egressLifeRequest
	logPath   string
}

type egressLifeRequest struct {
	method, path, query string
	status              int
	body, response      []byte
	at, finished        time.Time
	logBefore           []byte
	logMode             os.FileMode
	logErr              error
}

type egressLifeBody struct {
	io.ReadCloser
	recorder *egressLifeRecorder
	request  *egressLifeRequest
}

func (b *egressLifeBody) Write(p []byte) (int, error) {
	writer, ok := b.ReadCloser.(io.Writer)
	if !ok {
		return 0, errors.New("engine response is not an upgraded stream")
	}
	return writer.Write(p)
}

func (b *egressLifeBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.recorder.mu.Lock()
	// Configs and wait replies fit this bound. Log streams stay streamed; the
	// last-refusal assertion independently reads the protected file at removal.
	if left := 1<<20 - len(b.request.response); left > 0 {
		b.request.response = append(b.request.response, p[:min(n, left)]...)
	}
	if err != nil {
		b.request.finished = time.Now()
	}
	b.recorder.mu.Unlock()
	return n, err
}

func (a *egressLifeRecorder) RoundTrip(r *http.Request) (*http.Response, error) {
	e := &egressLifeRequest{method: r.Method, path: doctorRequestPath(r.URL.Path), query: r.URL.RawQuery, at: time.Now()}
	if r.Body != nil {
		data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20+1))
		closeErr := r.Body.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(data) > 1<<20 {
			return nil, errors.New("engine request exceeds the recorder's bounded body limit")
		}
		e.body = data
		r.Body = io.NopCloser(bytes.NewReader(data))
	}
	a.mu.Lock()
	path := a.logPath
	a.requests = append(a.requests, e)
	a.mu.Unlock()
	if r.Method == http.MethodDelete && strings.HasPrefix(e.path, "/containers/") && path != "" {
		data, readErr := os.ReadFile(path)
		var mode os.FileMode
		if info, err := os.Lstat(path); err == nil {
			mode = info.Mode()
		}
		a.mu.Lock()
		e.logBefore, e.logErr, e.logMode = data, readErr, mode
		a.mu.Unlock()
	}
	response, err := a.transport.RoundTrip(r)
	a.mu.Lock()
	if response != nil {
		e.status = response.StatusCode
	}
	if err != nil {
		e.finished = time.Now()
	}
	a.mu.Unlock()
	if response != nil {
		response.Body = &egressLifeBody{response.Body, a, e}
	}
	return response, err
}

func (a *egressLifeRecorder) snapshot() []egressLifeRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := make([]egressLifeRequest, 0, len(a.requests))
	for _, request := range a.requests {
		r := *request
		r.body, r.response, r.logBefore = bytes.Clone(r.body), bytes.Clone(r.response), bytes.Clone(r.logBefore)
		result = append(result, r)
	}
	return result
}

func (w *world) egressLifeRecord() (*egressLifeRecorder, error) {
	if err := doctorRealGuard(); err != nil {
		return nil, err
	}
	n, err := w.egressNativeWorld()
	if err != nil {
		return nil, err
	}
	socket := filepath.Join(w.doctor.host, "slice-28-engine.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socket, 0600); err != nil {
		_ = listener.Close()
		return nil, err
	}
	a := &egressLifeRecorder{transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", "/var/run/docker.sock")
	}}}
	proxy := httputil.NewSingleHostReverseProxy(&url.URL{Scheme: "http", Host: "engine"})
	proxy.Transport = a
	a.server = &http.Server{Handler: proxy, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = a.server.Serve(listener) }()
	n.cleanup = append(n.cleanup, func() error {
		err := a.server.Close()
		a.transport.CloseIdleConnections()
		removeErr := os.Remove(socket)
		if errors.Is(removeErr, os.ErrNotExist) {
			removeErr = nil
		}
		return errors.Join(err, removeErr)
	})
	w.doctor.env["DOCKER_HOST"] = "unix://" + socket
	// Consent is still an independently seeded original, bound to the recorder
	// socket and the same real daemon identity before any product invocation.
	if err := w.approvalSeed("1", false); err != nil {
		return nil, err
	}
	return a, w.seedControlLocations()
}

func egressLifeWaitExit(requests []egressLifeRequest, id string, exit int) bool {
	for _, request := range requests {
		if request.path != "/containers/"+id+"/wait" || request.status != http.StatusOK || request.finished.IsZero() {
			continue
		}
		var response struct{ StatusCode *int }
		if request.method == http.MethodPost && json.Unmarshal(request.response, &response) == nil && response.StatusCode != nil && *response.StatusCode == exit {
			return true
		}
	}
	return false
}
