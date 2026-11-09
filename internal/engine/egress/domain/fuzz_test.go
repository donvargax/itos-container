package domain_test

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

// FuzzHead holds Judge to an oracle of its own, regular expressions here rather
// than the domain's helpers, which could not catch themselves: nothing panics; a
// pass happens exactly when the input begins with CONNECT, a listed name, :443,
// one space, HTTP/1.1 or HTTP/1.0 and CRLF and its first terminator ends within
// 8,192 bytes; a pass's tunnel offset follows that terminator; and the method,
// target and port it would log obey the log's rule.
func FuzzHead(f *testing.F) {
	for _, c := range judgeCases() {
		f.Add([]byte(c.input))
	}
	pass := regexp.MustCompile(`^CONNECT (listed\.example|other-listed\.example):443 HTTP/1\.[01]\r\n`)
	method := regexp.MustCompile(`^[A-Z]{1,16}$`)
	host := regexp.MustCompile(`^([A-Za-z0-9_.-]{1,255}|\[[0-9A-Fa-f:.]{1,253}\])$`)
	digits := regexp.MustCompile(`^[0-9]{1,5}$`)
	allowlist := list(f, "listed.example", "other-listed.example")
	f.Fuzz(func(t *testing.T, input []byte) {
		got := allowlist.Judge(input)
		index := bytes.Index(input, []byte("\r\n\r\n"))
		want := pass.Match(input) && index >= 0 && index+4 <= 8192
		if got.Pass != want {
			t.Fatalf("Judge passed = %v, oracle = %v for %q", got.Pass, want, input)
		}
		if got.Pass && (got.Tunnel != index+4 || got.Reason != "") {
			t.Fatalf("a pass has tunnel %d and reason %q; want %d and none", got.Tunnel, got.Reason, index+4)
		}
		if !got.Pass && got.Reason == "" {
			t.Fatalf("a refusal has no reason for %q", input)
		}
		if got.Method != "" && !method.MatchString(got.Method) {
			t.Fatalf("method %q breaks the log's rule", got.Method)
		}
		if (got.Target == "") != (got.Port == "") || (got.Target != "" && (!host.MatchString(got.Target) || !digits.MatchString(got.Port))) {
			t.Fatalf("target %q and port %q break the log's rule", got.Target, got.Port)
		}
		for _, kept := range []string{got.Method, got.Target, got.Port} {
			if kept != "" && !strings.Contains(string(input), kept) {
				t.Fatalf("kept %q is not in the input", kept)
			}
		}
	})
}
