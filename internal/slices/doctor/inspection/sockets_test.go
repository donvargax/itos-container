package inspection

import (
	"context"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port"
	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port/porttest"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
)

var linuxAmd64 = port.Platform{OS: "linux", Architecture: "amd64"}

func TestSocketsDetectsAfreshWithTheValidatedCheckout(t *testing.T) {
	kernel := &porttest.Kernel{ABI: port.ABI{Level: 8}}
	runtimes := &porttest.Runtimes{Registration: port.RunscRegistered}
	var roots []string
	var engines []readiness.EngineIdentity
	engine := readiness.EngineIdentity{Endpoint: "unix:///run/engine.sock", ID: "engine-A"}
	s := Sockets{Platform: linuxAmd64, Kernel: kernel, Runtimes: func(checkout string, validated readiness.EngineIdentity) port.Runtimes {
		roots = append(roots, checkout)
		engines = append(engines, validated)
		return runtimes
	}}
	if got := s.Detect(context.Background(), "/checkout", engine); got != (port.Selection{Backend: port.GVisor}) {
		t.Fatal(got)
	}
	kernel.ABI = port.ABI{Level: 9}
	if got := s.Detect(context.Background(), "/other", engine); got != (port.Selection{Backend: port.Landlock}) {
		t.Fatal(got)
	}
	if len(roots) != 2 || roots[0] != "/checkout" || roots[1] != "/other" {
		t.Fatal(roots)
	}
	if len(engines) != 2 || engines[0] != engine || engines[1] != engine {
		t.Fatal(engines)
	}
}

func TestSocketsWithoutALookupConfirmsNothing(t *testing.T) {
	s := Sockets{Platform: linuxAmd64, Kernel: &porttest.Kernel{ABI: port.ABI{Level: 9}}}
	if got := s.Detect(context.Background(), "/checkout", readiness.EngineIdentity{}); got.Confirmed() || got.Failure != port.NoProbe {
		t.Fatal(got)
	}
}
