// Package docker adapts the pinned Docker SDK to inspection-only capabilities.
package docker

import (
	"context"
	"errors"
	budgetdomain "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/client"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Engine uses explicitly supplied endpoint selection, never Docker configuration,
// environment options, credential helpers, registries, or command subprocesses.
type Engine struct{}

// Open validates and pins the socket before negotiating the API. The deadline
// belongs to the entire session and is not renewed by individual requests.
func (Engine) Open(selection, checkout string, budget time.Duration) (port.Session, port.Failure) {
	return openEngine(selection, checkout, budget, client.New)
}

func openEngine(selection, checkout string, budget time.Duration, newClient func(...client.Opt) (*client.Client, error)) (port.Session, port.Failure) {
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	return openContext(ctx, ctx, cancel, selection, checkout, newClient)
}

func (Engine) OpenContext(call context.Context, selection, checkout string) (port.Session, port.Failure) {
	lifetime, cancel := context.WithCancel(budgetdomain.Lifetime(call))
	return openContext(call, lifetime, cancel, selection, checkout, client.New)
}

func openContext(call, ctx context.Context, cancel context.CancelFunc, selection, checkout string, newClient func(...client.Opt) (*client.Client, error)) (port.Session, port.Failure) {
	success := false
	defer func() {
		if !success {
			cancel()
		}
	}()
	path, failure := socketPath(selection, checkout)
	if failure != port.None {
		return nil, failure
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return nil, port.EngineUnavailable
	}
	transport := &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", path)
	}}
	guard := &onceFailed{base: transport}
	api, err := newClient(client.WithHost("unix://"+path), client.WithHTTPClient(&http.Client{
		Transport:     guard,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}))
	if err != nil {
		cancel()
		transport.CloseIdleConnections()
		return nil, port.EngineSelection
	}
	s := &session{api: api, ctx: ctx, cancel: cancel, transport: transport, path: path, selected: selection, checkout: checkout, socket: info}
	_, err = api.Ping(call, client.PingOptions{NegotiateAPIVersion: true})
	if err != nil {
		failure = s.failure(err, port.EngineInspect)
		if guard.httpFailure {
			failure = port.EngineInspect
		}
		if errdefs.IsInvalidArgument(err) {
			failure = port.EngineAPI
		}
		s.Close()
		return nil, failure
	}
	if !s.Unchanged() {
		s.Close()
		return nil, port.EngineChanged
	}
	success = true
	return s, port.None
}

func socketPath(selection, checkout string) (string, port.Failure) {
	if selection == "" {
		selection = "unix:///var/run/docker.sock"
	}
	scheme, _, hasScheme := strings.Cut(selection, "://")
	if hasScheme && scheme != "unix" {
		return "", port.EngineRemote
	}
	u, err := url.Parse(selection)
	if err != nil || !hasScheme || u.Scheme != "unix" || u.Host != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(selection, "#") || u.Opaque != "" || !filepath.IsAbs(u.Path) {
		return "", port.EngineSelection
	}
	path, err := filepath.EvalSymlinks(u.Path)
	if err != nil {
		return "", port.EngineUnavailable
	}
	relative, err := filepath.Rel(checkout, path)
	if err != nil {
		return "", port.EngineSelection
	}
	if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return "", port.EngineUntrusted
	}
	return path, port.None
}

type session struct {
	api                      *client.Client
	ctx                      context.Context
	cancel                   context.CancelFunc
	transport                *http.Transport
	path, selected, checkout string
	socket                   fs.FileInfo
}

func (s *session) Endpoint() string { return "unix://" + s.path }

func (s *session) Version() (port.Version, port.Failure) {
	v, err := s.api.ServerVersion(s.ctx, client.ServerVersionOptions{})
	if err != nil {
		return port.Version{}, s.failure(err, port.EngineInspect)
	}
	return port.Version{Product: v.Platform.Name, OS: v.Os, Architecture: v.Arch, Minimum: v.MinAPIVersion, Maximum: v.APIVersion, Negotiated: s.api.ClientVersion(), SDKMinimum: client.MinAPIVersion, SDKMaximum: client.MaxAPIVersion}, port.None
}

func (s *session) Info() (port.Info, port.Failure) {
	result, err := s.api.Info(s.ctx, client.InfoOptions{})
	if err != nil {
		return port.Info{}, s.failure(err, port.EngineInspect)
	}
	i := result.Info
	return port.Info{ID: i.ID, OS: i.OSType, Architecture: i.Architecture, OperatingSystem: i.OperatingSystem}, port.None
}

func (s *session) Image(reference string) (port.Image, port.Failure) {
	i, err := s.api.ImageInspect(s.ctx, reference)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return port.Image{}, port.ImageMissing
		}
		return port.Image{}, s.failure(err, port.ImageInspect)
	}
	image := port.Image{ID: i.ID, OS: i.Os, Architecture: i.Architecture, RepoDigests: i.RepoDigests, ConfigPresent: i.Config != nil}
	if i.Config != nil {
		image.User = i.Config.User
		image.Label.Text, image.Label.Present = i.Config.Labels["devcontainer.metadata"]
	}
	return image, port.None
}

func (s *session) Unchanged() bool {
	path, failure := socketPath(s.selected, s.checkout)
	if failure != port.None || path != s.path {
		return false
	}
	current, err := os.Stat(path)
	return err == nil && os.SameFile(s.socket, current)
}

func (s *session) Close() { s.cancel(); _ = s.api.Close(); s.transport.CloseIdleConnections() }

func (s *session) failure(err error, otherwise port.Failure) port.Failure {
	if errors.Is(s.ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return port.EngineTimeout
	}
	if client.IsErrConnectionFailed(err) {
		return port.EngineUnavailable
	}
	return otherwise
}

// The SDK's ping can retry a failed HEAD as GET. Keep that compatibility
// fallback from issuing another request after any failure. Disabling idle
// connection reuse also prevents net/http's transparent stale-connection retry.
type onceFailed struct {
	base        *http.Transport
	failure     error
	httpFailure bool
}

func (t *onceFailed) RoundTrip(request *http.Request) (*http.Response, error) {
	if t.failure != nil {
		return nil, t.failure
	}
	response, err := t.base.RoundTrip(request)
	if err != nil {
		t.failure = err
	} else if response.StatusCode >= 300 {
		t.failure = errors.New("engine request failed")
		t.httpFailure = true
	}
	return response, err
}

// A request view shares the pinned socket/client, but never owns their closure.
func (s *session) ForContext(ctx context.Context) port.Session {
	view := *s
	view.ctx = ctx
	view.cancel = func() {}
	return &view
}
