package inspection

import (
	"context"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port"
	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port/porttest"
)

var linuxAmd64 = port.Platform{OS: "linux", Architecture: "amd64"}

func TestSocketsDetectsAfreshWithTheValidatedCheckout(t *testing.T) {
	kernel := &porttest.Kernel{ABI: port.ABI{Level: 8}}
	runtimes := &porttest.Runtimes{Registration: port.RunscRegistered}
	var roots []string
	s := Sockets{Platform: linuxAmd64, Kernel: kernel, Runtimes: func(checkout string) port.Runtimes {
		roots = append(roots, checkout)
		return runtimes
	}}
	if got := s.Detect(context.Background(), "/checkout"); got != (port.Selection{Backend: port.GVisor}) {
		t.Fatal(got)
	}
	kernel.ABI = port.ABI{Level: 9}
	if got := s.Detect(context.Background(), "/other"); got != (port.Selection{Backend: port.Landlock}) {
		t.Fatal(got)
	}
	if len(roots) != 2 || roots[0] != "/checkout" || roots[1] != "/other" {
		t.Fatal(roots)
	}
}

func TestSocketsWithoutALookupConfirmsNothing(t *testing.T) {
	s := Sockets{Platform: linuxAmd64, Kernel: &porttest.Kernel{ABI: port.ABI{Level: 9}}}
	if got := s.Detect(context.Background(), "/checkout"); got.Confirmed() || got.Failure != port.NoProbe {
		t.Fatal(got)
	}
}
