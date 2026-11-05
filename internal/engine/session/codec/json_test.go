package codec

import (
	"encoding/json"
	"strings"
	"testing"

	mp "github.com/donvargax/itos-container/internal/engine/manifest/port"
	mf "github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestRecordCodecRequiresEveryTypedFieldAndValidProgress(t *testing.T) {
	b, _, _ := sf.Fixture()
	record := b.Intent(strings.Repeat("1", 64))
	// The parser fake supplies parsed values; parser strictness has its own shared
	// contract. Cmd assembly and the feature suite exercise the real strict parser.
	parser := &mf.Parser{}
	codec := JSON{Parser: parser}
	data := codec.Encode(record)
	parser.Value = recordValue(t, data)
	decoded, f := codec.Decode(data)
	if f != "" || decoded != record || !parser.Calls[0].Strict {
		t.Fatal(decoded, f)
	}
	for name := range parser.Value.Fields {
		saved := parser.Value.Fields[name]
		delete(parser.Value.Fields, name)
		if _, f = codec.Decode(data); f != sp.Invalid {
			t.Fatalf("missing %s accepted", name)
		}
		parser.Value.Fields[name] = mp.Value{Kind: mp.Null}
		if _, f = codec.Decode(data); f != sp.Invalid {
			t.Fatalf("wrong type %s accepted", name)
		}
		parser.Value.Fields[name] = saved
	}
	for _, mutate := range []func(*sp.Record){
		func(r *sp.Record) { r.Project = "../x" }, func(r *sp.Record) { r.Name = "foreign" }, func(r *sp.Record) { r.Generation = "0" }, func(r *sp.Record) { r.Generation = "01" }, func(r *sp.Record) { r.Generation = "invalid" },
		func(r *sp.Record) { r.Stage = "running" }, func(r *sp.Record) { r.ContainerID = strings.Repeat("2", 64) }, func(r *sp.Record) { r.Stage = "owned" }, func(r *sp.Record) { r.Stage = "absent" },
	} {
		bad := record
		mutate(&bad)
		raw := codec.Encode(bad)
		parser.Value = recordValue(t, raw)
		if _, f = codec.Decode(raw); f != sp.Invalid {
			t.Fatal("invalid progress accepted", bad)
		}
	}
	for _, stage := range []string{"owned", "absent"} {
		valid := record
		valid.Stage = stage
		valid.ContainerID = strings.Repeat("2", 64)
		raw := codec.Encode(valid)
		parser.Value = recordValue(t, raw)
		if got, f := codec.Decode(raw); f != "" || got != valid {
			t.Fatal(got, f)
		}
	}
	parser.Value = mp.Value{Kind: mp.Array}
	if _, f = codec.Decode(data); f != sp.Invalid {
		t.Fatal(f)
	}
	parser.Issues = []mp.Issue{{Kind: mp.Syntax}}
	if _, f = codec.Decode(data); f != sp.Invalid {
		t.Fatal(f)
	}
	for _, raw := range [][]byte{nil, make([]byte, 65537)} {
		if _, f = codec.Decode(raw); f != sp.Invalid {
			t.Fatal(f)
		}
	}
}
func recordValue(t *testing.T, data []byte) mp.Value {
	t.Helper()
	var values map[string]json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		t.Fatal(err)
	}
	result := mp.Value{Kind: mp.Object, Fields: map[string]mp.Value{}}
	for name, raw := range values {
		if name == "schema" {
			result.Fields[name] = mp.Value{Kind: mp.Number, Text: string(raw)}
		} else {
			var text string
			if err := json.Unmarshal(raw, &text); err != nil {
				t.Fatal(err)
			}
			result.Fields[name] = mp.Value{Kind: mp.String, Text: text}
		}
	}
	return result
}
