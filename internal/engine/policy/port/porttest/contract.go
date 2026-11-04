package porttest

import (
	"bytes"
	"reflect"

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
		{Data: "on", Value: port.Value{Kind: port.String, Text: "on"}},
		{Data: "null", Value: port.Value{Kind: port.Null}},
		{Data: "", Issues: []port.Issue{{Kind: port.Empty}}},
		{Data: "{", Issues: []port.Issue{{Kind: port.Syntax}}},
		{Data: "a: one\na: two", Issues: []port.Issue{{Kind: port.DuplicateKey, Path: []string{"a"}}}},
		{Data: "null\n---\nnull", Issues: []port.Issue{{Kind: port.MultipleDocuments}}},
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
