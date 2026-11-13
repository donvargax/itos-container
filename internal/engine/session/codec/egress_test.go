package codec

import (
	"encoding/json"
	mp "github.com/donvargax/itos-container/internal/engine/manifest/port"
	mf "github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"testing"
)

func TestDecodeEgressRecord(t *testing.T) {
	b, _, _ := sf.Fixture()
	b = sf.EgressBinding(b)
	parser := &mf.Parser{}
	codec := JSON{Parser: parser}
	for n, r := range sf.EgressRecords(b)[1:] {
		data := codec.Encode(r)
		parser.Value = egressValue(t, data)
		decoded, f := codec.Decode(data)
		if f != "" || decoded != r {
			t.Errorf("round trip %d = %#v %s", n, decoded, f)
		}
	}
	r := sf.EgressRecords(b)[6]
	data := codec.Encode(r)
	for _, path := range [][]string{{"egress"}, {"anchor"}, {"egress", "internal"}, {"egress", "network"}, {"egress", "proxy"}, {"egress", "proxy", "name"}, {"egress", "proxy", "id"}, {"egress", "proxy", "stage"}} {
		for _, mode := range []string{"missing", "extra", "wrong type"} {
			parser.Value = egressValue(t, data)
			fields := parser.Value.Fields
			for _, key := range path[:len(path)-1] {
				fields = fields[key].Fields
			}
			key := path[len(path)-1]
			switch mode {
			case "missing":
				delete(fields, key)
			case "extra":
				fields["unknown"] = mp.Value{Kind: mp.String}
			case "wrong type":
				fields[key] = mp.Value{Kind: mp.Number, Text: "1"}
			}
			if _, f := codec.Decode(data); f != sp.Invalid {
				t.Errorf("%v %s accepted", path, mode)
			}
		}
	}
	for _, change := range []func(*sp.Record){
		func(r *sp.Record) { r.Egress.Internal.Name = "foreign" },
		func(r *sp.Record) { r.Egress.Network.Stage = "ready" },
		func(r *sp.Record) { r.Egress.Proxy.ID = "short" },
		func(r *sp.Record) { r.Egress.Internal.Stage = "absent" },
		func(r *sp.Record) { r.Stage = "intent"; r.Egress.Proxy.Stage = "intent"; r.Egress.Proxy.ID = "" },
	} {
		bad := r
		change(&bad)
		data, _ := json.Marshal(bad)
		parser.Value = egressValue(t, data)
		if _, f := codec.Decode(data); f != sp.Invalid {
			t.Error("invalid authority accepted", bad)
		}
	}
}

func egressValue(t *testing.T, data []byte) mp.Value {
	t.Helper()
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) == nil && fields != nil {
		v := mp.Value{Kind: mp.Object, Fields: map[string]mp.Value{}}
		for k, raw := range fields {
			v.Fields[k] = egressValue(t, raw)
		}
		return v
	}
	var text string
	if json.Unmarshal(data, &text) == nil {
		return mp.Value{Kind: mp.String, Text: text}
	}
	return mp.Value{Kind: mp.Number, Text: string(data)}
}
