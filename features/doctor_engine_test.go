package features

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type doctorRequest struct {
	Method, Path, Query string
	PID                 int
}
type runPeerKey struct{}

type doctorListener struct {
	net.Listener
	connections atomic.Int64
}

func (l *doctorListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err == nil {
		l.connections.Add(1)
	}
	return conn, err
}

type doctorAPI struct {
	mu                       sync.Mutex
	socket                   string
	server                   *http.Server
	listener                 *doctorListener
	extras                   []net.Listener
	journal                  []doctorRequest
	version, info, image     map[string]any
	imageStatus              int
	failure                  string
	trap                     *httptest.Server
	trapConnections          atomic.Int64
	paused                   bool
	reached, release         chan struct{}
	reachedOnce, releaseOnce sync.Once
	forward                  http.RoundTripper // nil for local fixtures; CI-only recorder uses it
	expectedImage            string
	runBarrier               func(int)
	offline                  *localOfflineAPI
}

func newDoctorAPI(image string) *doctorAPI {
	return &doctorAPI{
		expectedImage: image,
		version:       map[string]any{"Version": "29.5.0", "ApiVersion": "1.56", "MinAPIVersion": "1.44", "Os": "linux", "Arch": "amd64", "Platform": map[string]string{"Name": "Docker Engine - Community"}, "Components": []any{map[string]any{"Name": "Engine", "Version": "29.5.0"}}},
		info:          map[string]any{"ID": "engine-A", "OSType": "linux", "Architecture": "x86_64", "OperatingSystem": "Debian GNU/Linux 12", "ServerVersion": "29.5.0", "Name": "fixture-host"},
		image:         map[string]any{"Id": doctorImageID, "RepoDigests": []string{image}, "Os": "linux", "Architecture": "amd64", "Config": map[string]any{"User": "1000:1000", "Labels": map[string]string{}, "Volumes": map[string]any{}}},
		reached:       make(chan struct{}), release: make(chan struct{}),
	}
}

func (a *doctorAPI) config() map[string]any { return a.image["Config"].(map[string]any) }

func (w *world) doctorLocalAPI(paused bool) error {
	a := newDoctorAPI(w.doctor.image)
	a.paused = paused
	if err := w.startDoctorAPI(a); err != nil {
		return err
	}
	w.doctor.engine = a
	w.doctor.env["DOCKER_HOST"] = "unix://" + a.socket
	return nil
}

func (w *world) startDoctorAPI(a *doctorAPI) error {
	// Keep Unix paths short even under a long TMPDIR or Go test name.
	dir, err := os.MkdirTemp("/tmp", "slice-3-api-")
	if err != nil {
		return err
	}
	w.doctor.cleanup = append(w.doctor.cleanup, func() { _ = os.RemoveAll(dir) })
	a.socket = filepath.Join(dir, "engine.sock")
	listener, err := net.Listen("unix", a.socket)
	if err != nil {
		return err
	}
	a.listener = &doctorListener{Listener: listener}
	a.server = &http.Server{Handler: http.HandlerFunc(a.serve), ReadHeaderTimeout: time.Second, ConnContext: func(ctx context.Context, conn net.Conn) context.Context {
		return context.WithValue(ctx, runPeerKey{}, runPeerPID(conn))
	}}
	go func() { _ = a.server.Serve(a.listener) }()
	return nil
}

var doctorAPIVersionPath = regexp.MustCompile(`^/v[0-9]+\.[0-9]+/`)

func doctorRequestPath(path string) string { return doctorAPIVersionPath.ReplaceAllString(path, "/") }

func doctorAllowedRequest(r doctorRequest, image string, allowImage bool) bool {
	path := doctorRequestPath(r.Path)
	if r.Query != "" {
		return false
	}
	if path == "/_ping" {
		return r.Method == http.MethodGet || r.Method == http.MethodHead
	}
	if r.Method != http.MethodGet {
		return false
	}
	return path == "/version" || path == "/info" || (allowImage && path == "/images/"+image+"/json")
}

func (a *doctorAPI) serve(out http.ResponseWriter, r *http.Request) {
	pid, _ := r.Context().Value(runPeerKey{}).(int)
	record := doctorRequest{r.Method, r.URL.Path, r.URL.RawQuery, pid}
	a.mu.Lock()
	a.journal = append(a.journal, record)
	offline := a.offline
	a.mu.Unlock()
	if offline != nil && offline.serve(out, r) {
		return
	}
	if !doctorAllowedRequest(record, a.expectedImage, true) {
		http.Error(out, "fixture forbids this request", http.StatusForbidden)
		return
	}
	if a.forward != nil {
		a.forwardInspection(out, r)
		return
	}
	path := doctorRequestPath(r.URL.Path)
	a.mu.Lock()
	failure := a.failure
	var data any
	switch path {
	case "/version":
		data = a.version
	case "/info":
		data = a.info
	default:
		data = a.image
	}
	body, err := json.Marshal(data)
	status := a.imageStatus
	api, _ := a.version["ApiVersion"].(string)
	a.mu.Unlock()
	if err != nil {
		http.Error(out, "fixture encoding failed", 500)
		return
	}
	if (path == "/_ping" && failure == "never completes ping") || (path == "/info" && failure == "never completes info") || (strings.HasPrefix(path, "/images/") && failure == "never completes the image response") {
		select {
		case <-r.Context().Done():
		case <-a.release:
		}
		return
	}
	if path == "/_ping" {
		out.Header().Set("API-Version", api)
		out.Header().Set("Docker-Experimental", "false")
		out.Header().Set("OSType", "linux")
		_, _ = out.Write([]byte("OK"))
		return
	}
	if path == "/info" {
		switch failure {
		case "returns HTTP 500 from info":
			http.Error(out, "fixture failure "+doctorCanary, 500)
			return
		case "returns truncated JSON from info":
			_, _ = out.Write([]byte(`{"ID":`))
			return
		case "returns a redirect to a trap endpoint":
			http.Redirect(out, r, a.trap.URL, http.StatusTemporaryRedirect)
			return
		}
	}
	if strings.HasPrefix(path, "/images/") {
		if a.runBarrier != nil {
			a.runBarrier(pid)
		}
		if a.paused {
			a.reachedOnce.Do(func() { close(a.reached) })
			select {
			case <-a.release:
			case <-r.Context().Done():
				return
			}
		}
		switch failure {
		case "malformed JSON":
			_, _ = out.Write([]byte(`{"Id":` + doctorCanary))
			return
		}
		if status != 0 {
			out.WriteHeader(status)
			_, _ = out.Write([]byte(`{"message":"fixture failure"}`))
			return
		}
	}
	out.Header().Set("Content-Type", "application/json")
	_, _ = out.Write(body)
}

func (a *doctorAPI) unpause() { a.releaseOnce.Do(func() { close(a.release) }) }
func (a *doctorAPI) close() {
	a.unpause()
	if a.server != nil {
		_ = a.server.Close()
	}
	for _, l := range a.extras {
		_ = l.Close()
	}
	if a.trap != nil {
		a.trap.Close()
	}
}
func (a *doctorAPI) requests() []doctorRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]doctorRequest{}, a.journal...)
}
func (a *doctorAPI) replaceSocket() error {
	if err := os.Rename(a.socket, a.socket+".before"); err != nil {
		return err
	}
	listener, err := net.Listen("unix", a.socket)
	if err != nil {
		return err
	}
	a.extras = append(a.extras, listener)
	go func() { _ = a.server.Serve(listener) }()
	return nil
}

func (w *world) doctorObservation(observation string) error {
	if err := w.doctorLocalAPI(false); err != nil {
		return err
	}
	a := w.doctor.engine
	switch observation {
	case "empty daemon ID":
		a.info["ID"] = ""
	case "server maximum below SDK minimum":
		a.version["ApiVersion"] = "1.24"
		a.version["MinAPIVersion"] = "1.24"
	case "server minimum above SDK maximum":
		a.version["ApiVersion"] = "9.99"
		a.version["MinAPIVersion"] = "9.98"
	case "Docker Desktop product":
		a.version["Platform"] = map[string]string{"Name": "Docker Desktop"}
		a.info["OperatingSystem"] = "Docker Desktop"
	case "Rancher Desktop product":
		a.version["Platform"] = map[string]string{"Name": "Rancher Desktop"}
		a.info["OperatingSystem"] = "Rancher Desktop"
	case "Podman product":
		a.version["Platform"] = map[string]string{"Name": "Podman Engine"}
	case "unknown product":
		a.version["Platform"] = map[string]string{"Name": "unknown engine"}
	case "server OS windows":
		a.version["Os"] = "windows"
		a.info["OSType"] = "windows"
	case "server architecture arm64":
		a.version["Arch"] = "arm64"
		a.info["Architecture"] = "aarch64"
	case "version and info disagree on platform":
		a.info["Architecture"] = "aarch64"
	default:
		return fmt.Errorf("unknown engine observation %q", observation)
	}
	return nil
}

func (w *world) doctorFailure(failure string) error {
	if err := w.doctorLocalAPI(false); err != nil {
		return err
	}
	a := w.doctor.engine
	a.failure = failure
	if failure == "returns a redirect to a trap endpoint" {
		a.trap = httptest.NewUnstartedServer(http.HandlerFunc(func(out http.ResponseWriter, _ *http.Request) { out.WriteHeader(500) }))
		a.trap.Config.ConnState = func(_ net.Conn, state http.ConnState) {
			if state == http.StateNew {
				a.trapConnections.Add(1)
			}
		}
		a.trap.Start()
	}
	return nil
}

func (w *world) doctorDigests(kind string) error {
	if err := w.doctorLocalAPI(false); err != nil {
		return err
	}
	var digests []string
	switch kind {
	case "empty":
		digests = []string{}
	case "same repository with a different digest":
		digests = []string{"worker.test/agent@sha256:" + strings.Repeat("c", 64)}
	case "same digest under a different repository only":
		digests = []string{"worker.test/other@sha256:" + strings.Repeat("a", 64)}
	case "image config ID instead of the requested RepoDigest":
		digests = []string{doctorImageID}
	default:
		return fmt.Errorf("unknown digest fixture %q", kind)
	}
	w.doctor.engine.image["RepoDigests"] = digests
	return nil
}
func (w *world) doctorPlatform(platform string) error {
	if err := w.doctorLocalAPI(false); err != nil {
		return err
	}
	image := w.doctor.engine.image
	switch platform {
	case "linux/arm64":
		image["Architecture"] = "arm64"
	case "windows/amd64":
		image["Os"] = "windows"
	case "missing OS":
		delete(image, "Os")
	case "missing arch":
		delete(image, "Architecture")
	default:
		return fmt.Errorf("unknown image platform %q", platform)
	}
	return nil
}
func (w *world) doctorImageResponse(response string) error {
	if err := w.doctorLocalAPI(false); err != nil {
		return err
	}
	a := w.doctor.engine
	switch response {
	case "malformed JSON":
		a.failure = response
	case "missing image ID":
		delete(a.image, "Id")
	case "invalid image ID":
		a.image["Id"] = "invalid"
	case "missing Config":
		delete(a.image, "Config")
	case "HTTP 500":
		a.imageStatus = 500
	default:
		return fmt.Errorf("unknown image response %q", response)
	}
	return nil
}
func (w *world) doctorCanaries(location string) error {
	switch location {
	case "HTTP 500 error body":
		return w.doctorFailure("returns HTTP 500 from info")
	case "malformed image response body":
		return w.doctorImageResponse("malformed JSON")
	}
	if err := w.doctorLocalAPI(false); err != nil {
		return err
	}
	a := w.doctor.engine
	switch location {
	case "image environment and unrelated labels":
		a.config()["Env"] = []string{"SYNTHETIC=" + doctorCanary}
		a.config()["Labels"] = map[string]string{"fixture.canary": doctorCanary}
	case "engine info proxy and warning fields":
		a.info["HttpProxy"] = doctorCanary
		a.info["HttpsProxy"] = doctorCanary
		a.info["Warnings"] = []string{doctorCanary}
	default:
		return fmt.Errorf("unknown canary fixture %q", location)
	}
	return nil
}

func (w *world) doctorEndpoint(selection string) error {
	if err := w.doctorSentinelsPortable(); err != nil {
		return err
	}
	a := newDoctorAPI(w.doctor.image)
	// A portable loopback trap also observes incomplete TLS handshakes.
	a.trap = httptest.NewUnstartedServer(http.HandlerFunc(func(out http.ResponseWriter, _ *http.Request) { out.WriteHeader(500) }))
	a.trap.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			a.trapConnections.Add(1)
		}
	}
	a.trap.Start()
	w.doctor.engine = a
	host := strings.TrimPrefix(a.trap.URL, "http://")
	switch selection {
	case "tcp://127.0.0.1:2375":
		selection = "tcp://" + host
	case "ssh://engine.test":
		selection = "ssh://" + host
	case "https://engine.test:2376":
		selection = "https://" + host
	case "npipe:////./pipe/docker_engine":
	case "unix://relative.sock":
	case "unix:///tmp/engine.sock?mode=test", "unix:///tmp/engine.sock#fragment":
		if err := w.startDoctorAPI(a); err != nil {
			return err
		}
		suffix := "?mode=test"
		if strings.Contains(selection, "#") {
			suffix = "#fragment"
		}
		selection = "unix://" + a.socket + suffix
	case "checkout-owned socket", "outside alias to checkout socket":
		// A short checkout path avoids Unix sockaddr limits in Go test's temp tree.
		path := filepath.Join(w.doctor.checkout, "engine.sock")
		l, err := net.Listen("unix", path)
		if err != nil {
			return err
		}
		a.socket = path
		a.listener = &doctorListener{Listener: l}
		a.server = &http.Server{Handler: http.HandlerFunc(a.serve), ReadHeaderTimeout: time.Second, ConnContext: func(ctx context.Context, conn net.Conn) context.Context {
			return context.WithValue(ctx, runPeerKey{}, runPeerPID(conn))
		}}
		go func() { _ = a.server.Serve(a.listener) }()
		if selection == "outside alias to checkout socket" {
			path = filepath.Join(w.dir, "engine-alias.sock")
			if err := os.Symlink(a.socket, path); err != nil {
				return err
			}
		}
		selection = "unix://" + path
	default:
		return fmt.Errorf("unknown endpoint fixture %q", selection)
	}
	w.doctor.env["DOCKER_HOST"] = selection
	return nil
}

func (w *world) doctorRemoteCanary() error {
	if err := w.doctorEndpoint("https://engine.test:2376"); err != nil {
		return err
	}
	host := strings.TrimPrefix(w.doctor.engine.trap.URL, "http://")
	w.doctor.env["DOCKER_HOST"] = "https://" + doctorCanary + ":" + doctorCanary + "@" + host + "?token=" + doctorCanary
	return nil
}

func (w *world) doctorSocket(state string) error {
	dir, err := os.MkdirTemp("/tmp", "slice-3-socket-")
	if err != nil {
		return err
	}
	w.doctor.cleanup = append(w.doctor.cleanup, func() { _ = os.Chmod(dir, 0700); _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "engine.sock")
	switch state {
	case "absent":
	case "a regular file":
		if err := os.WriteFile(path, []byte("not a socket"), 0600); err != nil {
			return err
		}
	case "a socket without listener":
		l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
		if err != nil {
			return err
		}
		l.SetUnlinkOnClose(false)
		if err := l.Close(); err != nil {
			return err
		}
	case "permission denied":
		l, err := net.Listen("unix", path)
		if err != nil {
			return err
		}
		w.doctor.cleanup = append(w.doctor.cleanup, func() { _ = l.Close() })
		if err := os.Chmod(dir, 0000); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown socket fixture %q", state)
	}
	w.doctor.env["DOCKER_HOST"] = "unix://" + path
	return nil
}

func doctorTransport(socket string) *http.Transport {
	return &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socket)
	}}
}
