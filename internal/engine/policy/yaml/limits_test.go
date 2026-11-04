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

func TestRepresentationPathBudget(t *testing.T) {
	aliases := func(size, count int) string {
		return "? " + strings.Repeat("/", size) + "\n: &a [" + strings.Repeat("*a,", count) + "]\n"
	}
	explicitKey := func(key string) string {
		return "? '" + key + "'\n: 0\n"
	}
	for _, tc := range []struct {
		name, text string
		failure    port.Failure
		issues     int
	}{
		{"plain boundary", explicitKey(strings.Repeat("x", 32767)), port.None, 0},
		{"plain excess", explicitKey(strings.Repeat("x", 32768)), port.Complexity, 1},
		{"slash boundary", explicitKey(strings.Repeat("/", 16383) + "x"), port.None, 0},
		{"slash excess", explicitKey(strings.Repeat("/", 16384)), port.Complexity, 1},
		{"tilde boundary", explicitKey(strings.Repeat("~", 16383) + "x"), port.None, 0},
		{"tilde excess", explicitKey(strings.Repeat("~", 16384)), port.Complexity, 1},
		{"UTF-8 boundary", explicitKey(strings.Repeat("ñ", 16383) + "x"), port.None, 0},
		{"UTF-8 excess", explicitKey(strings.Repeat("ñ", 16384)), port.Complexity, 1},
		// Every path is short enough alone; their sum crosses the budget.
		{"alias fan-out within budget", aliases(250, 128), port.Anchor, 129},
		{"alias fan-out over budget", aliases(250, 129), port.Complexity, 1},
		{"exact review input", aliases(59000, 2045), port.Complexity, 1},
		{"nested mapping boundary", "a:\n  ? '" + strings.Repeat("x", 32763) + "'\n  : 0", port.None, 0},
		{"nested mapping excess", "a:\n  ? '" + strings.Repeat("x", 32764) + "'\n  : 0", port.Complexity, 1},
		{"complex key contents within budget", "? {" + strings.Repeat("key: 0,", 100) + "}\n: 0", port.DuplicateKey, 100},
		{"complex key contents over budget", "? {" + strings.Repeat("'"+strings.Repeat("x", 400)+"': 0,", 100) + "}\n: 0", port.Complexity, 1},
		{"scalar values do not extend paths", "a: '" + strings.Repeat("/", 60000) + "'", port.None, 0},
		{"path budget before scalar validation", "? '" + strings.Repeat("/", 16384) + "'\n: !!bool wrong", port.Complexity, 1},
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

func TestRepresentationPathsIncludeSequenceIndices(t *testing.T) {
	for _, tc := range []struct {
		index, keyLength int
	}{
		// Each has exactly 65536 aggregate path bytes, including the final
		// scalar after the mapping and decimal indices with increasing width.
		{0, 32763}, {10, 32751}, {100, 32614},
	} {
		for _, extra := range []int{0, 1} {
			text := "[" + strings.Repeat("0,", tc.index) + "{? '" + strings.Repeat("x", tc.keyLength+extra) + "': 0},0]"
			_, issues := read(text)
			if extra == 0 && len(issues) != 0 {
				t.Fatalf("index %d: boundary refused", tc.index)
			}
			if extra == 1 && !reflect.DeepEqual(issues, []port.Issue{{Kind: port.Complexity}}) {
				t.Fatalf("index %d: excess not refused", tc.index)
			}
		}
	}
}
