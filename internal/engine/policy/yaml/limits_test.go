package yaml_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/policy/port"
)

func TestRepresentationLimits(t *testing.T) {
	nested := func(open, close string, count int) string {
		return strings.Repeat(open, count) + "0" + strings.Repeat(close, count)
	}
	for _, tc := range []struct {
		name, text string
		failure    port.Failure
		issues     int
	}{
		{"sequence depth 32", nested("[", "]", 31), port.None, 0},
		{"sequence depth 33", nested("[", "]", 32), port.Complexity, 1},
		{"mapping depth 32", nested("{a: ", "}", 31), port.None, 0},
		{"mapping depth 33", nested("{a: ", "}", 32), port.Complexity, 1},
		{"key depth 32", "? " + nested("[", "]", 30) + "\n: value", port.NonStringKey, 1},
		{"key depth 33", "? " + nested("[", "]", 31) + "\n: value", port.Complexity, 1},
		{"2048 nodes", "[" + strings.Repeat("0,", 2047) + "]", port.None, 0},
		{"2049 nodes", "[" + strings.Repeat("0,", 2048) + "]", port.Complexity, 1},
		{"2048 nodes including keys", "[{" + strings.Repeat("a: 0,", 1023) + "}]", port.DuplicateKey, 1022},
		{"2049 nodes including keys", "[{" + strings.Repeat("a: 0,", 1023) + "},0]", port.Complexity, 1},
		{"2048 nodes including aliases", "&root [" + strings.Repeat("*root,", 2047) + "]", port.Anchor, 2048},
		{"2049 nodes including aliases", "&root [" + strings.Repeat("*root,", 2048) + "]", port.Complexity, 1},
		{"tagged review input", "extra: " + nested("!x [", "]", 6000), port.Complexity, 1},
		{"untagged review input", "extra: " + nested("[", "]", 6000), port.Complexity, 1},
		{"deeper review input", "extra: " + nested("[", "]", 8000), port.Complexity, 1},
		{"budget before scalar validation", strings.Repeat("[", 32) + "!!bool wrong" + strings.Repeat("]", 32), port.Complexity, 1},
		{"syntax before budget", nested("[", "]", 32) + "[", port.Syntax, 1},
		{"documents before budget", nested("[", "]", 32) + "\n---\n0", port.MultipleDocuments, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, issues := read(tc.text)
			if len(issues) != tc.issues || (len(issues) > 0 && issues[0].Kind != tc.failure) {
				t.Fatalf("got %d issues; want %d with first kind %v", len(issues), tc.issues, tc.failure)
			}
			if len(issues) > 0 && !reflect.DeepEqual(value, port.Value{}) {
				t.Fatal("failure returned a partial value")
			}
			if tc.failure == port.Complexity && len(issues[0].Path) != 0 {
				t.Fatal("complexity refusal must be at the root")
			}
		})
	}
}
