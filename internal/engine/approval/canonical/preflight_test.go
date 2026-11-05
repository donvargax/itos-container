package canonical

import (
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"strings"
	"testing"
)

func TestLocalComparisonNeverClaimsEngineMatch(t *testing.T) {
	codec := JSON{}
	r := porttest.Ready()
	full := codec.Candidate(r, "1000", "2000")
	local := codec.LocalCandidate(r, "1000", "2000")
	record := port.Record{Candidate: full, Generation: "1"}
	if local == full || !codec.LocalMatches(local, record) || strings.Contains(local.Canonical, "engine-A") {
		t.Fatal("local comparison imported unobserved engine facts")
	}
	r.Engine.ID = "engine-B"
	r.Image.ID = "sha256:" + strings.Repeat("c", 64)
	if changed := codec.Candidate(r, "1000", "2000"); !codec.LocalMatches(local, port.Record{Candidate: changed}) || changed == full {
		t.Fatal("engine facts compared before inspection")
	}
	r.Manifest.Effective.ContainerEnv["APP"] = "new"
	if codec.LocalMatches(codec.LocalCandidate(r, "1000", "2000"), record) {
		t.Fatal("local change accepted")
	}
	broken := local
	broken.Canonical = "{"
	if codec.LocalMatches(broken, record) || codec.LocalMatches(local, port.Record{Candidate: broken}) {
		t.Fatal("malformed settings compared")
	}
}
func TestFrozenEnvelopeRetainsValidation(t *testing.T) {
	codec := JSON{}
	c := codec.Candidate(porttest.Ready(), "1000", "2000")
	original := port.Frozen{Project: c.Project, Generation: "1", Fingerprint: c.Fingerprint, Settings: c.Canonical}
	if r, f := codec.FrozenRecord(original); f != "" || r.Candidate != c || r.Generation != "1" {
		t.Fatal(r, f)
	}
	for _, edit := range []func(*port.Frozen){
		func(f *port.Frozen) { f.Project = "other" }, func(f *port.Frozen) { f.Generation = "01" }, func(f *port.Frozen) { f.Fingerprint = "wrong" }, func(f *port.Frozen) { f.Settings += "\n" }, func(f *port.Frozen) { f.Settings = "{" }, func(f *port.Frozen) { f.Settings = "[]" }, func(f *port.Frozen) { f.Settings = strings.Repeat(" ", 1024*1024+1) }, func(f *port.Frozen) { f.Settings = strings.Repeat("[", 33) + "0" + strings.Repeat("]", 33) },
	} {
		f := original
		edit(&f)
		if r, failure := codec.FrozenRecord(f); failure != "state.invalid" || r != (port.Record{}) {
			t.Fatal("accepted invalid frozen input", r, failure)
		}
	}
}
