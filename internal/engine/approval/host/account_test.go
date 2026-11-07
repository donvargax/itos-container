package host

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
)

func TestAccountHomeRequiresLocalEffectiveUIDRecord(t *testing.T) {
	for _, tc := range []struct {
		name, passwd, want string
	}{
		{"valid", "fixture:x:1234:2345::/account/home:/bin/sh\n", "/account/home"},
		{"clean home", "fixture:x:1234:2345::/account/./home/:/bin/sh", "/account/home"},
		{"skip unrelated rows", "\n#comment\nother:x:2345:1234::/other:/bin/sh\nfixture:x:1234:2345::/account:/bin/sh\n", "/account"},
		{"empty database", "", ""},
		{"different UID", "fixture:x:2345:1234::/account:/bin/sh\n", ""},
		{"UID in other field", "1234:x:2345:1234::/account:/bin/sh\n", ""},
		{"invalid UID", "fixture:x:invalid:2345::/account:/bin/sh\n", ""},
		{"relative home", "fixture:x:1234:2345::relative:/bin/sh\n", ""},
		{"empty home", "fixture:x:1234:2345:::/bin/sh\n", ""},
		{"NUL home", "fixture:x:1234:2345::/account\x00:/bin/sh\n", ""},
		{"invalid GID", "fixture:x:1234:invalid::/account:/bin/sh\n", ""},
		{"negative GID", "fixture:x:1234:-1::/account:/bin/sh\n", ""},
		{"overflow GID", "fixture:x:1234:4294967296::/account:/bin/sh\n", ""},
		{"incomplete record", "fixture:x:1234:2345::/account\n", ""},
		{"extra field", "fixture:x:1234:2345::/account:/bin/sh:extra\n", ""},
		{"empty account name", ":x:1234:2345::/account:/bin/sh\n", ""},
		{"comment record", "#fixture:x:1234:2345::/account:/bin/sh\n", ""},
		{"NIS inclusion", "+fixture:x:1234:2345::/account:/bin/sh\n", ""},
		{"NIS exclusion", "-fixture:x:1234:2345::/account:/bin/sh\n", ""},
		{"overlong record", "fixture:x:1234:2345:" + strings.Repeat("x", 65536) + ":/account:/bin/sh\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", "/environment/home")
			t.Setenv("USER", "environment-user")
			t.Setenv("LOGNAME", "environment-logname")
			closed := false
			ops := Process{}.operations()
			ops.geteuid = func() int { return 1234 }
			ops.openAccount = func() (io.ReadCloser, error) {
				return accountReader{Reader: strings.NewReader(tc.passwd), closed: &closed}, nil
			}
			got, failure := (Process{ops: &ops}).AccountHome()
			if !closed {
				t.Fatal("account descriptor was not closed")
			}
			if got != tc.want {
				t.Fatalf("home %q, want %q", got, tc.want)
			}
			if tc.want == "" && failure != "state.unavailable" || tc.want != "" && failure != "" {
				t.Fatalf("unexpected failure %q", failure)
			}
		})
	}
}

type accountReader struct {
	io.Reader
	closed *bool
}

func (r accountReader) Close() error { *r.closed = true; return nil }

func TestAccountHomeUnavailableEvidence(t *testing.T) {
	for _, kind := range []string{"unavailable UID", "open failure", "read failure"} {
		t.Run(kind, func(t *testing.T) {
			ops := Process{}.operations()
			ops.geteuid = func() int {
				if kind == "unavailable UID" {
					return -1
				}
				return 1234
			}
			opened, closed := false, false
			ops.openAccount = func() (io.ReadCloser, error) {
				opened = true
				if kind == "open failure" {
					return nil, errors.New("unavailable")
				}
				return accountReader{Reader: iotest.ErrReader(errors.New("unreadable")), closed: &closed}, nil
			}
			got, failure := (Process{ops: &ops}).AccountHome()
			if got != "" || failure != "state.unavailable" {
				t.Fatal(got, failure)
			}
			if opened != (kind != "unavailable UID") || closed != (kind == "read failure") {
				t.Fatal("incorrect account descriptor lifetime")
			}
		})
	}
}

func TestAccountHomeReadOnlyHostLookup(t *testing.T) {
	// Never create state under the observed home. Changing process environment
	// must preserve either the database-backed result or the typed refusal.
	want, wantFailure := (Process{}).AccountHome()
	t.Setenv("HOME", filepath.Join(t.TempDir(), "environment"))
	t.Setenv("USER", "environment-user")
	t.Setenv("LOGNAME", "environment-logname")
	got, failure := (Process{}).AccountHome()
	if got != want || failure != wantFailure {
		t.Fatal("environment changed account observation")
	}
	if failure == "" && (!filepath.IsAbs(got) || os.Geteuid() < 0) {
		t.Fatal("invalid account observation")
	}
}
