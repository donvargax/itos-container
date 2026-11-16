package presentation_test

import (
	"bytes"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	fake "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/presentation"
	"strings"
	"testing"
)

func TestHomePresentationContract(t *testing.T) {
	t.Run("memory", func(t *testing.T) { fake.HomePresentationContract(t, &fake.HomePresentation{}) })
	t.Run("adapter", func(t *testing.T) {
		var out, diagnostic bytes.Buffer
		fake.HomePresentationContract(t, presentation.UI{Out: &out, Err: &diagnostic})
	})
}

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
