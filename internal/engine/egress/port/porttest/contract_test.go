package porttest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/egress/port"
)

func TestProxyContract(t *testing.T) {
	ProxyContract(t, func(f ProxyFixture) port.Proxy { return NewProxy(f) })
}

func TestProxyFake(t *testing.T) {
	log := NewLog()
	proxy := NewProxy(ProxyFixture{Log: log})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	names := []string{"a.example", "b.example"}
	go func() { done <- proxy.Serve(ctx, names) }()
	for len(log.Lines()) == 0 {
		time.Sleep(time.Millisecond)
	}
	names[0] = "changed"
	if got := proxy.Names(); len(got) != 2 || got[0] != "a.example" || got[1] != "b.example" {
		t.Errorf("Names() = %q; want the names given, unchanged by the caller", got)
	}
	got := proxy.Names()
	got[1] = "changed"
	if again := proxy.Names(); again[1] != "b.example" {
		t.Errorf("Names() shares its storage: %q", again)
	}
	cancel()
	if err := <-done; err != nil {
		t.Errorf("Serve = %v; want nil", err)
	}
	lines := log.Lines()
	lines[0][0] = 'X'
	if again := log.Lines(); again[0][0] != '{' {
		t.Error("Lines shares its storage with the log")
	}
}

type recordingReporter struct{ errors []string }

func (*recordingReporter) Helper() {}
func (r *recordingReporter) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

// faulty is a proxy with one fault, to show the contract fails it.
type faulty struct {
	fixture ProxyFixture
	fault   string
}

func (p *faulty) write(line string) { _, _ = p.fixture.Log.Write([]byte(line)) }

func ready(seq int, when, extra string) string {
	return fmt.Sprintf("{\"schema\":1,\"seq\":%d,\"time\":%q,\"event\":\"ready\"%s}\n", seq, when, extra)
}

func (p *faulty) Serve(ctx context.Context, names []string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	invalid := p.fixture.Failure == port.ListInvalid
	switch p.fault {
	case "no-ready":
		<-ctx.Done()
		return nil
	case "accepts-invalid":
		p.write(ready(1, now, ""))
		<-ctx.Done()
		return nil
	case "wrong-failure":
		if invalid {
			return port.ListenFailed
		}
	case "invalid-writes":
		if invalid {
			p.write(ready(1, now, ""))
			return port.ListInvalid
		}
	case "log-failure-ignored":
		if p.fixture.Failure == port.LogFailed {
			<-ctx.Done()
			return nil
		}
	}
	if invalid {
		return port.ListInvalid
	}
	if p.fixture.Failure == port.LogFailed {
		return port.LogFailed
	}
	switch p.fault {
	case "ready-twice":
		p.write(ready(1, now, ""))
		p.write(ready(2, now, ""))
	case "wrong-schema":
		p.write(strings.Replace(ready(1, now, ""), `"schema":1`, `"schema":2`, 1))
	case "wrong-seq":
		p.write(ready(2, now, ""))
	case "local-time":
		p.write(ready(1, time.Now().In(time.FixedZone("CET", 3600)).Format(time.RFC3339), ""))
	case "fractional-time":
		p.write(ready(1, time.Now().UTC().Format(time.RFC3339Nano), ""))
	case "no-newline":
		p.write(strings.TrimSuffix(ready(1, now, ""), "\n"))
	case "extra-field":
		p.write(ready(1, now, `,"client":"10.0.0.1"`))
	case "wrong-event":
		p.write(strings.Replace(ready(1, now, ""), `"ready"`, `"refused"`, 1))
	default:
		p.write(ready(1, now, ""))
	}
	switch p.fault {
	case "early-return":
		return nil
	case "late-write":
		go func() {
			<-ctx.Done()
			time.Sleep(5 * time.Millisecond)
			p.write(ready(2, now, ""))
		}()
	}
	<-ctx.Done()
	if p.fault == "cancel-error" {
		return errors.New("context canceled")
	}
	return nil
}

func TestProxyContractCatchesFaults(t *testing.T) {
	for fault, want := range map[string]string{
		"no-ready":            "wrote no ready record",
		"ready-twice":         "records in all",
		"wrong-schema":        "ready record =",
		"wrong-seq":           "ready record =",
		"local-time":          "not RFC 3339 UTC",
		"fractional-time":     "to the second",
		"no-newline":          "ready record =",
		"extra-field":         "not one JSON object",
		"wrong-event":         "ready record =",
		"early-return":        "before ctx ended",
		"cancel-error":        "want nil",
		"late-write":          "records in all",
		"accepts-invalid":     "did not return for a list or log it must fail",
		"wrong-failure":       `Serve(["LISTED.example"]) = `,
		"invalid-writes":      "records for an invalid list",
		"log-failure-ignored": "Serve with a failing log",
	} {
		t.Run(fault, func(t *testing.T) {
			t.Parallel()
			reporter := &recordingReporter{}
			proxyContract(reporter, 250*time.Millisecond, 60*time.Millisecond, func(f ProxyFixture) port.Proxy {
				return &faulty{fixture: f, fault: fault}
			})
			if !strings.Contains(strings.Join(reporter.errors, "\n"), want) {
				t.Fatalf("contract missed %s: %q", fault, reporter.errors)
			}
		})
	}
	t.Run("a sound proxy reports nothing", func(t *testing.T) {
		reporter := &recordingReporter{}
		proxyContract(reporter, 250*time.Millisecond, 60*time.Millisecond, func(f ProxyFixture) port.Proxy { return &faulty{fixture: f} })
		if len(reporter.errors) != 0 {
			t.Errorf("contract failed a sound proxy: %q", reporter.errors)
		}
	})
}
