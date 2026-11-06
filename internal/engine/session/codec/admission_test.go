package codec

import (
	"encoding/json"
	"strings"
	"testing"

	mf "github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestAdmissionCodecContract(t *testing.T) {
	for _, complete := range []bool{false, true} {
		r := sp.AdmissionRecord{Schema: 1, Root: "/work/child", ID: strings.Repeat("a", 64), Intent: sp.Reference(strings.Repeat("b", 64), strings.Repeat("c", 64)), Complete: complete}
		data := (JSON{}).EncodeAdmission(r)
		for name, codec := range map[string]sp.PublicCodec{
			"fake": &sf.PublicRecords{Admissions: map[string]sp.AdmissionRecord{string(data): r}},
			"json": JSON{Parser: &mf.Parser{Value: publicValue(t, data)}},
		} {
			t.Run(name, func(t *testing.T) { sf.AdmissionCodecContract(t, codec, r) })
		}
	}
}
func TestAdmissionCodecRejectsInvalidAuthority(t *testing.T) {
	r := sp.AdmissionRecord{Schema: 1, Root: "/work", ID: strings.Repeat("a", 64), Intent: sp.Reference(strings.Repeat("b", 64), strings.Repeat("c", 64))}
	for _, tc := range []struct{ field, raw string }{
		{"schema", "2"}, {"schema", `"1"`}, {"root", `null`}, {"root", `"relative"`}, {"root", `"/work/../other"`},
		{"id", `"short"`}, {"id", `0`}, {"intent", `"invalid"`}, {"intent", `null`}, {"complete", `null`}, {"complete", `"false"`}, {"unknown", `true`},
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
