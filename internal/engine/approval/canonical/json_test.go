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
