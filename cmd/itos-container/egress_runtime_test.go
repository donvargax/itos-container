package main

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	approvaldisk "github.com/donvargax/itos-container/internal/engine/approval/disk"
	sessiondisk "github.com/donvargax/itos-container/internal/engine/session/disk"
	sessiondocker "github.com/donvargax/itos-container/internal/engine/session/docker"
)

func TestEgressRuntimeAssemblyBindsExecutableAndBothRefusalLogs(t *testing.T) {
	service, _ := approvalServices("run", nil, io.Discard, io.Discard)
	runner, recovery := sessionServices(service)
	engine, ok := runner.Engine.(sessiondocker.Engine)
	if !ok || !filepath.IsAbs(engine.Executable) {
		t.Fatal("missing controller executable")
	}
	for _, log := range []any{runner.Refusals, recovery.Refusals} {
		real, ok := log.(sessiondisk.RefusalLog)
		if !ok || real.Store.Codec == nil {
			t.Fatal("missing real refusal log/store")
		}
		if _, ok := real.Storage.(approvaldisk.Registry); !ok {
			t.Fatal("refusal log bypasses protected session storage")
		}
	}
}

func TestHiddenProxyUsesRealAdapterAndStaysOutOfHelpAndCompletion(t *testing.T) {
	var out, errout bytes.Buffer
	if code := run([]string{"__egress-proxy", "--", "UPPER.example"}, nil, &out, &errout); code != 1 || out.Len() != 0 || errout.String() != "egress allowlist is invalid\n" {
		t.Fatalf("hidden adapter: %d %q %q", code, out.String(), errout.String())
	}
	for _, args := range [][]string{{"--help"}, {"__complete", ""}} {
		out.Reset()
		errout.Reset()
		if code := run(args, nil, &out, &errout); code != 0 || strings.Contains(out.String()+errout.String(), "__egress-proxy") {
			t.Fatalf("hidden command advertised: %d %q", code, out.String())
		}
	}
}
