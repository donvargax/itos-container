package domain_test

import (
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/egress/domain"
	"github.com/donvargax/itos-container/internal/engine/egress/port"
)

func list(t testing.TB, names ...string) domain.Allowlist {
	t.Helper()
	allowlist, ok := domain.NewAllowlist(names)
	if !ok {
		t.Errorf("NewAllowlist(%q) refused a valid list", names)
	}
	return allowlist
}

// head is a request line with an empty header block.
func head(line string) string { return line + "\r\n\r\n" }

type judgeCase struct {
	name   string
	input  string
	pass   bool
	reason port.Reason
	fields [3]string // method, target, port
}

// judgeCases is TestJudge's table and FuzzHead's seeds.
func judgeCases() []judgeCase {
	host255 := strings.Repeat("a", 255)
	return []judgeCase{
		{"a pass", head("CONNECT listed.example:443 HTTP/1.1"), true, "", [3]string{"CONNECT", "listed.example", "443"}},
		{"a pass over HTTP/1.0", head("CONNECT listed.example:443 HTTP/1.0"), true, "", [3]string{"CONNECT", "listed.example", "443"}},
		{"a pass with headers unread", "CONNECT listed.example:443 HTTP/1.1\r\nHost: other.example:80\r\nProxy-Authorization: Basic c2VjcmV0\r\n\r\n", true, "", [3]string{"CONNECT", "listed.example", "443"}},
		{"a pass with bytes after the head", head("CONNECT listed.example:443 HTTP/1.1") + "\x16\x03\x01", true, "", [3]string{"CONNECT", "listed.example", "443"}},
		{"the second listed name", head("CONNECT other-listed.example:443 HTTP/1.1"), true, "", [3]string{"CONNECT", "other-listed.example", "443"}},
		// not-listed
		{"not listed", head("CONNECT other.example:443 HTTP/1.1"), false, port.NotListed, [3]string{"CONNECT", "other.example", "443"}},
		{"not listed on port 80 reads not-listed before port", head("CONNECT other.example:80 HTTP/1.1"), false, port.NotListed, [3]string{"CONNECT", "other.example", "80"}},
		{"a bare number", head("CONNECT 1234:443 HTTP/1.1"), false, port.NotListed, [3]string{"CONNECT", "1234", "443"}},
		{"three dotted runs", head("CONNECT 1.2.3:443 HTTP/1.1"), false, port.NotListed, [3]string{"CONNECT", "1.2.3", "443"}},
		{"two trailing dots", head("CONNECT listed.example..:443 HTTP/1.1"), false, port.NotListed, [3]string{"CONNECT", "listed.example..", "443"}},
		{"a final xn--, compared as written", head("CONNECT listed.example.xn--:443 HTTP/1.1"), false, port.NotListed, [3]string{"CONNECT", "listed.example.xn--", "443"}},
		{"a prefix match would pass", head("CONNECT listed.example.evil.test:443 HTTP/1.1"), false, port.NotListed, [3]string{"CONNECT", "listed.example.evil.test", "443"}},
		{"a subdomain, which a suffix match would pass", head("CONNECT a.listed.example:443 HTTP/1.1"), false, port.NotListed, [3]string{"CONNECT", "a.listed.example", "443"}},
		{"a longer name, which a suffix match would pass", head("CONNECT xlisted.example:443 HTTP/1.1"), false, port.NotListed, [3]string{"CONNECT", "xlisted.example", "443"}},
		{"a 255-byte host", head("CONNECT " + host255 + ":443 HTTP/1.1"), false, port.NotListed, [3]string{"CONNECT", host255, "443"}},
		// not-canonical
		{"upper case", head("CONNECT LISTED.example:443 HTTP/1.1"), false, port.NotCanonical, [3]string{"CONNECT", "LISTED.example", "443"}},
		{"a trailing dot", head("CONNECT listed.example.:443 HTTP/1.1"), false, port.NotCanonical, [3]string{"CONNECT", "listed.example.", "443"}},
		{"upper case on port 80 reads not-canonical before port", head("CONNECT LISTED.example:80 HTTP/1.1"), false, port.NotCanonical, [3]string{"CONNECT", "LISTED.example", "80"}},
		// literal-ip
		{"dotted IPv4", head("CONNECT 127.0.0.1:443 HTTP/1.1"), false, port.LiteralIP, [3]string{"CONNECT", "127.0.0.1", "443"}},
		{"dotted IPv4 on port 80 reads literal-ip before port", head("CONNECT 127.0.0.1:80 HTTP/1.1"), false, port.LiteralIP, [3]string{"CONNECT", "127.0.0.1", "80"}},
		{"dotted IPv4 out of range", head("CONNECT 999.999.999.999:443 HTTP/1.1"), false, port.LiteralIP, [3]string{"CONNECT", "999.999.999.999", "443"}},
		{"bracketed IPv6", head("CONNECT [::1]:443 HTTP/1.1"), false, port.LiteralIP, [3]string{"CONNECT", "[::1]", "443"}},
		// port
		{"a leading zero in the port", head("CONNECT listed.example:0443 HTTP/1.1"), false, port.Port, [3]string{"CONNECT", "listed.example", "0443"}},
		{"port 80", head("CONNECT listed.example:80 HTTP/1.1"), false, port.Port, [3]string{"CONNECT", "listed.example", "80"}},
		{"a 5-digit port", head("CONNECT listed.example:65535 HTTP/1.1"), false, port.Port, [3]string{"CONNECT", "listed.example", "65535"}},
		// not-connect
		{"GET with a URL", head("GET http://listed.example/path?token=secret HTTP/1.1"), false, port.NotConnect, [3]string{"GET", "", ""}},
		{"GET with a plain target", head("GET listed.example:443 HTTP/1.1"), false, port.NotConnect, [3]string{"GET", "listed.example", "443"}},
		{"lower-case connect", head("connect listed.example:443 HTTP/1.1"), false, port.NotConnect, [3]string{"", "listed.example", "443"}},
		{"a 16-letter method", head("ABCDEFGHIJKLMNOP listed.example:443 HTTP/1.1"), false, port.NotConnect, [3]string{"ABCDEFGHIJKLMNOP", "listed.example", "443"}},
		{"a 17-letter method", head("ABCDEFGHIJKLMNOPQ listed.example:443 HTTP/1.1"), false, port.NotConnect, [3]string{"", "listed.example", "443"}},
		{"a method with a digit", head("M0 listed.example:443 HTTP/1.1"), false, port.NotConnect, [3]string{"", "listed.example", "443"}},
		// malformed targets of a CONNECT
		{"userinfo", head("CONNECT secret-token@listed.example:443 HTTP/1.1"), false, port.Malformed, [3]string{"CONNECT", "", ""}},
		{"a path and a query before the port", head("CONNECT a/b?token=secret:443 HTTP/1.1"), false, port.Malformed, [3]string{"CONNECT", "", ""}},
		{"a path and a query after the port", head("CONNECT listed.example:443/path?token=secret HTTP/1.1"), false, port.Malformed, [3]string{"CONNECT", "", ""}},
		{"a URL", head("CONNECT http://listed.example/path?token=secret HTTP/1.1"), false, port.Malformed, [3]string{"CONNECT", "", ""}},
		{"no port", head("CONNECT listed.example HTTP/1.1"), false, port.Malformed, [3]string{"CONNECT", "", ""}},
		{"an empty host", head("CONNECT :443 HTTP/1.1"), false, port.Malformed, [3]string{"CONNECT", "", ""}},
		{"an empty port", head("CONNECT listed.example: HTTP/1.1"), false, port.Malformed, [3]string{"CONNECT", "", ""}},
		{"empty brackets", head("CONNECT []:443 HTTP/1.1"), false, port.Malformed, [3]string{"CONNECT", "", ""}},
		{"an IPv6 zone", head("CONNECT [fe80::1%25eth0]:443 HTTP/1.1"), false, port.Malformed, [3]string{"CONNECT", "", ""}},
		{"an unclosed bracket", head("CONNECT [::1:443 HTTP/1.1"), false, port.Malformed, [3]string{"CONNECT", "", ""}},
		{"an unbracketed IPv6", head("CONNECT ::1:443 HTTP/1.1"), false, port.Malformed, [3]string{"CONNECT", "", ""}},
		{"a Unicode spelling", head("CONNECT lïsted.example:443 HTTP/1.1"), false, port.Malformed, [3]string{"CONNECT", "", ""}},
		{"a 256-byte host", head("CONNECT " + host255 + "a:443 HTTP/1.1"), false, port.Malformed, [3]string{"CONNECT", "", ""}},
		{"a 6-digit port", head("CONNECT listed.example:100000 HTTP/1.1"), false, port.Malformed, [3]string{"CONNECT", "", ""}},
		{"a non-digit port", head("CONNECT listed.example:44a HTTP/1.1"), false, port.Malformed, [3]string{"CONNECT", "", ""}},
		// malformed heads: none of the three is written
		{"empty input", "", false, port.Malformed, [3]string{}},
		{"a partial head", "CONNECT listed.example:443 HTTP/1.1\r\n", false, port.Malformed, [3]string{}},
		{"a request line without a terminator", "CONNECT listed.example:443 HTTP/1.1", false, port.Malformed, [3]string{}},
		{"a bare CR", "CONNECT listed.example:443 HTTP/1.1\r\r\n\r\n", false, port.Malformed, [3]string{}},
		{"a bare CR inside the line", "CONNECT listed.example:443\r HTTP/1.1\r\n\r\n", false, port.Malformed, [3]string{}},
		{"a bare LF", "CONNECT listed.example:443 HTTP/1.1\n\r\n\r\n", false, port.Malformed, [3]string{}},
		{"a bare LF terminator", "CONNECT listed.example:443 HTTP/1.1\n\n", false, port.Malformed, [3]string{}},
		{"a double space", head("CONNECT  listed.example:443 HTTP/1.1"), false, port.Malformed, [3]string{}},
		{"a trailing space", head("CONNECT listed.example:443 HTTP/1.1 "), false, port.Malformed, [3]string{}},
		{"a leading space", head(" CONNECT listed.example:443 HTTP/1.1"), false, port.Malformed, [3]string{}},
		{"HTTP/1.2", head("CONNECT listed.example:443 HTTP/1.2"), false, port.Malformed, [3]string{}},
		{"HTTP/2", head("CONNECT listed.example:443 HTTP/2.0"), false, port.Malformed, [3]string{}},
		{"the HTTP/2 preface", "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n", false, port.Malformed, [3]string{}},
		{"lower-case version", head("CONNECT listed.example:443 http/1.1"), false, port.Malformed, [3]string{}},
		{"a missing version", head("CONNECT listed.example:443"), false, port.Malformed, [3]string{}},
		{"a four-part line", head("CONNECT listed.example:443 HTTP/1.1 extra"), false, port.Malformed, [3]string{}},
		{"an empty method", head(" listed.example:443 HTTP/1.1"), false, port.Malformed, [3]string{}},
		{"a TLS record", "\x16\x03\x01\x02\x00\x01\x00\x01\xfc\x03\x03", false, port.Malformed, [3]string{}},
	}
}

func TestJudge(t *testing.T) {
	allowlist := list(t, "listed.example", "other-listed.example")
	for _, c := range judgeCases() {
		t.Run(c.name, func(t *testing.T) {
			got := allowlist.Judge([]byte(c.input))
			if got.Pass != c.pass || got.Reason != c.reason {
				t.Fatalf("Judge = pass %v, reason %q; want pass %v, reason %q", got.Pass, got.Reason, c.pass, c.reason)
			}
			if got.Method != c.fields[0] || got.Target != c.fields[1] || got.Port != c.fields[2] {
				t.Errorf("Judge kept %q %q %q; want %q", got.Method, got.Target, got.Port, c.fields)
			}
			if got.Pass {
				if end := strings.Index(c.input, "\r\n\r\n") + 4; got.Tunnel != end {
					t.Errorf("tunnel starts at %d; want %d", got.Tunnel, end)
				}
			}
		})
	}
	t.Run("the zero values refuse", func(t *testing.T) {
		var zero domain.Head
		if zero.Pass || zero.Reason != "" {
			t.Errorf("zero Head = %+v; want a refusal", zero)
		}
		if got := (domain.Allowlist{}).Judge([]byte(head("CONNECT listed.example:443 HTTP/1.1"))); got.Pass || got.Reason != port.NotListed {
			t.Errorf("a zero Allowlist gave %+v; want a not-listed refusal", got)
		}
	})
}

func TestJudgeHeadLimit(t *testing.T) {
	allowlist := list(t, "listed.example")
	request := "CONNECT listed.example:443 HTTP/1.1\r\nX: "
	for _, c := range []struct {
		name string
		size int // bytes through the end of the terminator
		pass bool
	}{{"8192", 8192, true}, {"8193", 8193, false}, {"8191", 8191, true}} {
		t.Run(c.name, func(t *testing.T) {
			input := request + strings.Repeat("a", c.size-len(request)-4) + "\r\n\r\n"
			if len(input) != c.size {
				t.Fatalf("built %d bytes, want %d", len(input), c.size)
			}
			got := allowlist.Judge([]byte(input))
			if got.Pass != c.pass {
				t.Errorf("a head whose terminator ends at byte %d: pass %v, reason %q; want pass %v", c.size, got.Pass, got.Reason, c.pass)
			}
			if !c.pass && (got.Reason != port.Malformed || got.Method != "" || got.Target != "" || got.Port != "") {
				t.Errorf("an oversized head = %+v; want malformed with no fields", got)
			}
			if c.pass && got.Tunnel != c.size {
				t.Errorf("tunnel starts at %d; want %d", got.Tunnel, c.size)
			}
		})
	}
	t.Run("the first of two terminators says where the tunnel starts", func(t *testing.T) {
		first := head("CONNECT listed.example:443 HTTP/1.1")
		got := allowlist.Judge([]byte(first + "\r\n\r\nrest"))
		if !got.Pass || got.Tunnel != len(first) {
			t.Errorf("Judge = %+v; want a pass with the tunnel at %d", got, len(first))
		}
	})
	t.Run("HeadEnd", func(t *testing.T) {
		for _, c := range []struct {
			input string
			end   int
			ok    bool
		}{{"", 0, false}, {"\r\n\r", 0, false}, {"\r\n\r\n", 4, true}, {"ab\r\n\r\ncd\r\n\r\n", 6, true}, {"\n\n", 0, false}} {
			if end, ok := domain.HeadEnd([]byte(c.input)); end != c.end || ok != c.ok {
				t.Errorf("HeadEnd(%q) = %d, %v; want %d, %v", c.input, end, ok, c.end, c.ok)
			}
		}
	})
}
