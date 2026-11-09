package porttest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/donvargax/itos-container/internal/engine/egress/port"
)

// Reporter is the test output used by the shared contract. A recording fake lets
// its tests verify that faulty proxies really trigger each assertion.
type Reporter interface {
	Helper()
	Errorf(string, ...any)
}

// ProxyContract runs the same port assertions against the fake and the adapter.
// A valid list writes exactly one ready record, Serve stays in until ctx ends and
// returns nil, and nothing follows. Each invalid list returns ListInvalid with no
// record, and a failing log returns LogFailed.
func ProxyContract(t Reporter, open func(ProxyFixture) port.Proxy) {
	t.Helper()
	proxyContract(t, 30*time.Second, 100*time.Millisecond, open)
}

// proxyContract waits up to wait for anything it expects to happen, and settle
// for anything it expects not to.
func proxyContract(t Reporter, wait, settle time.Duration, open func(ProxyFixture) port.Proxy) {
	t.Helper()
	valid := []string{"listed.example"}
	serveValid(t, wait, settle, open, valid)
	for _, names := range [][]string{
		nil, {}, {"LISTED.example"}, {"listed.example."}, {"127.0.0.1"}, {"listed.example", "listed.example"},
	} {
		log := NewLog()
		err := serveUntilDone(t, wait, open(ProxyFixture{Names: names, Log: log, Failure: port.ListInvalid}), names)
		if err != port.ListInvalid {
			t.Errorf("Serve(%q) = %v; want %v", names, err, port.ListInvalid)
		}
		if lines := log.Lines(); len(lines) != 0 {
			t.Errorf("Serve(%q) wrote %d records for an invalid list", names, len(lines))
		}
	}
	err := serveUntilDone(t, wait, open(ProxyFixture{Names: valid, Log: FailingLog{}, Failure: port.LogFailed}), valid)
	if err != port.LogFailed {
		t.Errorf("Serve with a failing log = %v; want %v", err, port.LogFailed)
	}
}

// serveUntilDone returns what Serve returns, or nil after reporting that it did
// not return in time.
func serveUntilDone(t Reporter, wait time.Duration, proxy port.Proxy, names []string) error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- proxy.Serve(ctx, names) }()
	select {
	case err := <-done:
		return err
	case <-time.After(wait):
		t.Errorf("Serve(%q) did not return for a list or log it must fail", names)
		cancel()
		return nil
	}
}

func serveValid(t Reporter, wait, settle time.Duration, open func(ProxyFixture) port.Proxy, names []string) {
	t.Helper()
	log := NewLog()
	proxy := open(ProxyFixture{Names: names, Log: log})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- proxy.Serve(ctx, names) }()
	deadline := time.Now().Add(wait)
	for len(log.Lines()) == 0 {
		select {
		case err := <-done:
			t.Errorf("Serve returned %v before it wrote a ready record", err)
			return
		default:
		}
		if time.Now().After(deadline) {
			t.Errorf("Serve wrote no ready record within %v", wait)
			cancel()
			<-done
			return
		}
		time.Sleep(time.Millisecond)
	}
	if lines := log.Lines(); len(lines) != 1 {
		t.Errorf("Serve wrote %d records at start; want exactly one ready record", len(lines))
	} else {
		checkReady(t, lines[0])
	}
	select {
	case err := <-done:
		t.Errorf("Serve returned %v before ctx ended", err)
		return
	case <-time.After(settle):
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve = %v after ctx ended; want nil", err)
		}
	case <-time.After(wait):
		t.Errorf("Serve did not return after ctx ended")
		return
	}
	time.Sleep(settle)
	if lines := log.Lines(); len(lines) != 1 {
		t.Errorf("Serve wrote %d records in all; a record followed its return or ready came twice", len(lines))
	}
}

// checkReady holds the ready record to its exact bytes: schema 1, seq 1, an
// RFC 3339 UTC time, event ready, nothing more, one final newline.
func checkReady(t Reporter, line []byte) {
	t.Helper()
	var record port.Record
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		t.Errorf("ready record %q is not one JSON object: %v", line, err)
		return
	}
	parsed, err := time.Parse(time.RFC3339, record.Time)
	if err != nil || !strings.HasSuffix(record.Time, "Z") || parsed.UTC().Format(time.RFC3339) != record.Time {
		t.Errorf("ready record time %q is not RFC 3339 UTC to the second", record.Time)
	}
	want := fmt.Sprintf("{\"schema\":1,\"seq\":1,\"time\":%q,\"event\":\"ready\"}\n", record.Time)
	if string(line) != want {
		t.Errorf("ready record = %q; want %q", line, want)
	}
}
