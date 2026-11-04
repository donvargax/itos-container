package yaml_test

import (
	"reflect"
	"testing"

	"github.com/donvargax/itos-container/internal/policy/port"
	policyyaml "github.com/donvargax/itos-container/internal/policy/yaml"
)

type files struct {
	data    []byte
	failure port.Failure
}

func (f files) Read() ([]byte, port.Failure) { return f.data, f.failure }
func read(text string) (port.Value, []port.Issue) {
	return (policyyaml.Source{Files: files{data: []byte(text)}}).Read()
}

func TestCoreScalarMeanings(t *testing.T) {
	for _, tc := range []struct {
		text    string
		kind    port.Kind
		number  string
		literal string
	}{
		{"!!str 1", port.String, "", "1"}, {"'null'", port.String, "", "null"},
		{"'|−'", port.String, "", "|−"}, {"on", port.String, "", "on"}, {"yes", port.String, "", "yes"},
		{"2026-11-04", port.String, "", "2026-11-04"}, {"1_000", port.String, "", "1_000"},
		{"0b10", port.String, "", "0b10"}, {"0XFF", port.String, "", "0XFF"},
		{"1", port.Number, "1", ""}, {"077", port.Number, "77", ""}, {"0o77", port.Number, "63", ""},
		{"0xFF", port.Number, "255", ""}, {"+0x1", port.String, "", "+0x1"}, {"-0o7", port.String, "", "-0o7"},
		{"1e0", port.Number, "1", ""}, {".5", port.Number, "1/2", ""},
		{"1.00000000000000000001", port.Number, "100000000000000000001/100000000000000000000", ""},
		{"!!float 2", port.Number, "2", ""}, {"!!float 2.5", port.Number, "5/2", ""},
		{"! 1", port.Number, "1", ""}, {"!!int '1'", port.Number, "1", ""},
		{"null", port.Null, "", ""}, {"Null", port.Null, "", ""}, {"NULL", port.Null, "", ""},
		{"~", port.Null, "", ""}, {"!!null ''", port.Null, "", ""},
		{"true", port.Boolean, "", ""}, {"True", port.Boolean, "", ""}, {"TRUE", port.Boolean, "", ""},
		{"false", port.Boolean, "", ""}, {"False", port.Boolean, "", ""}, {"FALSE", port.Boolean, "", ""},
		{"!!bool true", port.Boolean, "", ""},
		{"!!seq [one, two]", port.Array, "", ""}, {"!!map {}", port.Object, "", ""},
		{"!<example:custom> 1", port.Number, "1", ""},
	} {
		t.Run(tc.text, func(t *testing.T) {
			got, issues := read(tc.text)
			if tc.text == "!<example:custom> 1" {
				if !reflect.DeepEqual(issues, []port.Issue{{Kind: port.Tag}}) {
					t.Fatalf("%#v", issues)
				}
				return
			}
			if len(issues) != 0 || got.Kind != tc.kind || got.Text != tc.literal {
				t.Fatalf("got %#v, issues %#v", got, issues)
			}
			if tc.number != "" && got.Number.RatString() != tc.number {
				t.Fatalf("got %s, want %s", got.Number, tc.number)
			}
		})
	}
}

func TestMalformedAndTaggedData(t *testing.T) {
	for _, text := range []string{
		"\xff", "{", "!!int no", "!!bool yes", "!!null no", "!!int .inf",
		"!!map one", "!!seq one", "!!map []", "!!seq {}", "!!str [one]", "!!int {}", "!!float 0x1",
		"!!float word", "1e999999999999999999999", "!!str {a: b}",
		"version: 1\n---\n[", "{extra: [!!bool wrong], version: &a 1}",
	} {
		t.Run(text, func(t *testing.T) {
			_, issues := read(text)
			if !reflect.DeepEqual(issues, []port.Issue{{Kind: port.Syntax}}) {
				t.Fatalf("%#v", issues)
			}
		})
	}
	for _, text := range []string{".nan", ".NaN", ".NAN", ".inf", ".Inf", ".INF", "+.inf", "-.inf", "!!float .inf"} {
		_, issues := read(text)
		if !reflect.DeepEqual(issues, []port.Issue{{Kind: port.NonFinite}}) {
			t.Fatalf("%q: %#v", text, issues)
		}
	}
	for _, text := range []string{"!!timestamp 2026-11-04", "!include a", "!!set {a: null}", "!!binary YQ=="} {
		_, issues := read(text)
		if !reflect.DeepEqual(issues, []port.Issue{{Kind: port.Tag}}) {
			t.Fatalf("%q: %#v", text, issues)
		}
	}
}

func TestStructuralTraversal(t *testing.T) {
	_, issues := read("extra: [one, {a: x, a: y}, {b: 1, b: 2}]")
	want := []port.Issue{{Kind: port.DuplicateKey, Path: []string{"extra", "1", "a"}}, {Kind: port.DuplicateKey, Path: []string{"extra", "2", "b"}}}
	if !reflect.DeepEqual(issues, want) {
		t.Fatalf("%#v", issues)
	}
	_, issues = read("extra: {[{a: x, a: y}]: value}")
	want = []port.Issue{{Kind: port.DuplicateKey, Path: []string{"extra", "0", "a"}}, {Kind: port.NonStringKey, Path: []string{"extra"}}}
	if !reflect.DeepEqual(issues, want) {
		t.Fatalf("%#v", issues)
	}
	_, issues = read("&root {self: *root, '<<': {a: x, a: y}}")
	want = []port.Issue{{Kind: port.Anchor}, {Kind: port.Alias, Path: []string{"self"}}, {Kind: port.MergeKey, Path: []string{"<<"}}, {Kind: port.DuplicateKey, Path: []string{"<<", "a"}}}
	if !reflect.DeepEqual(issues, want) {
		t.Fatalf("%#v", issues)
	}
	got, issues := read("{yes: on, 2026-11-04: no, !!str 1: text}")
	if len(issues) != 0 || got.Fields["yes"].Text != "on" || got.Fields["2026-11-04"].Text != "no" || got.Fields["1"].Text != "text" {
		t.Fatalf("%#v %#v", got, issues)
	}
}

func TestReadBoundaries(t *testing.T) {
	for _, failure := range []port.Failure{port.Missing, port.Unreadable} {
		value, issues := (policyyaml.Source{Files: files{data: []byte("version: 1"), failure: failure}}).Read()
		if !reflect.DeepEqual(issues, []port.Issue{{Kind: failure}}) || !reflect.DeepEqual(value, port.Value{}) {
			t.Fatalf("%#v %#v", value, issues)
		}
	}
	for _, tc := range []struct {
		text    string
		failure port.Failure
	}{
		{"", port.Empty}, {"# comment", port.Empty}, {"---", port.Empty},
		{"---\n---", port.MultipleDocuments}, {"null\n---", port.MultipleDocuments},
	} {
		_, issues := read(tc.text)
		if !reflect.DeepEqual(issues, []port.Issue{{Kind: tc.failure}}) {
			t.Fatalf("%q: %#v", tc.text, issues)
		}
	}
}
