package command

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/egress/port"
	"github.com/donvargax/itos-container/internal/engine/egress/port/porttest"
)

func TestProxyHandlerPreservesEveryNameAndOnlyRecordsOnStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	p := porttest.NewProxy(porttest.ProxyFixture{Log: &stdout})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	names := []string{"one.example", "xn--bcher-kva.example", "two.example", "", "--literal"}
	code := (CLI{Args: append([]string{"--"}, names...)}).Run(ctx, p, &stderr)
	var record port.Record
	if code != 0 || stderr.Len() != 0 || !slices.Equal(p.Names(), names) || json.Unmarshal(stdout.Bytes(), &record) != nil || record.Event != port.Ready {
		t.Fatalf("proxy handler: %d, names %v, stdout %q, stderr %q", code, p.Names(), stdout.String(), stderr.String())
	}
}

func TestProxyHandlerRefusesBadInvocationAndServeFailure(t *testing.T) {
	for _, args := range [][]string{nil, {"--"}, {"one.example"}} {
		var stdout, stderr bytes.Buffer
		p := porttest.NewProxy(porttest.ProxyFixture{Log: &stdout})
		if code := (CLI{Args: args}).Run(t.Context(), p, &stderr); code != 1 || len(p.Names()) != 0 || stdout.Len() != 0 || stderr.String() != string(port.ListInvalid)+"\n" {
			t.Fatalf("bad invocation: %d %q", code, stderr.String())
		}
	}
	for _, failure := range []port.Failure{port.ListInvalid, port.LogFailed} {
		var stderr bytes.Buffer
		p := porttest.NewProxy(porttest.ProxyFixture{Failure: failure, Log: porttest.FailingLog{}})
		if code := (CLI{Args: []string{"--", "one.example"}}).Run(t.Context(), p, &stderr); code != 1 || stderr.String() != string(failure)+"\n" {
			t.Fatalf("Serve failure: %d %q", code, stderr.String())
		}
	}
}

func TestProxyHandlerDoesNotPrintUnknownErrorPayload(t *testing.T) {
	var stdout, stderr bytes.Buffer
	p := porttest.NewProxy(porttest.ProxyFixture{Log: &stdout, Failure: port.Failure("private untrusted error\x1b[31m")})
	if code := (CLI{Args: []string{"--", "one.example"}}).Run(t.Context(), p, &stderr); code != 1 || stdout.Len() != 0 || stderr.String() != string(port.AcceptFailed)+"\n" {
		t.Fatalf("unsafe proxy diagnostic: %d %q %q", code, stdout.String(), stderr.String())
	}
}
