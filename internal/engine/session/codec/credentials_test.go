package codec

import (
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
