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
		"%TAG !e! " + strings.Repeat("x", 32768) + "\n---\n[" + strings.Repeat("!e!s 0,", 4678) + "]\n",
		"0\n...\n%TAG !e! " + strings.Repeat("x", 32768) + "\n---\n[" + strings.Repeat("!e!s 0,", 4677) + "]\n",
		"\ufeff# comment\u0085%TAG !e! tag:example:\n---\n!e!value 0",
		"? " + strings.Repeat("/", 59000) + "\n: &a [" + strings.Repeat("*a,", 2045) + "]\n",
		"? " + strings.Repeat("/", 250) + "\n: &a [" + strings.Repeat("*a,", 128) + "]\n",
		"? " + strings.Repeat("/", 250) + "\n: &a [" + strings.Repeat("*a,", 129) + "]\n",
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
		// preflight also bounds their aggregate escaped path bytes, regardless
		// of whether paths share a long key or every node carries custom tags.
		if len(issues) > 4*2048 {
			t.Fatalf("unbounded diagnostic count: %d", len(issues))
		}
		pathBytes := 0
		for _, issue := range issues {
			for _, component := range issue.Path {
				pathBytes += 1 + len(component) + strings.Count(component, "~") + strings.Count(component, "/")
			}
			if len(issue.Path) > 31 {
				t.Fatalf("unbounded diagnostic path: %d", len(issue.Path))
			}
			if issue.Kind == port.Complexity && (len(issues) != 1 || len(issue.Path) != 0) {
				t.Fatal("complexity refusal must be a single root issue")
			}
		}
		if pathBytes > 4*65536 {
			t.Fatalf("unbounded diagnostic path bytes: %d", pathBytes)
		}
	})
}
