package porttest

import (
	"bytes"
	"reflect"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/manifest/port"
)

// Reporter keeps the contracts usable from package tests without test I/O here.
type Reporter interface {
	Helper()
	Errorf(string, ...any)
}

// FilesContract exercises bytes, typed failure, repeated reads and independent
// returned storage. Each factory supplies worker.json, src and a non-directory.
func FilesContract(t Reporter, open func([]byte, port.Failure) port.Files) {
	t.Helper()
	for _, data := range [][]byte{{}, []byte("one\x00two\n")} {
		files := open(data, port.None)
		for range 2 {
			got, failure := files.Read("worker.json")
			if !bytes.Equal(got, data) || failure != port.None {
				t.Errorf("read = %q, %v; want %q, success", got, failure, data)
			}
			for i := range got {
				got[i] ^= 0xff
			}
		}
		if !files.Directory("src") || files.Directory("worker.json") || files.Directory("missing") {
			t.Errorf("Directory did not distinguish existing real directories")
		}
	}
	for _, failure := range []port.Failure{port.Missing, port.Path} {
		files := open(nil, failure)
		for range 2 {
			if data, got := files.Read("worker.json"); data != nil || got != failure {
				t.Errorf("failed read = %q, %v; want no data and %v", data, got, failure)
			}
		}
	}
}

// ParserFixture pairs raw representation and mode with the required parsed data.
type ParserFixture struct {
	Data   string
	Strict bool
	Value  port.Value
	Issues []port.Issue
}

// ParserContract checks decoded duplicates before projection, strict label JSON,
// literal strings and opaque numbers, and caller-independent returned objects.
func ParserContract(t Reporter, open func(ParserFixture) port.Parser) {
	t.Helper()
	fixtures := []ParserFixture{
		{Data: `{"s":"a\n${literal}","n":1e999999,"t":true,"f":false,"z":null,"a":[]}`, Value: port.Value{Kind: port.Object, Fields: map[string]port.Value{
			"s": {Kind: port.String, Text: "a\n${literal}"}, "n": {Kind: port.Number, Text: "1e999999"}, "t": {Kind: port.Boolean, Text: "true"}, "f": {Kind: port.Boolean, Text: "false"}, "z": {}, "a": {Kind: port.Array, Elements: []port.Value{}},
		}}},
		{Data: `/*c*/ {"a":["x",],}`, Value: port.Value{Kind: port.Object, Fields: map[string]port.Value{"a": {Kind: port.Array, Elements: []port.Value{{Kind: port.String, Text: "x"}}}}}},
		{Data: `[{"name":"display"}]`, Strict: true, Value: port.Value{Kind: port.Array, Elements: []port.Value{{Kind: port.Object, Fields: map[string]port.Value{"name": {Kind: port.String, Text: "display"}}}}}},
		{Data: `"// literal /* text */"`, Value: port.Value{Kind: port.String, Text: "// literal /* text */"}},
		{Data: `{"a/b~c":1,"a\u002fb~c":2}`, Issues: []port.Issue{{Kind: port.Duplicate, Path: []string{"a/b~c"}}}},
		{Data: `[{"customizations":{"vscode":{"settings":{"x":1,"\u0078":2}}}}]`, Strict: true, Issues: []port.Issue{{Kind: port.Duplicate, Path: []string{"0", "customizations", "vscode", "settings", "x"}}}},
	}
	for _, data := range []string{"", "// comment", "{", "{} {}", "{'a':1}", "{a:1}", `{"x":NaN}`, `{"x":0x10}`} {
		fixtures = append(fixtures, ParserFixture{Data: data, Issues: []port.Issue{{Kind: port.Syntax}}})
	}
	for _, data := range []string{`[/*c*/{}]`, `[{},]`} {
		fixtures = append(fixtures, ParserFixture{Data: data, Strict: true, Issues: []port.Issue{{Kind: port.Syntax}}})
	}
	for _, fixture := range fixtures {
		parser := open(fixture)
		for range 2 {
			value, issues := parser.Parse([]byte(fixture.Data), fixture.Strict)
			if !reflect.DeepEqual(value, fixture.Value) || !reflect.DeepEqual(issues, fixture.Issues) {
				t.Errorf("Parse(%q, %v) = %#v, %#v; want %#v, %#v", fixture.Data, fixture.Strict, value, issues, fixture.Value, fixture.Issues)
			}
			if value.Fields != nil {
				value.Fields["mutated"] = port.Value{}
			}
			if len(value.Elements) != 0 {
				value.Elements[0] = port.Value{}
			}
			if len(issues) != 0 && len(issues[0].Path) != 0 {
				issues[0].Path[0] = "mutated"
			}
		}
	}
}

// ReferenceContract establishes syntax only, including unqualified names and
// optional tags alongside digests. Fakes supply each expected result as data.
func ReferenceContract(t Reporter, open func(string, bool) port.References) {
	t.Helper()
	digest := "@sha256:" + strings.Repeat("1", 64)
	for _, image := range []string{"worker" + digest, "registry.example/worker" + digest, "registry.example:5000/team/worker:release" + digest} {
		ref := open(image, true)
		for range 2 {
			if !ref.Valid(image) {
				t.Errorf("valid pinned reference refused: %q", image)
			}
		}
	}
	for _, image := range []string{"", "worker", "worker:latest", "worker@sha256:1111", "worker@sha512:" + strings.Repeat("1", 128), "sha256:" + strings.Repeat("1", 64), "Worker" + digest, "registry.example/Worker" + digest, "https://registry.example/worker" + digest, "worker@sha256:" + strings.Repeat("A", 64), "worker@sha256:" + strings.Repeat("g", 64), " worker" + digest, "worker" + digest + " "} {
		ref := open(image, false)
		for range 2 {
			if ref.Valid(image) {
				t.Errorf("invalid pinned reference accepted: %q", image)
			}
		}
	}
}
