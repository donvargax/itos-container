package yaml_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
	policyyaml "github.com/donvargax/itos-container/internal/engine/policy/yaml"
)

func blockMappings(count int) string {
	var text strings.Builder
	for i := range count {
		text.WriteString(strings.Repeat("  ", i) + "a:\n")
	}
	return text.String() + strings.Repeat("  ", count)
}

func TestParsingLimitsAndPrecedence(t *testing.T) {
	excess := strings.Repeat("[", 33)
	nodes := "[" + strings.Repeat("0,", 2048) + "]\n"
	paths := "? '" + strings.Repeat("a", 32768) + "'\n: 0\n"
	depth := strings.Repeat("[", 32) + "0" + strings.Repeat("]", 32) + "\n"
	for _, tc := range []struct {
		name, text string
		failure    port.Failure
	}{
		{"flow at limit unfinished", strings.Repeat("[", 32), port.Syntax},
		{"flow over limit unfinished", excess, port.Complexity},
		{"block at limit unfinished", blockMappings(32) + "[", port.Syntax},
		{"block over limit unfinished", blockMappings(33) + "[", port.Complexity},
		{"mixed stacks independently at limit", blockMappings(32) + strings.Repeat("[", 32), port.Syntax},
		{"mixed flow excess", blockMappings(1) + excess, port.Complexity},
		{"mixed representation limit", blockMappings(16) + strings.Repeat("[", 15) + "0" + strings.Repeat("]", 15), port.None},
		{"mixed representation excess", blockMappings(16) + strings.Repeat("[", 16) + "0" + strings.Repeat("]", 16), port.Complexity},
		{"scalar indentation is not a collection", blockMappings(31) + strings.Repeat(" ", 300) + "0", port.None},
		{"flow siblings unwind", "[" + strings.Repeat("[],", 100) + "]", port.None},
		{"block siblings unwind", strings.Repeat("- a: 0\n", 100), port.None},
		{"quoted punctuation", "'" + excess + "'", port.None},
		{"block scalar punctuation", "|\n  " + excess + " &a *a !x %TAG\n", port.None},
		{"comment punctuation", "0 # " + excess + " &a *a !x %TAG\n", port.None},
		{"syntax before stack excess", "]\n" + excess, port.Syntax},
		{"syntax before representation check", nodes + "[", port.Syntax},
		{"UTF-8 before stack excess", excess + "\xff", port.Syntax},
		{"directive before stack excess", excess + "\n%TAG !x! tag:example:\n", port.Syntax},
		{"UTF-8 before representation excess", nodes + "---\n\xff", port.Syntax},
		{"directive before representation excess", nodes + "---\n%YAML 1.2\n", port.Syntax},
		{"depth before second syntax", depth + "---\n[", port.Complexity},
		{"nodes before second syntax", nodes + "---\n[", port.Complexity},
		{"paths before second syntax", paths + "---\n[", port.Complexity},
		{"nodes before complete second", nodes + "---\n0", port.Complexity},
		{"paths before complete second", paths + "---\n0", port.Complexity},
		{"second document stack excess", "0\n---\n" + excess, port.Complexity},
		{"second syntax before stack excess", "0\n---\n]\n" + excess, port.Syntax},
		{"complete second document", "0\n---\n0", port.MultipleDocuments},
		{"second representation is not projected", "0\n---\n" + nodes, port.MultipleDocuments},
		{"second tags are not validated", "0\n---\n!!bool wrong", port.MultipleDocuments},
		{"second aliases are not expanded", "0\n---\n&a [*a]", port.MultipleDocuments},
		{"second syntax before first structural error", "&a 0\n---\n[", port.Syntax},
		{"second stack before first structural error", "&a 0\n---\n" + excess, port.Complexity},
		{"second document before first structural error", "&a 0\n---\n0", port.MultipleDocuments},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, issues := read(tc.text)
			if tc.failure == port.None {
				if len(issues) != 0 {
					t.Fatalf("unexpected issues: %#v", issues)
				}
				return
			}
			if !reflect.DeepEqual(issues, []port.Issue{{Kind: tc.failure}}) || !reflect.DeepEqual(value, port.Value{}) {
				t.Fatalf("got %#v, %#v; want no value and one root %v issue", value, issues, tc.failure)
			}
		})
	}
}

func TestParsingBudgetIsLocalToInspection(t *testing.T) {
	files := &porttest.Files{Data: []byte(strings.Repeat("[", 33))}
	source := policyyaml.Source{Files: files}
	for range 2 {
		value, issues := source.Read()
		if !reflect.DeepEqual(issues, []port.Issue{{Kind: port.Complexity}}) || !reflect.DeepEqual(value, port.Value{}) {
			t.Fatalf("exhausted read: %#v, %#v", value, issues)
		}
	}
	files.Data = []byte("version: 1\nmanifest: a.json\n")
	value, issues := source.Read()
	if len(issues) != 0 || value.Fields["manifest"].Text != "a.json" {
		t.Fatalf("valid read after exhaustion: %#v, %#v", value, issues)
	}
}
