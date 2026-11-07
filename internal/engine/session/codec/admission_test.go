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

func TestAdmissionCodecContract(t *testing.T) {
	for _, state := range []string{"pending", "active", "complete"} {
		r := sp.AdmissionRecord{Schema: 1, Root: "/work/child", ID: strings.Repeat("a", 64), Intent: sp.Reference(strings.Repeat("b", 64), strings.Repeat("c", 64)), Complete: state == "complete", Pending: state == "pending"}
		data := (JSON{}).EncodeAdmission(r)
		for name, codec := range map[string]sp.PublicCodec{
			"fake": &sf.PublicRecords{Admissions: map[string]sp.AdmissionRecord{string(data): r}},
			"json": JSON{Parser: &mf.Parser{Value: publicValue(t, data)}},
		} {
			t.Run(name, func(t *testing.T) { sf.AdmissionCodecContract(t, codec, r) })
		}
	}
}

func TestAdmissionCodecSizeBoundary(t *testing.T) {
	r := sp.AdmissionRecord{Schema: 1, Root: "/work", ID: strings.Repeat("a", 64), Intent: sp.Reference(strings.Repeat("b", 64), strings.Repeat("c", 64))}
	encoded := (JSON{}).EncodeAdmission(r)
	for _, tc := range []struct {
		name string
		size int
		want sp.Outcome
	}{
		{"below limit", 65535, ""},
		{"at limit", 65536, ""},
		{"above limit", 65537, sp.Invalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Whitespace changes the byte envelope without changing the record.
			data := []byte(string(encoded) + strings.Repeat(" ", tc.size-len(encoded)))
			codec := JSON{Parser: &mf.Parser{Value: publicValue(t, data)}}
			wantRecord := r
			if tc.want != "" {
				wantRecord = sp.AdmissionRecord{}
			}
			if got, failure := codec.DecodeAdmission(data); failure != tc.want || got != wantRecord {
				t.Fatalf("%d-byte admission: got %#v, %s; want %#v, %s", tc.size, got, failure, wantRecord, tc.want)
			}
		})
	}
}

func TestAdmissionCodecRejectsInvalidAuthority(t *testing.T) {
	r := sp.AdmissionRecord{Schema: 1, Root: "/work", ID: strings.Repeat("a", 64), Intent: sp.Reference(strings.Repeat("b", 64), strings.Repeat("c", 64))}
	for _, tc := range []struct{ field, raw string }{
		{"schema", "2"}, {"schema", `"1"`}, {"root", `null`}, {"root", `"relative"`}, {"root", `"/work/../other"`},
		{"id", `"short"`}, {"id", `0`}, {"intent", `"invalid"`}, {"intent", `null`}, {"complete", `null`}, {"complete", `"false"`}, {"unknown", `true`},
		{"pending", `false`}, {"pending", `"true"`}, {"pending", `null`},
	} {
		t.Run(tc.field+tc.raw, func(t *testing.T) {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal((JSON{}).EncodeAdmission(r), &fields); err != nil {
				t.Fatal(err)
			}
			fields[tc.field] = json.RawMessage(tc.raw)
			data, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			codec := JSON{Parser: &mf.Parser{Value: publicValue(t, data)}}
			if value, failure := codec.DecodeAdmission(data); failure != sp.Invalid || value != (sp.AdmissionRecord{}) {
				t.Fatal(value, failure)
			}
		})
	}
	data := (JSON{}).EncodeAdmission(r)
	codec := JSON{Parser: &mf.Parser{Value: publicValue(t, data)}}
	for _, invalid := range [][]byte{nil, []byte("bad"), []byte(strings.Repeat(" ", 65537)), []byte(strings.Repeat("[", 17) + strings.Repeat("]", 17)), []byte(`{"schema":"wrong"}`)} {
		if _, failure := codec.DecodeAdmission(invalid); failure != sp.Invalid {
			t.Fatal("invalid representation accepted", failure)
		}
	}
}

func TestAdmissionCodecRefusesCompletedPendingClaim(t *testing.T) {
	r := sp.AdmissionRecord{Schema: 1, Root: "/work", ID: strings.Repeat("a", 64), Intent: sp.Reference(strings.Repeat("b", 64), strings.Repeat("c", 64)), Pending: true, Complete: true}
	data := (JSON{}).EncodeAdmission(r)
	codec := JSON{Parser: &mf.Parser{Value: publicValue(t, data)}}
	if _, failure := codec.DecodeAdmission(data); failure != sp.Invalid {
		t.Fatal("pending registration certified complete", failure)
	}
}

func TestAdmissionCodecRejectsInvalidParsedObject(t *testing.T) {
	r := sp.AdmissionRecord{Schema: 1, Root: "/work", ID: strings.Repeat("a", 64), Intent: sp.Reference(strings.Repeat("b", 64), strings.Repeat("c", 64))}
	data := (JSON{}).EncodeAdmission(r)
	for _, fault := range []string{"parser issue", "non-object", "missing field", "excess fields"} {
		t.Run(fault, func(t *testing.T) {
			parser := &mf.Parser{Value: publicValue(t, data)}
			switch fault {
			case "parser issue":
				parser.Issues = []mp.Issue{{Kind: mp.Syntax}}
			case "non-object":
				parser.Value.Kind = mp.Array
			case "missing field":
				delete(parser.Value.Fields, "complete")
			case "excess fields":
				parser.Value.Fields["pending"] = mp.Value{Kind: mp.Boolean, Text: "true"}
				parser.Value.Fields["unknown"] = mp.Value{Kind: mp.Boolean, Text: "true"}
			}
			if value, failure := (JSON{Parser: parser}).DecodeAdmission(data); failure != sp.Invalid || value != (sp.AdmissionRecord{}) {
				t.Fatal("invalid parsed admission acquired authority", value, failure)
			}
			if len(parser.Calls) != 1 || !parser.Calls[0].Strict {
				t.Fatal("admission did not use strict parsing", parser.Calls)
			}
		})
	}
}
