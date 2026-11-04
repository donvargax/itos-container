package yaml_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/policy/port"
)

func FuzzSourceRead(f *testing.F) {
	for _, seed := range []string{
		"", "\xff", "{", "version: 1\nmanifest: a.json", "null\n---\nnull",
		"&root {self: *root}", "{a: 0, a: 1}", "? [a, b]\n: value",
		"{extra: [!!bool wrong, .inf, !x 1]}", "1e-999999999999999999999",
		"extra: " + strings.Repeat("!x [", 6000) + "0" + strings.Repeat("]", 6000),
		"extra: " + strings.Repeat("[", 8000) + "0" + strings.Repeat("]", 8000),
		strings.Repeat("[", 31) + "0" + strings.Repeat("]", 31),
		"[" + strings.Repeat("0,", 2047) + "]",
		"[" + strings.Repeat("0,", 2048) + "]",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		// Match the maximum input delivered by the bounded files adapter.
		if len(input) > 65536 {
			t.Skip()
		}
		value, issues := read(input)
		if len(issues) > 0 && !reflect.DeepEqual(value, port.Value{}) {
			t.Fatal("failure returned a partial value")
		}
		// A node can report only a fixed number of structural failures. The
		// depth limit bounds retained paths even when every node has a tag.
		if len(issues) > 4*2048 {
			t.Fatalf("unbounded diagnostic count: %d", len(issues))
		}
		for _, issue := range issues {
			if len(issue.Path) > 31 {
				t.Fatalf("unbounded diagnostic path: %d", len(issue.Path))
			}
			if issue.Kind == port.Complexity && (len(issues) != 1 || len(issue.Path) != 0) {
				t.Fatal("complexity refusal must be a single root issue")
			}
		}
	})
}
