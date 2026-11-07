package codec

import (
	"encoding/json"
	"strings"
	"testing"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
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
		func(r *sp.Record) { r.Stage, r.ContainerID = "reserved", strings.Repeat("2", 64) },
		func(r *sp.Record) { r.Stage, r.ContainerID = "cancelled", strings.Repeat("2", 64) },
	} {
		bad := record
		mutate(&bad)
		raw := codec.Encode(bad)
		parser.Value = recordValue(t, raw)
		if _, f = codec.Decode(raw); f != sp.Invalid {
			t.Fatal("invalid progress accepted", bad)
		}
	}
	for _, stage := range []string{"owned", "absent", "reserved", "cancelled"} {
		valid := record
		valid.Stage = stage
		if stage == "owned" || stage == "absent" {
			valid.ContainerID = strings.Repeat("2", 64)
		}
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

func TestRecordCodecRejectsUndecodableBytesAfterParserSuccess(t *testing.T) {
	b, _, _ := sf.Fixture()
	record := b.Intent(strings.Repeat("1", 64))
	parser := &mf.Parser{}
	codec := JSON{Parser: parser}
	data := codec.Encode(record)
	parser.Value = recordValue(t, data)
	// Keep the parser's accepted shape to isolate the subsequent typed decoder.
	// A successful parser observation alone must not hand out a partial record.
	for _, invalid := range [][]byte{data[:len(data)-1], []byte(strings.Replace(string(data), `"schema":1`, `"schema":1.0`, 1))} {
		if got, failure := codec.Decode(invalid); failure != sp.Invalid || got != (sp.Record{}) {
			t.Fatalf("undecodable record = %#v, %s", got, failure)
		}
	}
}

func TestFrozenAndSnapshotRequireValidatedApprovalEnvelope(t *testing.T) {
	b, _, _ := sf.Fixture()
	settings, err := json.Marshal(map[string]any{
		"checkout": map[string]string{"root": b.Checkout.Root, "device": "1", "inode": "2", "git_device": "1", "git_inode": "3"},
		"engine":   map[string]string{"endpoint": b.Endpoint, "id": b.EngineID},
		"image":    map[string]string{"reference": b.ImageReference, "id": b.ImageID},
	})
	if err != nil {
		t.Fatal(err)
	}
	b.Frozen.Settings = string(settings)
	b.Checkout.Device, b.Checkout.Inode, b.Checkout.GitDevice, b.Checkout.GitInode = "1", "2", "1", "3"
	record := ap.Record{Candidate: ap.Candidate{Project: b.Frozen.Project, Fingerprint: b.Frozen.Fingerprint, Canonical: b.Frozen.Settings, UID: b.UID, GID: b.GID, Root: b.Checkout.Root}, Generation: b.Frozen.Generation}
	data, err := json.Marshal(map[string]any{"schema": 1, "project": b.Frozen.Project, "generation": b.Frozen.Generation, "fingerprint": b.Frozen.Fingerprint, "settings": json.RawMessage(settings)})
	if err != nil {
		t.Fatal(err)
	}
	approval := af.FrozenRecords{Values: map[ap.Frozen]ap.Record{b.Frozen: record}, Bytes: map[ap.Record]string{record: string(data)}}
	codec := JSON{Approval: approval}
	if got, failure := codec.Frozen(b.Frozen); failure != "" || got != b {
		t.Fatalf("valid frozen binding = %#v, %s", got, failure)
	}
	if got, failure := codec.Snapshot(data); failure != "" || got != b {
		t.Fatalf("valid snapshot binding = %#v, %s", got, failure)
	}
	invalid := b.Frozen
	invalid.Fingerprint = "wrong"
	if got, failure := codec.Frozen(invalid); failure != sp.Invalid || got != (sp.Binding{}) {
		t.Fatalf("invalid frozen binding = %#v, %s", got, failure)
	}
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"malformed JSON", []byte("{")},
		{"oversize envelope", make([]byte, 1024*1024+1)},
		{"invalid frozen binding", []byte(strings.Replace(string(data), b.Frozen.Fingerprint, "wrong", 1))},
		{"noncanonical envelope", append(append([]byte{}, data...), '\n')},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, failure := codec.Snapshot(tc.data); failure != sp.Invalid || got != (sp.Binding{}) {
				t.Fatalf("invalid snapshot binding = %#v, %s", got, failure)
			}
		})
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
