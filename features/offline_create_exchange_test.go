package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// One authorized POST /containers/create the recorder forwarded. Its engine-side
// exchange outlives the controller: the engine finishes a create it has accepted
// whether or not anyone reads the answer, so the recorder reads it for everyone.
type offlineCreate struct {
	name   string        // the authorized name, itos-container-<token>
	labels any           // the request's labels
	done   chan struct{} // closed once the exchange ended and its outcome is recorded

	// Under the recorder lock.
	id        string // the listed worker, when the engine answered 201 with a full ID
	expired   bool   // the exchange ended at engineCreateOutcomeAllowance
	failed    bool   // the exchange ended in an engine-side failure
	abandoned bool   // a plain fixture's controller left before the reply

	// Written before done closes and read only after it.
	status  int
	header  http.Header
	body    []byte
	failure error
	bare    bool // 201 without a full ID: relayed with no barrier and no reply time
}

var (
	errOfflineCreateAbandoned = errors.New("offline native recorder create abandoned: the controller left before the reply")
	errOfflineTransport       = errors.New("offline native recorder transport failed")
	offlineCreateExpired      = fmt.Sprintf("recorder create outcome allowance expired: no engine answer within %s of forwarding", engineCreateOutcomeAllowance)
	// What one hop may not pass to the next. A create is a plain request, so the
	// headers a Connection field could name besides these do not occur.
	offlineHopHeaders = map[string]bool{"Connection": true, "Proxy-Connection": true, "Keep-Alive": true, "Proxy-Authenticate": true,
		"Proxy-Authorization": true, "Te": true, "Trailer": true, "Transfer-Encoding": true, "Upgrade": true}
)

// forwardCreate replaces the reverse proxy for the one request whose answer the
// harness may not lose. A connected controller sees what the proxy gave it: the
// same reply, after the same recording and inspection, behind the same barriers.
func (a *offlineAPI) forwardCreate(out http.ResponseWriter, request *http.Request, entry offlineRequest, index int, body []byte) {
	x := &offlineCreate{name: request.URL.Query().Get("name"), labels: entry.Body["Labels"], done: make(chan struct{})}
	// Nothing on the controller side ends the engine side: not a cancelled or
	// closed request, a killed controller, a lifecycle disconnect or this server
	// closing. Only the engine's answer or the allowance does.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(request.Context()), engineCreateOutcomeAllowance)
	outbound := request.Clone(ctx)
	outbound.URL.Scheme, outbound.URL.Host, outbound.Host, outbound.RequestURI = "http", "engine", "engine", ""
	// The body was read whole to journal it; send exactly those bytes.
	outbound.Body, outbound.ContentLength, outbound.GetBody = io.NopCloser(bytes.NewReader(body)), int64(len(body)), nil
	outbound.TransferEncoding, outbound.Trailer = nil, nil
	for name := range offlineHopHeaders {
		outbound.Header.Del(name)
	}
	a.mu.Lock()
	a.creates = append(a.creates, x)
	a.requests[index].Forwarded = time.Now()
	a.mu.Unlock()
	go func() {
		defer close(x.done)
		defer cancel()
		a.createOutcome(x, outbound, request, index)
	}()
	select {
	case <-x.done:
	case <-request.Context().Done():
	}
	if request.Context().Err() != nil {
		// The controller left, or a lifecycle disconnect cancelled its request while
		// its connection may still be open: that one reads the transport failure,
		// never an empty or successful reply. The answer, when it comes, is recorded,
		// holds nothing and reaches no barrier.
		a.createAbandoned(x, index)
		http.Error(out, "offline fixture transport failed", http.StatusBadGateway)
		return
	}
	if x.failure != nil {
		http.Error(out, "offline fixture transport failed", http.StatusBadGateway)
		return
	}
	if !x.bare {
		a.anchorsResponse(request, entry, x.status)
		if a.lifecycle != nil {
			a.lifecycle.response(request, entry, x.status)
		}
		a.mu.Lock()
		a.requests[index].Cancelled = request.Context().Err() != nil
		if !a.requests[index].Cancelled {
			a.requests[index].Replied = time.Now()
		}
		a.mu.Unlock()
	}
	header := out.Header()
	for name, values := range x.header {
		if !offlineHopHeaders[name] {
			header[name] = values
		}
	}
	header.Set("Content-Length", strconv.Itoa(len(x.body)))
	out.WriteHeader(x.status)
	_, _ = out.Write(x.body)
}

// createOutcome reads the engine's answer and records it once, whether or not
// the controller is still there.
func (a *offlineAPI) createOutcome(x *offlineCreate, outbound, request *http.Request, index int) {
	response, err := a.w.doctor.real.transport.RoundTrip(outbound)
	var data []byte
	if err == nil {
		data, err = io.ReadAll(io.LimitReader(response.Body, 4<<20))
		_ = response.Body.Close()
	}
	answered := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	if err != nil {
		x.failure, x.failed = err, true
		a.requests[index].Status = http.StatusBadGateway
		a.requests[index].Diagnostic = "recorder transport: " + err.Error()
		if errors.Is(outbound.Context().Err(), context.DeadlineExceeded) {
			x.expired = true
			a.requests[index].Diagnostic = offlineCreateExpired
		}
		// As for every engine-side failure: a recovery fixture reports none for a
		// request its controller abandoned, a lifecycle fixture none at all.
		if a.lifecycle == nil && (!a.recovery || request.Context().Err() == nil) {
			a.problem = errOfflineTransport
			if x.abandoned {
				a.problem = errors.Join(errOfflineCreateAbandoned, errOfflineTransport)
			}
		}
		return
	}
	x.status, x.header, x.body = response.StatusCode, response.Header, data
	a.requests[index].Answered, a.requests[index].Status = answered, response.StatusCode
	var observed map[string]any
	_ = json.Unmarshal(data, &observed)
	if response.StatusCode >= http.StatusBadRequest {
		message, _ := observed["message"].(string)
		a.requests[index].Diagnostic = "engine response: " + message
	}
	if response.StatusCode == http.StatusCreated {
		id, _ := observed["Id"].(string)
		if !offlineHex(id) {
			a.problem = errors.New("native create reply lacks a full container ID")
			x.bare = true
			return
		}
		a.created = append(a.created, id)
		a.identities[id] = map[string]any{"Name": "/" + x.name, "Labels": x.labels}
		x.id = id
		// A create reply alone never counts as observed ownership.
		if observed, err = a.inspect(id); err != nil {
			a.problem = err
		}
	}
	if observed != nil {
		a.requests[index].Observation = observed
		if state, ok := observed["State"].(map[string]any); ok {
			a.requests[index].State = state
		}
	}
}

// A plain fixture, neither recovery nor lifecycle, hands the abandonment to its
// first Then through observeAfter, under words of its own: the transport did
// not fail. The other fixtures set no problem for an abandoned request; their
// barrier diagnostics carry the report.
func (a *offlineAPI) createAbandoned(x *offlineCreate, index int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.requests[index].Cancelled = true
	if a.lifecycle != nil || a.recovery {
		return
	}
	x.abandoned = true
	a.problem = errOfflineCreateAbandoned
	if x.failed {
		a.problem = errors.Join(errOfflineCreateAbandoned, errOfflineTransport)
	}
}

// awaitCreates waits for every create this recorder forwarded whose engine-side
// exchange is still open. It holds no recorder lock while it waits, since the
// answer is recorded under it, and each wait ends with its exchange: at the
// engine's answer or at the allowance.
func (a *offlineAPI) awaitCreates() {
	a.mu.Lock()
	open := make([]<-chan struct{}, 0, len(a.creates))
	for _, x := range a.creates {
		open = append(open, x.done)
	}
	a.mu.Unlock()
	for _, done := range open {
		<-done
	}
}

// awaitCreateOutcomes is the step wait: every native recorder the scenario
// started, those of its control, second and overlap worlds included. No Then
// judges an open exchange, so a late answer is in the recorder's lists and
// journal before the next step reads them.
func (w *world) awaitCreateOutcomes() {
	seen := map[*world]bool{}
	var visit func(*world)
	visit = func(w *world) {
		if w == nil || seen[w] {
			return
		}
		seen[w] = true
		if w.offline != nil && w.offline.api != nil {
			w.offline.api.awaitCreates()
		}
		if w.recovery != nil {
			visit(w.recovery.second)
		}
		if w.anchor != nil {
			visit(w.anchor.control)
		}
		if w.lifecycle != nil {
			visit(w.lifecycle.control)
		}
		if w.overlap != nil {
			for _, p := range w.overlap.worlds {
				visit(p)
			}
		}
	}
	visit(w)
}
