package domain

import (
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"reflect"
	"testing"
)

func TestDifferencesKeepSortedAtomicValuesAndEscapedPaths(t *testing.T) {
	before := map[string]any{"policy": map[string]any{"network": map[string]any{"allow": []any{"a.example", "b.example"}}}, "manifest": map[string]any{"containerEnv": map[string]any{"APP": "old"}}, "a/b~c": true, "gone": "old"}
	after := map[string]any{"policy": map[string]any{"network": map[string]any{"allow": []any{"a.example"}}}, "manifest": map[string]any{"containerEnv": map[string]any{"APP": "new"}}, "a/b~c": false, "proxy": map[string]any{"image": "pinned"}}
	want := []port.Change{
		{Path: "/a~1b~0c", Before: true, After: false},
		{Path: "/gone", Before: "old", After: nil},
		{Path: "/manifest/containerEnv", Before: map[string]any{"APP": "old"}, After: map[string]any{"APP": "new"}},
		{Path: "/policy/network/allow", Before: []any{"a.example", "b.example"}, After: []any{"a.example"}},
		{Path: "/proxy", Before: nil, After: map[string]any{"image": "pinned"}},
	}
	if got := Differences(before, after); !reflect.DeepEqual(got, want) {
		t.Fatalf("%#v", got)
	}
	if got := Differences(before, before); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestDifferencesCompareDecodedJSONTypes(t *testing.T) {
	for _, tc := range []struct {
		before, after any
		same          bool
	}{
		{int(1), int(1), false}, // An unsupported value must not silently compare equal.
		{nil, nil, true}, {nil, "", false}, {"a", "a", true}, {"a", "b", false}, {"a", true, false},
		{float64(1), float64(1), true}, {float64(1), float64(2), false}, {float64(1), "1", false},
		{true, true, true}, {true, false, false}, {true, "true", false},
		{[]any{"a"}, []any{"a"}, true}, {[]any{"a"}, []any{"b"}, false}, {[]any{}, []any{"a"}, false}, {[]any{}, "", false},
		{map[string]any{"x": nil}, map[string]any{"x": nil}, true},
		{map[string]any{"x": nil}, map[string]any{"y": "value"}, false},
		{map[string]any{}, map[string]any{"x": "value"}, false}, {map[string]any{}, nil, false},
	} {
		got := Differences(tc.before, tc.after)
		if (len(got) == 0) != tc.same {
			t.Errorf("%#v -> %#v = %#v", tc.before, tc.after, got)
		}
	}
}
