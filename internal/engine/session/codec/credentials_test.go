package codec

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	mp "github.com/donvargax/itos-container/internal/engine/manifest/port"
	mf "github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"testing"
)

func TestCredentialRecords(t *testing.T) {
	parser := &mf.Parser{}
	codec := JSON{Parser: parser}
	r := f.CredentialIntent(port.CredentialKinds{Claude: true, GitHub: true})
	for _, stage := range []string{"intent", "owned", "absent"} {
		r.Credentials.Stage = stage
		if stage != "intent" {
			r.Credentials.Device = "1"
			r.Credentials.Inode = "2"
		}
		data := codec.Encode(r)
		parser.Value = credentialValue(t, data)
		if got, fail := codec.Decode(data); fail != "" || got != r {
			t.Fatal("credential metadata round trip refused", fail)
		}
		credential := parser.Value.Fields["credentials"]
		for name, saved := range credential.Fields {
			delete(credential.Fields, name)
			if _, fail := codec.Decode(data); fail != port.Invalid {
				t.Error("missing credential field accepted", name)
			}
			credential.Fields[name] = mp.Value{Kind: mp.Null}
			if _, fail := codec.Decode(data); fail != port.Invalid {
				t.Error("mistyped credential field accepted", name)
			}
			credential.Fields[name] = saved
		}
		credential.Fields["secret"] = mp.Value{Kind: mp.String, Text: "unrecognized"}
		if _, fail := codec.Decode(data); fail != port.Invalid {
			t.Error("unknown credential field accepted")
		}
	}
}

func TestCredentialRecordsRejectEmptyAndSingleByteInputs(t *testing.T) {
	r := f.CredentialIntent(port.CredentialKinds{Claude: true})
	parser := &mf.Parser{}
	codec := JSON{Parser: parser}
	data := codec.Encode(r)
	parser.Value = credentialValue(t, data)
	if got, failure := codec.Decode(data); failure != "" || got != r {
		t.Fatal("valid credential record control failed", failure)
	}
	// Even a parser reporting every required field cannot turn these bytes
	// into a record. JSON decoding independently refuses them.
	inputs := [][]byte{nil}
	for value := range 256 {
		inputs = append(inputs, []byte{byte(value)})
	}
	for _, input := range inputs {
		if got, failure := codec.Decode(input); failure != port.Invalid || got != (port.Record{}) {
			t.Fatal("empty or single-byte input produced a credential record")
		}
	}
}

func TestCredentialRecordsSizeBoundary(t *testing.T) {
	r := f.CredentialIntent(port.CredentialKinds{Claude: true})
	parser := &mf.Parser{}
	codec := JSON{Parser: parser}
	data := codec.Encode(r)
	parser.Value = credentialValue(t, data)
	// JSON whitespace keeps a complete valid record at the wire-size limit.
	data = append(data, bytes.Repeat([]byte(" "), 65536-len(data))...)
	if got, failure := codec.Decode(data); failure != "" || got != r {
		t.Fatal("credential record at the size limit was refused", failure)
	}
	if got, failure := codec.Decode(append(data, ' ')); failure != port.Invalid || got != (port.Record{}) {
		t.Fatal("oversized credential record was accepted")
	}
}

func TestCredentialRecordsNestedTypes(t *testing.T) {
	r := f.CredentialIntent(port.CredentialKinds{Claude: true, GitHub: true})
	parser := &mf.Parser{}
	codec := JSON{Parser: parser}
	data := codec.Encode(r)
	parser.Value = credentialValue(t, data)
	if got, fail := codec.Decode(data); fail != "" || got != r {
		t.Fatal("healthy credential record refused", fail)
	}
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"numeric file identity", func(c map[string]any) { c["files"] = []any{1, ""} }},
		{"null file identity", func(c map[string]any) { c["files"] = []any{"", nil} }},
		{"short files array", func(c map[string]any) { c["files"] = []any{""} }},
		{"extra file identity", func(c map[string]any) { c["files"] = []any{"", "", ""} }},
		{"string Claude kind", func(c map[string]any) { c["kinds"].(map[string]any)["Claude"] = "true" }},
		{"numeric GitHub kind", func(c map[string]any) { c["kinds"].(map[string]any)["GitHub"] = 1 }},
		{"unknown kind", func(c map[string]any) {
			k := c["kinds"].(map[string]any)
			delete(k, "GitHub")
			k["Other"] = true
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var wire map[string]any
			if json.Unmarshal(data, &wire) != nil {
				t.Fatal("credential record fixture unreadable")
			}
			tc.change(wire["credentials"].(map[string]any))
			bad, err := json.Marshal(wire)
			if err != nil {
				t.Fatal("credential record fixture cannot be encoded")
			}
			parser.Value = credentialValue(t, bad)
			if got, fail := codec.Decode(bad); fail != port.Invalid || got != (port.Record{}) {
				t.Fatal("malformed nested credential metadata accepted")
			}
		})
	}
}

func TestCredentialRecordsWithEgress(t *testing.T) {
	b, _, _ := f.Fixture()
	r := f.CredentialIntent(port.CredentialKinds{Claude: true, GitHub: true})
	r.Egress = f.EgressRecords(f.EgressBinding(b))[6].Egress
	parser := &mf.Parser{}
	codec := JSON{Parser: parser}
	data := codec.Encode(r)
	parser.Value = credentialValue(t, data)
	if got, fail := codec.Decode(data); fail != "" || got != r {
		t.Fatal("credential record with egress authority refused", fail)
	}
	var wire map[string]any
	if json.Unmarshal(data, &wire) != nil {
		t.Fatal("credential record fixture unreadable")
	}
	wire["egress"].(map[string]any)["proxy"].(map[string]any)["stage"] = true
	bad, err := json.Marshal(wire)
	if err != nil {
		t.Fatal("credential record fixture cannot be encoded")
	}
	parser.Value = credentialValue(t, bad)
	if got, fail := codec.Decode(bad); fail != port.Invalid || got != (port.Record{}) {
		t.Fatal("malformed egress metadata in credential record accepted")
	}
}

func TestCredentialRecordRetainsHome(t *testing.T) {
	r := f.CredentialIntent(port.CredentialKinds{Claude: true})
	scope := homeScopeFixture()
	r.Home = scope.Generations[0].Reference(scope.Scope)
	data := (JSON{}).Encode(r)
	codec := JSON{Parser: &mf.Parser{Value: credentialValue(t, data)}}
	if got, failure := codec.Decode(data); failure != "" || got != r {
		t.Fatalf("credential session failed to round-trip its home reference: %s", failure)
	}
}

func TestCredentialRecordsRejectUnknownAnchorField(t *testing.T) {
	r := f.CredentialIntent(port.CredentialKinds{Claude: true})
	parser := &mf.Parser{}
	codec := JSON{Parser: parser}
	data := codec.Encode(r)
	parser.Value = credentialValue(t, data)
	if got, failure := codec.Decode(data); failure != "" || got != r {
		t.Fatal("valid credential anchor record refused", failure)
	}
	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal("credential record fixture unreadable")
	}
	wire["anchor"].(map[string]any)["unrecognized"] = "extra"
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatal("credential record fixture cannot be encoded")
	}
	parser.Value = credentialValue(t, data)
	if got, failure := codec.Decode(data); failure != port.Invalid || got != (port.Record{}) {
		t.Fatal("credential record accepted an unknown anchor field")
	}
}

func credentialValue(t *testing.T, data []byte) mp.Value {
	t.Helper()
	if string(data) == "null" {
		return mp.Value{Kind: mp.Null}
	}
	var array []json.RawMessage
	if json.Unmarshal(data, &array) == nil && array != nil {
		v := mp.Value{Kind: mp.Array}
		for _, raw := range array {
			v.Elements = append(v.Elements, credentialValue(t, raw))
		}
		return v
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(data, &obj) == nil && obj != nil {
		v := mp.Value{Kind: mp.Object, Fields: map[string]mp.Value{}}
		for k, raw := range obj {
			v.Fields[k] = credentialValue(t, raw)
		}
		return v
	}
	if string(data) == "true" || string(data) == "false" {
		return mp.Value{Kind: mp.Boolean, Text: string(data)}
	}
	return egressValue(t, data)
}

func TestCredentialCodecOutputAbsence(t *testing.T) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal("marker setup failed")
	}
	marker := []byte("itos-synthetic-credential-" + hex.EncodeToString(random[:]))
	r := f.CredentialIntent(port.CredentialKinds{Claude: true})
	codec := JSON{Parser: &mf.Parser{}}
	if !f.CredentialMarkerPresent(marker, marker) {
		t.Fatal("codec detector positive control failed")
	}
	data := codec.Encode(r)
	if f.CredentialMarkerPresent(data, marker) {
		t.Fatal("credential marker in encoded metadata")
	}
	var object map[string]any
	if json.Unmarshal(data, &object) != nil {
		t.Fatal("record setup failed")
	}
	object["secret"] = string(marker)
	unknown, err := json.Marshal(object)
	if err != nil {
		t.Fatal("record setup failed")
	}
	codec.Parser.(*mf.Parser).Value = credentialValue(t, unknown)
	_, failure := codec.Decode(unknown)
	if failure != port.Invalid || f.CredentialMarkerPresent([]byte(failure), marker) {
		t.Fatal("unknown content field accepted or exposed in failure")
	}
}
