package domain

import (
	"testing"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func TestProxyExecutableAdmitsOnlyAnOutsideStaticLinuxAmd64File(t *testing.T) {
	static := port.Executable{Path: "/usr/local/bin/itos-container", Regular: true, OS: "linux", Architecture: "amd64"}
	if got := ProxyExecutable(static, port.None, "/home/person/checkout"); got != port.None {
		t.Fatalf("the installed static program was refused as %v", got)
	}
	change := func(apply func(*port.Executable)) port.Executable {
		observed := static
		apply(&observed)
		return observed
	}
	for name, tc := range map[string]struct {
		observed port.Executable
		failure  port.Failure
	}{
		"an observation that could not be made": {static, port.EgressExecutable},
		"any other failure beside a reading":    {static, port.EngineInspect},
		"no observation at all":                 {port.Executable{}, port.None},
		"not a regular file":                    {change(func(e *port.Executable) { e.Regular = false }), port.None},
		"another operating system":              {change(func(e *port.Executable) { e.OS = "freebsd" }), port.None},
		"no operating system read":              {change(func(e *port.Executable) { e.OS = "" }), port.None},
		"another machine":                       {change(func(e *port.Executable) { e.Architecture = "arm64" }), port.None},
		"the wrong class":                       {change(func(e *port.Executable) { e.Architecture = "amd64p32" }), port.None},
		"no architecture read":                  {change(func(e *port.Executable) { e.Architecture = "" }), port.None},
		"an interpreter":                        {change(func(e *port.Executable) { e.Interpreter = true }), port.None},
		"a dynamic segment":                     {change(func(e *port.Executable) { e.Dynamic = true }), port.None},
		"an interpreter and a dynamic segment":  {change(func(e *port.Executable) { e.Interpreter, e.Dynamic = true, true }), port.None},
	} {
		if got := ProxyExecutable(tc.observed, tc.failure, "/home/person/checkout"); got != port.EgressExecutable {
			t.Errorf("%s: judged %v", name, got)
		}
	}
}

func TestProxyExecutableMustLieOutsideTheCheckout(t *testing.T) {
	at := func(path string) port.Executable {
		return port.Executable{Path: path, Regular: true, OS: "linux", Architecture: "amd64"}
	}
	for _, tc := range []struct {
		path, root string
		want       port.Failure
	}{
		{"/usr/local/bin/itos-container", "/home/person/checkout", port.None},
		{"/home/person/checkout-tools/itos-container", "/home/person/checkout", port.None},
		{"/home/person/itos-container", "/home/person/checkout", port.None},
		{"/home/person/itos-container", "/home/person/checkout/", port.None},
		{"/checkout", "/checkout/deeper", port.None},
		{"/home/person/checkout/itos-container", "/home/person/checkout", port.EgressExecutable},
		{"/home/person/checkout/itos-container", "/home/person/checkout/", port.EgressExecutable},
		{"/home/person/checkout/tools/bin/itos-container", "/home/person/checkout", port.EgressExecutable},
		{"/home/person/checkout", "/home/person/checkout", port.EgressExecutable},
		{"/home/person/checkout/", "/home/person/checkout", port.EgressExecutable},
		{"/usr/local/bin/itos-container", "/", port.EgressExecutable},
		// A path or root that is not absolute and shortest cannot place the file.
		{"", "/home/person/checkout", port.EgressExecutable},
		{"itos-container", "/home/person/checkout", port.EgressExecutable},
		{"./itos-container", "/home/person/checkout", port.EgressExecutable},
		{"/usr/local/../../home/person/checkout/itos-container", "/home/person/checkout", port.EgressExecutable},
		{"/usr/./bin/itos-container", "/home/person/checkout", port.EgressExecutable},
		{"/usr//bin/itos-container", "/home/person/checkout", port.EgressExecutable},
		{"/usr/local/bin/itos-container", "", port.EgressExecutable},
		{"/usr/local/bin/itos-container", "checkout", port.EgressExecutable},
		{"/usr/local/bin/itos-container", "/home/person/../person/checkout", port.EgressExecutable},
		{"/usr/local/bin/itos-container", "/home/person//checkout", port.EgressExecutable},
		{"/usr/local/bin/itos-container", "/home/./checkout", port.EgressExecutable},
	} {
		if got := ProxyExecutable(at(tc.path), port.None, tc.root); got != tc.want {
			t.Errorf("%q with checkout %q: judged %v, want %v", tc.path, tc.root, got, tc.want)
		}
	}
}
