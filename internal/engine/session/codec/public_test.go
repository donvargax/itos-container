package codec

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	mp "github.com/donvargax/itos-container/internal/engine/manifest/port"
	mf "github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestPublicCodecRejectsInvalidRecoveryFacts(t *testing.T) {
	id, project, token := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	valid := sp.PublicRecord{Schema: 1, ID: id, Project: project, Intent: sp.Reference(project, token), State: "failed"}
	for _, tc := range []struct{ field, raw string }{
		{"schema", `2`}, {"schema", `"1"`}, {"id", `null`}, {"id", `"short"`},
		{"state", `"future"`}, {"workload_exit", `-1`}, {"workload_exit", `256`},
		{"workload_exit", `-0`}, {"workload_exit", `"7"`}, {"last_problem", `"engine body"`},
		{"intent", `"intents/foreign/record.json"`}, {"workload_exit", `1.5`},
	} {
		t.Run(tc.field+"="+tc.raw, func(t *testing.T) {
			data := (JSON{}).EncodePublic(valid)
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			fields[tc.field] = json.RawMessage(tc.raw)
			data, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			parser := &mf.Parser{Value: publicValue(t, data)}
			codec := JSON{Parser: parser}
			if result, failure := codec.DecodePublic(data); failure != sp.Invalid || result != (sp.PublicRecord{}) {
				t.Fatal("invalid recovery fact acquired authority", result, failure)
			}
		})
	}
	for _, field := range []string{"workload_exit", "last_problem"} {
		data := (JSON{}).EncodePublic(valid)
		parser := &mf.Parser{Value: publicValue(t, data)}
		delete(parser.Value.Fields, field)
		parser.Value.Fields["unknown"] = mp.Value{Kind: mp.Null}
		if _, failure := (JSON{Parser: parser}).DecodePublic(data); failure != sp.Invalid {
			t.Fatal("unknown field replaced a required field", field)
		}
	}
	for _, exit := range []int{0, 7, 255} {
		r := valid
		r.WorkloadExit = &exit
		problem := sp.Incomplete
		r.LastProblem = &problem
		data := (JSON{}).EncodePublic(r)
		parser := &mf.Parser{Value: publicValue(t, data)}
		decoded, failure := (JSON{Parser: parser}).DecodePublic(data)
		if failure != "" || !reflect.DeepEqual(decoded, r) || !parser.Calls[0].Strict {
			t.Fatal("valid recovery facts did not round trip", decoded, failure)
		}
	}
}

func TestPublicCodecRejectsUnboundedOrMalformedInput(t *testing.T) {
	for _, raw := range []string{"", "{", "[}", strings.Repeat(" ", 65537), strings.Repeat("[", 17) + "0" + strings.Repeat("]", 17)} {
		parser := &mf.Parser{}
		if _, failure := (JSON{Parser: parser}).DecodePublic([]byte(raw)); failure != sp.Invalid || len(parser.Calls) != 0 {
			t.Fatal("invalid byte envelope reached the parser", failure)
		}
	}
	for _, parser := range []*mf.Parser{{Value: mp.Value{Kind: mp.Array}}, {Issues: []mp.Issue{{Kind: mp.Syntax}}}} {
		if _, failure := (JSON{Parser: parser}).DecodePublic([]byte(`{}`)); failure != sp.Invalid {
			t.Fatal("invalid parsed object accepted", failure)
		}
	}
	if !publicDepth([]byte(strings.Repeat("[", 16) + "0" + strings.Repeat("]", 16))) {
		t.Fatal("valid depth boundary rejected")
	}
}

// Parsed-value fixtures use the standard JSON decoder only to assemble test
// inputs. Cmd's strict-schema suite binds the real manifest parser to this codec.
func publicValue(t *testing.T, data []byte) mp.Value {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	value := mp.Value{Kind: mp.Object, Fields: map[string]mp.Value{}}
	for name, raw := range fields {
		v := mp.Value{Kind: mp.Number, Text: string(raw)}
		if string(raw) == "null" {
			v.Kind = mp.Null
		} else if raw[0] == '"' {
			v.Kind = mp.String
			if err := json.Unmarshal(raw, &v.Text); err != nil {
				t.Fatal(err)
			}
		}
		value.Fields[name] = v
	}
	return value
}
