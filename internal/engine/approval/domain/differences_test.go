package domain

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
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
		name          string
		before, after any
		same          bool
	}{
		{"unsupported value", int(1), int(1), false},
		{"null equal", nil, nil, true},
		{"null to empty string", nil, "", false},
		{"string equal", "a", "a", true},
		{"empty string equal", "", "", true},
		{"string changed", "a", "b", false},
		{"string type changed", "a", true, false},
		{"empty string type changed", "", nil, false},
		{"number equal", float64(1), float64(1), true},
		{"zero equal", float64(0), float64(0), true},
		{"number changed", float64(1), float64(2), false},
		{"number type changed", float64(1), "1", false},
		{"zero type changed", float64(0), nil, false},
		{"true equal", true, true, true},
		{"false equal", false, false, true},
		{"boolean changed", true, false, false},
		{"boolean type changed", true, "true", false},
		{"false type changed", false, nil, false},
		{"array equal", []any{"a", "b"}, []any{"a", "b"}, true},
		{"empty array equal", []any{}, []any{}, true},
		{"array first changed", []any{"a", "b"}, []any{"c", "b"}, false},
		{"array last changed", []any{"a", "b"}, []any{"a", "c"}, false},
		{"array order changed", []any{"a", "b"}, []any{"b", "a"}, false},
		{"array grew", []any{"a"}, []any{"a", "b"}, false},
		{"array shrank", []any{"a", "b"}, []any{"a"}, false},
		{"array type changed", []any{"a"}, "a", false},
		{"empty array type changed", []any{}, nil, false},
		{"object equal", map[string]any{"x": nil}, map[string]any{"x": nil}, true},
		{"empty object equal", map[string]any{}, map[string]any{}, true},
		{"object value changed", map[string]any{"x": "old"}, map[string]any{"x": "new"}, false},
		{"object key changed", map[string]any{"x": "value"}, map[string]any{"y": "value"}, false},
		{"null object key changed", map[string]any{"x": nil}, map[string]any{"y": nil}, false},
		{"object grew", map[string]any{"x": nil}, map[string]any{"x": nil, "y": nil}, false},
		{"object shrank", map[string]any{"x": nil, "y": nil}, map[string]any{"x": nil}, false},
		{"object type changed", map[string]any{"x": nil}, nil, false},
		{"empty object type changed", map[string]any{}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrays are atomic: a false inequality for an equal object cannot
			// disappear into a recursive walk that happens to emit no changes.
			before, after := []any{tc.before}, []any{tc.after}
			want := []port.Change{}
			if !tc.same {
				want = append(want, port.Change{Path: "", Before: before, After: after})
			}
			if got := Differences(before, after); !reflect.DeepEqual(got, want) {
				t.Fatalf("Differences = %#v, want %#v", got, want)
			}
		})
	}
}

func TestDifferencesEachApprovedSetting(t *testing.T) {
	const settings = `{
		"schema":1,"profile":"allowlist-v1",
		"host":{"uid":"1000","gid":"1001"},
		"checkout":{"root":"/checkout","device":"1","inode":"2","git_device":"3","git_inode":"4"},
		"engine":{"endpoint":"unix:///engine.sock","id":"daemon","os":"linux","architecture":"amd64"},
		"policy":{"version":2,"manifest":".devcontainer/devcontainer.json","network":{"mode":"allowlist","allow":["a.example","b.example"]}},
		"proxy":{"image":"proxy-image"},
		"manifest":{"name":null,"image":"worker-image","workspaceFolder":"/workspace","containerEnv":{"APP":"old"},"containerUser":"1000","remoteUser":"1000"},
		"image":{"reference":"worker-image","id":"config-id","os":"linux","architecture":"amd64"}
	}`
	decode := func(t *testing.T) map[string]any {
		t.Helper()
		var value map[string]any
		if err := json.Unmarshal([]byte(settings), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	if got := Differences(decode(t), decode(t)); !reflect.DeepEqual(got, []port.Change{}) {
		t.Fatalf("identical settings have differences: %#v", got)
	}
	for _, tc := range []struct {
		path          string
		before, after any
	}{
		{"/schema", float64(1), float64(2)},
		{"/profile", "allowlist-v1", "offline-v1"},
		{"/host/uid", "1000", "2000"},
		{"/host/gid", "1001", "2001"},
		{"/checkout/root", "/checkout", "/other"},
		{"/checkout/device", "1", "5"},
		{"/checkout/inode", "2", "6"},
		{"/checkout/git_device", "3", "7"},
		{"/checkout/git_inode", "4", "8"},
		{"/engine/endpoint", "unix:///engine.sock", "unix:///other.sock"},
		{"/engine/id", "daemon", "other-daemon"},
		{"/engine/os", "linux", "other-os"},
		{"/engine/architecture", "amd64", "arm64"},
		{"/policy/version", float64(2), float64(1)},
		{"/policy/manifest", ".devcontainer/devcontainer.json", "other.json"},
		{"/policy/network/mode", "allowlist", "offline"},
		{"/policy/network/allow", []any{"a.example", "b.example"}, []any{"a.example", "c.example"}},
		{"/proxy/image", "proxy-image", "other-proxy-image"},
		{"/manifest/name", nil, "named"},
		{"/manifest/image", "worker-image", "other-worker-image"},
		{"/manifest/workspaceFolder", "/workspace", "/workspace/nested"},
		{"/manifest/containerEnv", map[string]any{"APP": "old"}, map[string]any{"APP": "new"}},
		{"/manifest/containerUser", "1000", "2000"},
		{"/manifest/remoteUser", "1000", "2000"},
		{"/image/reference", "worker-image", "other-worker-image"},
		{"/image/id", "config-id", "other-config-id"},
		{"/image/os", "linux", "other-os"},
		{"/image/architecture", "amd64", "arm64"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			before, after := decode(t), decode(t)
			parts := strings.Split(strings.TrimPrefix(tc.path, "/"), "/")
			parent := after
			for _, key := range parts[:len(parts)-1] {
				parent = parent[key].(map[string]any)
			}
			parent[parts[len(parts)-1]] = tc.after
			want := []port.Change{{Path: tc.path, Before: tc.before, After: tc.after}}
			if got := Differences(before, after); !reflect.DeepEqual(got, want) {
				t.Fatalf("changed setting = %#v, want %#v", got, want)
			}
			want = []port.Change{{Path: tc.path, Before: tc.after, After: tc.before}}
			if got := Differences(after, before); !reflect.DeepEqual(got, want) {
				t.Fatalf("reverted setting = %#v, want %#v", got, want)
			}
		})
	}
}

func TestDifferencesObjectBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name          string
		before, after any
		want          []port.Change
	}{
		{"empty objects", map[string]any{}, map[string]any{}, []port.Change{}},
		{"added false", map[string]any{}, map[string]any{"flag": false}, []port.Change{{Path: "/flag", Before: nil, After: false}}},
		{"removed false", map[string]any{"flag": false}, map[string]any{}, []port.Change{{Path: "/flag", Before: false, After: nil}}},
		{"added object", nil, map[string]any{"image": "proxy"}, []port.Change{{Path: "", Before: nil, After: map[string]any{"image": "proxy"}}}},
		{"removed object", map[string]any{"image": "proxy"}, nil, []port.Change{{Path: "", Before: map[string]any{"image": "proxy"}, After: nil}}},
		{"scalar to object", "old", map[string]any{"image": "proxy"}, []port.Change{{Path: "", Before: "old", After: map[string]any{"image": "proxy"}}}},
		{"object to scalar", map[string]any{"image": "proxy"}, "new", []port.Change{{Path: "", Before: map[string]any{"image": "proxy"}, After: "new"}}},
		{
			"nested union and escaped path",
			map[string]any{"nested": map[string]any{"removed": "old", "same": true}},
			map[string]any{"nested": map[string]any{"added~/": "new", "same": true}},
			[]port.Change{{Path: "/nested/added~0~1", Before: nil, After: "new"}, {Path: "/nested/removed", Before: "old", After: nil}},
		},
		{
			"only the exact environment path is atomic",
			map[string]any{"containerEnv": map[string]any{"APP": "old"}},
			map[string]any{"containerEnv": map[string]any{"APP": "new"}},
			[]port.Change{{Path: "/containerEnv/APP", Before: "old", After: "new"}},
		},
		{
			"equal environment is omitted beside a changed field",
			map[string]any{"manifest": map[string]any{"name": "old", "containerEnv": map[string]any{"APP": "same"}}},
			map[string]any{"manifest": map[string]any{"name": "new", "containerEnv": map[string]any{"APP": "same"}}},
			[]port.Change{{Path: "/manifest/name", Before: "old", After: "new"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Differences(tc.before, tc.after); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Differences = %#v, want %#v", got, tc.want)
			}
		})
	}
}
