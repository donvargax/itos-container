// Package docker reads the runtime registrations of the caller's native Docker
// daemon through the pinned SDK. A lookup is read-only and makes four daemon
// requests, all bound to the call's context: the readiness engine's ping, which
// negotiates the API version, its GET /version and GET /info, which validate
// the daemon as native, and this adapter's own GET /info, which reports the
// runtimes.
//
// The second GET /info is not a repeat of the first. The readiness port keeps
// only identity and platform fields of the response (its Info type), so the
// validating read cannot carry the Runtimes map, and widening that shared port
// for one reader would change every readiness caller and fake. The runtimes
// are therefore read through the SDK, on a transport that dials only the
// validated socket and refuses to start or to return once the session reports
// the socket changed, so both reads reach the daemon that was validated.
// Whatever fails to read, decode or arrive is an unreadable inventory
// (RuntimeInspect or Cancelled); only a decoded, non-null map without the key
// runsc confirms the runtime's absence.
package docker

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path"
	"slices"
	"strings"

	"github.com/moby/moby/api/types/system"
	"github.com/moby/moby/client"

	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/domain"
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Runtimes inspects the daemon the caller selected. Each lookup opens the
// selection afresh through the readiness engine, which refuses remote,
// untrusted and replaced sockets, then validates the engine as native.
type Runtimes struct {
	Readiness           rp.ContextEngine
	Selection, Checkout string
}

// Runsc reports the registration of the runtime named runsc.
func (r Runtimes) Runsc(ctx context.Context) (port.Registration, port.Failure) {
	if ctx.Err() != nil {
		return port.RegistrationUnknown, port.Cancelled
	}
	if r.Readiness == nil {
		return port.RegistrationUnknown, port.NoProbe
	}
	pin, failure := r.Readiness.OpenContext(ctx, r.Selection, r.Checkout)
	if failure != rp.None {
		return unreadable(ctx)
	}
	defer pin.Close()
	// The session keeps the lifetime its establishment was given, which can
	// outlast this call: only a view bound to the call's context ends with it.
	requests, ok := pin.(rp.ContextSession)
	if !ok {
		return unreadable(ctx)
	}
	inspect := requests.ForContext(ctx)
	version, versionFailure := inspect.Version()
	info, infoFailure := inspect.Info()
	if versionFailure != rp.None || infoFailure != rp.None || readiness.ValidateEngine(version, info) != rp.None {
		return unreadable(ctx)
	}
	socket := strings.TrimPrefix(pin.Endpoint(), "unix://")
	transport := &http.Transport{DisableKeepAlives: true, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		if !pin.Unchanged() {
			return nil, errors.New("engine socket changed")
		}
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	defer transport.CloseIdleConnections()
	api, err := client.New(client.WithHost(pin.Endpoint()), client.WithAPIVersion(version.Negotiated), client.WithHTTPClient(&http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}))
	if err != nil {
		return unreadable(ctx)
	}
	defer func() { _ = api.Close() }()
	result, err := api.Info(ctx, client.InfoOptions{})
	if err != nil || !pin.Unchanged() {
		return unreadable(ctx)
	}
	return registration(result.Info.Runtimes)
}

func unreadable(ctx context.Context) (port.Registration, port.Failure) {
	if ctx.Err() != nil {
		return port.RegistrationUnknown, port.Cancelled
	}
	return port.RegistrationUnknown, port.RuntimeInspect
}

// registration judges the reported inventory. A missing or null map is no
// inventory; an inventory without the exact key runsc confirms its absence.
// The runsc entry must be a path-based registration: Moby reports no
// runtimeType, so a nonempty one is a shim registration this detector cannot
// identify, and an entry without a recognized path says nothing about the
// executable behind the name.
func registration(runtimes map[string]system.RuntimeWithStatus) (port.Registration, port.Failure) {
	if runtimes == nil {
		return port.RegistrationUnknown, port.RuntimeInspect
	}
	entry, ok := runtimes["runsc"]
	if !ok {
		return port.RunscAbsent, port.None
	}
	if entry.Type != "" || !runscPath(entry.Path) || !hostSocketsUnreachable(entry.Args) {
		return port.RegistrationUnknown, port.RuntimeRegistration
	}
	return port.RunscRegistered, port.None
}

// runscPath accepts the bare name or an absolute, canonical Linux path whose
// final component is runsc.
func runscPath(p string) bool {
	return p == "runsc" || (strings.HasPrefix(p, "/") && path.Clean(p) == p && path.Base(p) == "runsc")
}

// hostSocketsUnreachable inspects the complete runtimeArgs list. An empty list
// leaves runsc's default host-uds policy of none, and the exact default flag
// restates it. Any other list either permits host sockets or changes the
// policy in a way the detector cannot establish.
func hostSocketsUnreachable(args []string) bool {
	return len(args) == 0 || slices.Equal(args, []string{"--host-uds=none"})
}
