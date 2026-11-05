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

func TestFrozenRecordBoundsTheCompleteEnvelope(t *testing.T) {
	const limit = 1024 * 1024
	codec := JSON{}
	for _, tc := range []struct {
		name string
		size int
		want port.Failure
	}{
		{"below limit", limit - 1, ""},
		{"at limit", limit, ""},
		{"above limit", limit + 1, "state.invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ready := porttest.Ready()
			ready.Manifest.Effective.ContainerEnv["PADDING"] = ""
			base := port.Record{Candidate: codec.Candidate(ready, "1000", "2000"), Generation: "1"}
			ready.Manifest.Effective.ContainerEnv["PADDING"] = strings.Repeat("x", tc.size-len(codec.Encode(base)))
			c := codec.Candidate(ready, "1000", "2000")
			want := port.Record{Candidate: c, Generation: "1"}
			if size := len(codec.Encode(want)); size != tc.size {
				t.Fatalf("envelope size = %d, want %d", size, tc.size)
			}
			frozen := port.Frozen{Project: c.Project, Generation: "1", Fingerprint: c.Fingerprint, Settings: c.Canonical}
			record, failure := codec.FrozenRecord(frozen)
			if failure != tc.want {
				t.Fatalf("failure = %q, want %q", failure, tc.want)
			}
			if tc.want != "" {
				want = port.Record{}
			}
			if record != want {
				t.Fatal("returned record differs from the expected validated envelope")
			}
		})
	}
}

func TestFrozenRecordRejectsSettingsAtEnvelopeLimit(t *testing.T) {
	const limit = 1024 * 1024
	codec := JSON{}
	ready := porttest.Ready()
	ready.Manifest.Effective.ContainerEnv["PADDING"] = ""
	base := codec.Candidate(ready, "1000", "2000")
	ready.Manifest.Effective.ContainerEnv["PADDING"] = strings.Repeat("x", limit-len(base.Canonical))
	c := codec.Candidate(ready, "1000", "2000")
	if len(c.Canonical) != limit {
		t.Fatalf("settings size = %d, want %d", len(c.Canonical), limit)
	}
	// Valid canonical settings of exactly 1 MiB still exceed the limit once the
	// required envelope fields are included; no selected record may be returned.
	if size := len(codec.Encode(port.Record{Candidate: c, Generation: "1"})); size <= limit {
		t.Fatalf("complete envelope size = %d, want more than %d", size, limit)
	}
	frozen := port.Frozen{Project: c.Project, Generation: "1", Fingerprint: c.Fingerprint, Settings: c.Canonical}
	if record, failure := codec.FrozenRecord(frozen); record != (port.Record{}) || failure != "state.invalid" {
		t.Fatalf("settings at the envelope limit were accepted: failure = %q", failure)
	}
}
