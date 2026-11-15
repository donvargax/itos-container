package codec

import (
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

func credentialValue(t *testing.T, data []byte) mp.Value {
	t.Helper()
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
