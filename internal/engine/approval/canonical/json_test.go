package canonical

import (
	"bytes"
	"encoding/json"
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"strings"
	"testing"
)

func TestCanonicalEscapesAndRecordValidation(t *testing.T) {
	r := porttest.Ready()
	r.Manifest.Effective.ContainerEnv["APP"] = "\"\\<>&\u2028\x1b"
	codec := JSON{}
	c := codec.Candidate(r, "2000", "3000")
	if !strings.Contains(c.Canonical, `"APP":"\"\\\u003c\u003e\u0026\u2028\u001b"`) {
		t.Fatal(c.Canonical)
	}
	record := port.Record{Candidate: c, Generation: "1"}
	data := codec.Encode(record)
	if got, f := codec.Decode(c, data); f != "" || got != record {
		t.Fatal(got, f)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{nil, []byte("null"), []byte("{"), append(append([]byte{}, data...), 10), append([]byte(`{"schema":1,`), data[1:]...), []byte(strings.Repeat("[", 33) + "0" + strings.Repeat("]", 33)), bytes.Repeat([]byte(" "), 1024*1024+1)} {
		if _, f := codec.Decode(c, bad); f != "state.invalid" {
			t.Fatalf("accepted malformed record: %s", f)
		}
	}
	for _, change := range []func(map[string]any){
		func(v map[string]any) { v["schema"] = 2 }, func(v map[string]any) { v["generation"] = "0" }, func(v map[string]any) { v["generation"] = "01" }, func(v map[string]any) { v["generation"] = "18446744073709551616" },
		func(v map[string]any) { v["settings"].(map[string]any)["profile"] = "other" }, func(v map[string]any) { delete(v, "settings") }, func(v map[string]any) { v["unknown"] = true },
		func(v map[string]any) { v["project"] = strings.Repeat("0", 64) }, func(v map[string]any) { v["fingerprint"] = "sha256:" + strings.Repeat("0", 64) },
	} {
		var obj map[string]any
		_ = json.Unmarshal(data, &obj)
		change(obj)
		changed, _ := json.Marshal(obj)
		if _, f := codec.Decode(c, changed); f != "state.invalid" {
			t.Fatalf("accepted inconsistent record %s", changed)
		}
	}
	broken := c
	broken.Canonical = "not JSON"
	if codec.Encode(port.Record{Candidate: broken}) != nil {
		t.Fatal("encoded invalid canonical input")
	}
	if _, f := codec.Decode(broken, data); f != "state.invalid" {
		t.Fatal(f)
	}
}
func TestCandidateCopiesInputAndExcludesObservations(t *testing.T) {
	r := porttest.Ready()
	r.Manifest.Effective.ContainerEnv["APP"] = "before"
	codec := JSON{}
	first := codec.Candidate(r, "2000", "3000")
	r.Manifest.Effective.ContainerEnv["APP"] = "after"
	second := codec.Candidate(r, "2000", "3000")
	if first.Canonical == second.Canonical || !strings.Contains(first.Canonical, "before") {
		t.Fatal("candidate aliases input")
	}
	r.Engine.APIVersion = "another"
	if got := codec.Candidate(r, "2000", "3000"); got != second {
		t.Fatal("negotiation affected approval")
	}
	r.Manifest.Effective.ContainerEnv = nil
	c := codec.Candidate(r, "2000", "3000")
	if !strings.Contains(c.Canonical, `"containerEnv":{}`) {
		t.Fatal(c)
	}
}

func TestDecodeRejectsInvalidJSONTokens(t *testing.T) {
	codec := JSON{}
	candidate := codec.Candidate(porttest.Ready(), "2000", "3000")
	for _, data := range []string{`{"schema":!}`, `{"schema":"\q"}`, `{"schema":1,]`, `{"schema":1} garbage`} {
		t.Run(data, func(t *testing.T) {
			if record, failure := codec.Decode(candidate, []byte(data)); failure != "state.invalid" || record != (port.Record{}) {
				t.Fatalf("invalid token produced record %#v, failure %q", record, failure)
			}
		})
	}
}

func TestDecodeRejectsSelfConsistentInvalidProfile(t *testing.T) {
	testRejectsSelfConsistentInvalidSettings(t, func(s *settings) { s.Profile = "other" })
}

func TestDecodeRejectsSelfConsistentInvalidEngineOS(t *testing.T) {
	testRejectsSelfConsistentInvalidSettings(t, func(s *settings) { s.Engine.OS = "windows" })
}

func testRejectsSelfConsistentInvalidSettings(t *testing.T, change func(*settings)) {
	t.Helper()
	codec := JSON{}
	current := codec.Candidate(porttest.Ready(), "2000", "3000")
	record := port.Record{Candidate: current, Generation: "1"}
	if got, failure := codec.Decode(current, codec.Encode(record)); failure != "" || got != record {
		t.Fatalf("valid record decoded as %#v, failure %q", got, failure)
	}
	frozen := port.Frozen{
		Settings: current.Canonical, Fingerprint: current.Fingerprint,
		Project: current.Project, Generation: record.Generation,
	}
	if got, failure := codec.FrozenRecord(frozen); failure != "" || got != record {
		t.Fatalf("valid frozen record decoded as %#v, failure %q", got, failure)
	}

	var s settings
	if err := json.Unmarshal([]byte(current.Canonical), &s); err != nil {
		t.Fatal(err)
	}
	change(&s)
	// Recompute the fingerprint so a digest mismatch cannot mask invalid settings.
	changed := port.Record{Candidate: candidate(s), Generation: record.Generation}
	if changed.UID != current.UID || changed.GID != current.GID || changed.Root != current.Root || changed.Project != current.Project {
		t.Fatal("invalid-settings fixture changed the host or project identity")
	}
	if got, failure := codec.Decode(current, codec.Encode(changed)); failure != "state.invalid" || got != (port.Record{}) {
		t.Errorf("invalid settings decoded as %#v, failure %q; want empty record and state.invalid", got, failure)
	}
	frozen.Settings, frozen.Fingerprint = changed.Canonical, changed.Fingerprint
	if got, failure := codec.FrozenRecord(frozen); failure != "state.invalid" || got != (port.Record{}) {
		t.Errorf("invalid frozen settings decoded as %#v, failure %q; want empty record and state.invalid", got, failure)
	}
}
