package domain_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/egress/domain"
	"github.com/donvargax/itos-container/internal/engine/egress/port"
)

func TestAllowlist(t *testing.T) {
	label63 := strings.Repeat("a", 63)
	valid := []string{
		"listed.example", "a.b", "a1.b2", "a-b.example", "xn--bcher-kva.example", "foo.0xzz", "foo.1a", "foo.0x1g",
		label63 + ".example",
		// 253 bytes: 63 + 1 + 63 + 1 + 63 + 1 + 61.
		label63 + "." + label63 + "." + label63 + "." + strings.Repeat("a", 61),
	}
	for _, name := range valid {
		if _, ok := domain.NewAllowlist([]string{name}); !ok {
			t.Errorf("NewAllowlist(%.40q) refused a canonical name", name)
		}
	}
	invalid := []string{
		"", "localhost", "LISTED.example", "listed.EXAMPLE", "listed.example.", ".example", "a..b", "listed_example.com",
		"127.0.0.1", "1.2.3.4", "foo.127", "foo.0", "foo.0x7f", "foo.0x", "foo.0x7F", "a.b.0xff", "lïsted.example",
		"listed.example:443", "listed example.com", "-", "http://listed.example",
		label63 + "a.example",
		// 254 bytes.
		label63 + "." + label63 + "." + label63 + "." + strings.Repeat("a", 62),
	}
	for _, name := range invalid {
		if _, ok := domain.NewAllowlist([]string{name}); ok {
			t.Errorf("NewAllowlist(%.40q) accepted a name that is not canonical", name)
		}
	}
	for name, names := range map[string][]string{
		"nil": nil, "empty": {}, "a repeated name": {"a.example", "b.example", "a.example"}, "one bad among good": {"a.example", "B.example"},
	} {
		if _, ok := domain.NewAllowlist(names); ok {
			t.Errorf("NewAllowlist accepted a list that is %s", name)
		}
	}
	if _, ok := domain.NewAllowlist([]string{"b.example", "a.example"}); !ok {
		t.Error("NewAllowlist refused two distinct names")
	}
	if got := domain.Allowlist.Judge(list(t, "a.example"), []byte(head("CONNECT a.example:443 HTTP/1.1"))); !got.Pass {
		t.Errorf("a one-name list refused its name: %+v", got)
	}
}

func TestJudgeHello(t *testing.T) {
	const name = "listed.example"
	for _, c := range []struct {
		label  string
		hello  port.Hello
		pass   bool
		reason port.Reason
	}{
		{"the exact name", port.Hello{Seen: true, ServerName: name}, true, ""},
		{"the exact name with other extensions", port.Hello{Seen: true, ServerName: name, Extensions: []uint16{0, 11, 0xfe0a, 0xffce}}, true, ""},
		{"a callback that never ran", port.Hello{}, false, port.NotTLS},
		{"a callback that never ran, whatever else is set", port.Hello{ServerName: name, Extensions: []uint16{0xfe0d}}, false, port.NotTLS},
		{"ECH", port.Hello{Seen: true, ServerName: name, Extensions: []uint16{0, 0xfe0d}}, false, port.TLSECH},
		{"ECH reads before a missing name", port.Hello{Seen: true, Extensions: []uint16{0xfe0d}}, false, port.TLSECH},
		{"ECH reads before a wrong name", port.Hello{Seen: true, ServerName: "other.example", Extensions: []uint16{0xfe0d}}, false, port.TLSECH},
		{"no name", port.Hello{Seen: true}, false, port.TLSNoName},
		{"no name with extensions", port.Hello{Seen: true, Extensions: []uint16{0, 11}}, false, port.TLSNoName},
		{"another name", port.Hello{Seen: true, ServerName: "other.example"}, false, port.TLSName},
		{"upper case", port.Hello{Seen: true, ServerName: "LISTED.example"}, false, port.TLSName},
		{"a trailing dot", port.Hello{Seen: true, ServerName: name + "."}, false, port.TLSName},
		{"a prefix", port.Hello{Seen: true, ServerName: "listed.exampl"}, false, port.TLSName},
	} {
		got := domain.JudgeHello(c.hello, name)
		if got.Pass != c.pass || got.Reason != c.reason {
			t.Errorf("%s: JudgeHello = pass %v, reason %q; want pass %v, reason %q", c.label, got.Pass, got.Reason, c.pass, c.reason)
		}
	}
	var zero domain.HelloVerdict
	if zero.Pass {
		t.Error("the zero HelloVerdict passes; want it to refuse")
	}
}

func TestLog(t *testing.T) {
	at := time.Date(2026, 11, 9, 14, 30, 59, 999_000_000, time.FixedZone("CET", 3600))
	log := domain.NewLog(3)
	ready, full := log.Stamp(at, port.Record{Event: port.Ready, Method: "CONNECT", Target: "x", Port: "443", Reason: port.NotListed})
	if want := (port.Record{Schema: 1, Seq: 1, Time: "2026-11-09T13:30:59Z", Event: port.Ready}); ready != want || full {
		t.Errorf("ready = %+v, full %v; want %+v: UTC, to the second, nothing else", ready, full, want)
	}
	refused := port.Record{Event: port.Refused, Reason: port.NotListed, Method: "CONNECT", Target: "other.example", Port: "443"}
	got, full := log.Stamp(at, refused)
	if want := (port.Record{Schema: 1, Seq: 2, Time: "2026-11-09T13:30:59Z", Event: port.Refused, Method: "CONNECT", Target: "other.example", Port: "443", Reason: port.NotListed}); got != want || full {
		t.Errorf("refused = %+v, full %v; want %+v with its fields kept", got, full, want)
	}
	failed, full := log.Stamp(at, port.Record{Event: port.UpstreamFailed, Reason: port.DNS, Method: "CONNECT", Target: "listed.example", Port: "443"})
	if failed.Seq != 3 || failed.Reason != port.DNS || failed.Target != "listed.example" || full {
		t.Errorf("upstream-failed = %+v, full %v; want seq 3 and not full at two counted", failed, full)
	}
	last, full := log.Stamp(at, refused)
	if last.Seq != 4 || !full {
		t.Errorf("third counted record = %+v, full %v; want seq 4 and full", last, full)
	}
	over, full := log.Stamp(at, port.Record{Event: port.LogFull, Reason: port.NotListed, Target: "x"})
	if want := (port.Record{Schema: 1, Seq: 5, Time: "2026-11-09T13:30:59Z", Event: port.LogFull}); over != want || full {
		t.Errorf("log-full = %+v, full %v; want %+v: nothing added, not counted", over, full, want)
	}
	t.Run("the limit is 20,000: the last two records are seq 20,001 and 20,002", func(t *testing.T) {
		log := domain.NewLog(20000)
		at := time.Unix(0, 0)
		log.Stamp(at, port.Record{Event: port.Ready})
		for i := 1; i <= 20000; i++ {
			event := port.Refused
			if i%2 == 0 {
				event = port.UpstreamFailed
			}
			got, full := log.Stamp(at, port.Record{Event: event})
			if got.Seq != i+1 || full != (i == 20000) {
				t.Fatalf("counted record %d = seq %d, full %v; want seq %d, full %v", i, got.Seq, full, i+1, i == 20000)
			}
		}
		if got, _ := log.Stamp(at, port.Record{Event: port.LogFull}); got.Seq != 20002 {
			t.Errorf("log-full has seq %d; want 20002", got.Seq)
		}
	})
}

func TestLimits(t *testing.T) {
	for name, c := range map[string][2]int{
		"HeadLimit": {domain.HeadLimit, 8192}, "HelloLimit": {domain.HelloLimit, 65536}, "LogLimit": {domain.LogLimit, 20000},
	} {
		if c[0] != c[1] {
			t.Errorf("%s = %s; want %s", name, strconv.Itoa(c[0]), strconv.Itoa(c[1]))
		}
	}
}
