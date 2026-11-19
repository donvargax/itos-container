package presentation_test

import (
	"bytes"
	"encoding/json"
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

func TestHomePlainOutputDoesNotMisreportAnEmptyCatalog(t *testing.T) {
	var out, diagnostic bytes.Buffer
	ui := presentation.UI{Out: &out, Err: &diagnostic}
	id := strings.Repeat("a", 64)
	if code := ui.PrintHomes(port.HomeResult{Homes: []port.HomeEntry{{ID: id}}}, false); code != 0 {
		t.Fatal(code)
	}
	var entry port.HomeEntry
	if err := json.Unmarshal(out.Bytes(), &entry); err != nil || entry.ID != id || diagnostic.Len() != 0 {
		t.Fatal("nonempty catalog included a contradictory empty notice", out.String(), err)
	}
	out.Reset()
	if code := ui.PrintHomes(port.HomeResult{Failures: []port.RecoveryFailure{{Outcome: port.Incomplete}}}, false); code != 75 || out.Len() != 0 || !strings.Contains(diagnostic.String(), "recovery.temporary") {
		t.Fatal("failed discovery claimed an empty catalog", code, out.String(), diagnostic.String())
	}
}
