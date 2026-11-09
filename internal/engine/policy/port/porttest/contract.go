package porttest

import (
	"bytes"
	"reflect"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/policy/port"
)

// Reporter is the test output used by the shared contracts.
type Reporter interface {
	Helper()
	Errorf(string, ...any)
}

// FilesContract checks byte reads and failures against the fake and disk adapter.
// The factory represents Unreadable with a directory, not permission bits.
func FilesContract(t Reporter, open func([]byte, port.Failure) port.Files) {
	t.Helper()
	for _, data := range [][]byte{{}, []byte("one\x00two\n")} {
		files := open(data, port.None)
		first, failure := files.Read()
		if failure != port.None || !bytes.Equal(first, data) {
			t.Errorf("Read() = %q, %v; want %q, no failure", first, failure, data)
		}
		second, failure := files.Read()
		if failure != port.None || !bytes.Equal(second, data) {
			t.Errorf("repeat Read() = %q, %v; want %q", second, failure, data)
		}
		for i := range first {
			first[i] ^= 0xff
		}
		again, failure := files.Read()
		if failure != port.None || !bytes.Equal(second, data) || !bytes.Equal(again, data) {
			t.Errorf("Read() shares returned storage or retained caller mutation")
		}
	}
	for _, failure := range []port.Failure{port.Missing, port.Unreadable} {
		files := open(nil, failure)
		for range 2 {
			data, got := files.Read()
			if data != nil || got != failure {
				t.Errorf("Read() = %q, %v; want no data, %v", data, got, failure)
			}
		}
	}
}

// SourceFixture pairs raw input with the representation outcome it must produce.
// Adapters consume Data/Failure; the fake supplies Value/Issues without parsing.
type SourceFixture struct {
	Data    string
	Failure port.Failure
	Value   port.Value
	Issues  []port.Issue
}

// SourceContract checks representation validation before domain inspection.
func SourceContract(t Reporter, open func(SourceFixture) port.Source) {
	t.Helper()
	fixtures := []SourceFixture{
		{Data: "version: 1.0\nmanifest: a.json", Value: port.Value{Kind: port.Object, Fields: map[string]port.Value{
			"version": {Kind: port.Number, Number: port.Decimal{Coefficient: "1", Exponent: "0"}}, "manifest": {Kind: port.String, Text: "a.json"},
		}}},
		{Data: "0xFF", Value: port.Value{Kind: port.Number, Number: port.Decimal{Coefficient: "255", Exponent: "0"}}},
		{Data: "1.00000000000000000001", Value: port.Value{Kind: port.Number, Number: port.Decimal{Coefficient: "100000000000000000001", Exponent: "-20"}}},
		{Data: "10e999999999999999999999", Value: port.Value{Kind: port.Number, Number: port.Decimal{Coefficient: "1", Exponent: "1000000000000000000000"}}},
		{Data: "0e-999999999999999999999", Value: port.Value{Kind: port.Number, Number: port.Decimal{Coefficient: "0", Exponent: "0"}}},
		{Data: "network: {allow: [a, 7, [], [x], {k: v}]}\n", Value: port.Value{Kind: port.Object, Fields: map[string]port.Value{
			"network": {Kind: port.Object, Fields: map[string]port.Value{"allow": {Kind: port.Array, Items: []port.Value{
				{Kind: port.String, Text: "a"}, {Kind: port.Number, Number: port.Decimal{Coefficient: "7", Exponent: "0"}}, {Kind: port.Array},
				{Kind: port.Array, Items: []port.Value{{Kind: port.String, Text: "x"}}},
				{Kind: port.Object, Fields: map[string]port.Value{"k": {Kind: port.String, Text: "v"}}},
			}}}},
		}}},
		{Data: "on", Value: port.Value{Kind: port.String, Text: "on"}},
		{Data: "null", Value: port.Value{Kind: port.Null}},
		{Data: "", Issues: []port.Issue{{Kind: port.Empty}}},
		{Data: "{", Issues: []port.Issue{{Kind: port.Syntax}}},
		{Data: "a: one\na: two", Issues: []port.Issue{{Kind: port.DuplicateKey, Path: []string{"a"}}}},
		{Data: "null\n---\nnull", Issues: []port.Issue{{Kind: port.MultipleDocuments}}},
		{Data: strings.Repeat("!x [", 32) + "0" + strings.Repeat("]", 32), Issues: []port.Issue{{Kind: port.Complexity}}},
		{Data: "[" + strings.Repeat("0,", 2048) + "]", Issues: []port.Issue{{Kind: port.Complexity}}},
		{Data: "sensitive", Failure: port.Missing, Issues: []port.Issue{{Kind: port.Missing}}},
		{Data: "sensitive", Failure: port.Unreadable, Issues: []port.Issue{{Kind: port.Unreadable}}},
	}
	for _, fixture := range fixtures {
		source := open(fixture)
		for range 2 {
			value, issues := source.Read()
			if !reflect.DeepEqual(value, fixture.Value) || !reflect.DeepEqual(issues, fixture.Issues) {
				t.Errorf("Read(%q) = %#v, %#v; want %#v, %#v", fixture.Data, value, issues, fixture.Value, fixture.Issues)
			}
		}
	}
}

// NameFixture pairs a spelling with the answer every Names implementation must give.
// The adapter consumes Spelling; the fake is built from the table the fixtures make.
type NameFixture struct {
	Spelling, ASCII string
	OK              bool
}

// NameFixtures are the spellings the contract holds. Some pin what the library
// accepts and the domain therefore has to refuse: its shape checks are not redundant.
func NameFixtures() []NameFixture {
	return []NameFixture{
		{"api.example.com", "api.example.com", true},
		{"API.Example.COM.", "api.example.com.", true},
		{"bücher.example", "xn--bcher-kva.example", true},
		{"XN--BCHER-KVA.example", "xn--bcher-kva.example", true},
		{"日本。example。", "xn--wgv71a.example.", true},
		{"faß.example", "xn--fa-hia.example", true},
		{"\u0430pple.example", "xn--pple-43d.example", true},
		{"ＡＰＩ.example.com", "api.example.com", true},
		{"exa\u00ADmple.com", "example.com", true},
		{"127.0.0.1", "127.0.0.1", true},
		{"127.1", "127.1", true},
		{"0x7f.0.0.1", "0x7f.0.0.1", true},
		{"1.2.3.0X4", "1.2.3.0x4", true},
		{"example", "example", true},
		{".example.com", ".example.com", true},
		{"example.com..", "example.com..", true},
		{"xn--.example", ".example", true},
		{"example.com.\u00AD", "example.com.", true},
		{"example.\u05D0", "example.xn--4db", true},
		{"xn--", "", false},
		{"a.example.xn--", "", false},
		{"a.example.XN--", "", false},
		{"a\u3002example\u3002xn--", "", false},
		{"a.example.\uFF58\uFF4E\uFF0D\uFF0D", "", false},
		{"a.example.xn--\u00AD", "", false},
		{".xn--", "", false},
		{strings.Repeat("a", 64) + ".example", strings.Repeat("a", 64) + ".example", true},
		{"", "", true},
		{"*.example.com", "", false},
		{"ex*mple.com", "", false},
		{"exa mple.com", "", false},
		{"example.com:443", "", false},
		{"example.com\uFF1A443", "", false},
		{"https://example.com", "", false},
		{"::1", "", false},
		{"[::1]", "", false},
		{"%65xample.com", "", false},
		{"_dmarc.example.com", "", false},
		{"-example.com", "", false},
		{"example-.com", "", false},
		{"ab--cd.example", "", false},
		{"xn--a.example", "", false},
		{"xn--b\u00FCcher.example", "", false},
		{"exa\u200Dmple.com", "", false},
		{"\u05D0a.example", "", false},
		{"example.com\n", "", false},
		{"exa\x00mple.com", "", false},
	}
}

// NamesContract checks canonicalising against the fake and the library adapter:
// the same spelling gives the same answer each time, and a refusal carries no text.
func NamesContract(t Reporter, open func([]NameFixture) port.Names) {
	t.Helper()
	fixtures := NameFixtures()
	names := open(fixtures)
	for range 2 {
		for _, fixture := range fixtures {
			ascii, ok := names.ASCII(fixture.Spelling)
			if ascii != fixture.ASCII || ok != fixture.OK {
				t.Errorf("ASCII(%q) = %q, %v; want %q, %v", fixture.Spelling, ascii, ok, fixture.ASCII, fixture.OK)
			}
		}
	}
}
