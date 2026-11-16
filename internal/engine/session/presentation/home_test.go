package presentation_test

import (
	"bytes"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/donvargax/itos-container/internal/engine/session/presentation"
	"strings"
	"testing"
)

func TestHomePlainDiscoveryAndSessionDisclosure(t *testing.T) {
	var out, diagnostic bytes.Buffer
	ui := presentation.UI{Out: &out, Err: &diagnostic}
	if code := ui.PrintHomes(port.HomeResult{}, false); code != 0 || out.String() != "No retained homes.\n" {
		t.Fatal(code, out.String())
	}
	out.Reset()
	home := &port.HomeDisclosure{Storage: "volume", State: "present", Name: "itos-container-home-original"}
	if code := ui.Print(port.RecoveryResult{Entries: []port.RecoveryEntry{{ID: strings.Repeat("a", 64), Home: home}}}, false); code != 0 || !strings.Contains(out.String(), "home storage=volume state=present name="+home.Name) {
		t.Fatal(code, out.String())
	}
}
