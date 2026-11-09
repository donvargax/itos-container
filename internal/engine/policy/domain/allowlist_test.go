package domain_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/policy/domain"
	"github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
)

func array(items ...port.Value) port.Value { return port.Value{Kind: port.Array, Items: items} }

// version2 is a minimal version 2 file with the given network.
func version2(network port.Value) port.Value {
	root := object(map[string]port.Value{"version": number("2", "0"), "manifest": text("a.json")})
	root.Fields["network"] = network
	return root
}

func allowlist(entries ...string) port.Value {
	items := make([]port.Value, len(entries))
	for i, entry := range entries {
		items[i] = text(entry)
	}
	return object(map[string]port.Value{"mode": text("allowlist"), "allow": array(items...)})
}

// identity canonicalises each spelling to itself, so a case tests the domain's own checks.
func identity(spellings ...string) *porttest.Names {
	names := &porttest.Names{Table: map[string]string{}}
	for _, spelling := range spellings {
		names.Table[spelling] = spelling
	}
	return names
}

func issue(kind port.Failure, path ...string) port.Issue {
	return port.Issue{Kind: kind, Path: path, Version: 2}
}

func TestAllowlistIsCanonicalSortedAndDeduplicated(t *testing.T) {
	names := &porttest.Names{Table: map[string]string{
		"b.example": "b.example", "B.EXAMPLE.": "b.example.", "a.example": "a.example", "A-B.example": "a-b.example",
	}}
	got := domain.Inspect(&porttest.Source{Value: version2(allowlist("b.example", "a.example", "A-B.example"))}, names)
	want := &port.Policy{Version: 2, Manifest: "a.json", Network: port.Network{Mode: "allowlist", Allow: []string{"a-b.example", "a.example", "b.example"}}}
	if !reflect.DeepEqual(got.Policy, want) || len(got.Issues) != 0 {
		t.Fatalf("got %#v", got)
	}
	// The trailing dot goes after mapping, so this repeats the first entry.
	got = domain.Inspect(&porttest.Source{Value: version2(allowlist("b.example", "B.EXAMPLE."))}, names)
	if got.Policy != nil || !reflect.DeepEqual(got.Issues, []port.Issue{issue(port.HostDuplicate, "network", "allow", "1")}) {
		t.Fatalf("got %#v", got)
	}
}

func TestOfflineAtVersion2HasNoList(t *testing.T) {
	for _, network := range []*port.Value{nil, ptr(object(nil)), ptr(object(map[string]port.Value{"mode": text("offline")}))} {
		root := version2(port.Value{})
		delete(root.Fields, "network")
		if network != nil {
			root.Fields["network"] = *network
		}
		got := domain.Inspect(&porttest.Source{Value: root}, nil)
		want := &port.Policy{Version: 2, Manifest: "a.json", Network: port.Network{Mode: "offline"}}
		if !reflect.DeepEqual(got.Policy, want) || len(got.Issues) != 0 {
			t.Fatalf("got %#v", got)
		}
	}
}

func ptr[T any](v T) *T { return &v }

func TestHostNameShape(t *testing.T) {
	label63, label64 := strings.Repeat("a", 63), strings.Repeat("a", 64)
	name253 := strings.Join([]string{label63, label63, label63, strings.Repeat("a", 61)}, ".")
	valid := map[string]string{
		"a.b": "a.b", "a-b.example": "a-b.example", "123.example": "123.example", "0x1f.example": "0x1f.example",
		"a.0xg": "a.0xg", "example.0xg1": "example.0xg1", "a.b0x": "a.b0x", label63 + ".example": label63 + ".example",
		name253: name253, name253 + ".": name253, "api.example.com.": "api.example.com", "a.1x": "a.1x", "a.x1": "a.x1",
	}
	for spelling, want := range valid {
		got := domain.Inspect(&porttest.Source{Value: version2(allowlist(spelling))}, identity(spelling))
		if got.Policy == nil || !reflect.DeepEqual(got.Policy.Network.Allow, []string{want}) {
			t.Errorf("%q: got %#v, want %q", spelling, got, want)
		}
	}
	refused := []string{
		"", ".", "example", "localhost.", "com", ".example.com", "..example.com", "example.com..", "a..b", "a.b..", ".a.b",
		label64 + ".example", name253 + "a", name253 + "a.", "a." + label64,
		"127.0.0.1", "127.0.0.1.", "127.1", "0x7f.0.0.1", "1.2.3.0x4", "2130706433", "example.123", "example.0x1f", "example.0x",
		"example.0x0", "example.0xabcdef", "example.0", "a.9", "Example.com", "EXAMPLE.COM", "a.B", "bücher.example", "a_b.example",
		"a b.example", "a:b.example", "a*.example", "a/b.example", "a@b.example", "a.b\n", "a.b,c.d", "xn--.",
	}
	for _, spelling := range refused {
		got := domain.Inspect(&porttest.Source{Value: version2(allowlist(spelling))}, identity(spelling))
		if got.Policy != nil || !reflect.DeepEqual(got.Issues, []port.Issue{issue(port.HostName, "network", "allow", "0")}) {
			t.Errorf("%q: got %#v", spelling, got)
		}
	}
}

func TestEntryLengthIsCheckedBeforeCanonicalising(t *testing.T) {
	at, over := strings.Repeat("a", 1024), strings.Repeat("a", 1025)
	names := &porttest.Names{Table: map[string]string{at: "a.example", over: "a.example"}}
	got := domain.Inspect(&porttest.Source{Value: version2(allowlist(at))}, names)
	if got.Policy == nil || !reflect.DeepEqual(got.Policy.Network.Allow, []string{"a.example"}) || !reflect.DeepEqual(names.Calls, []string{at}) {
		t.Fatalf("1024 bytes: got %#v, calls %d", got, len(names.Calls))
	}
	names.Calls = nil
	got = domain.Inspect(&porttest.Source{Value: version2(allowlist(over))}, names)
	if got.Policy != nil || !reflect.DeepEqual(got.Issues, []port.Issue{issue(port.HostName, "network", "allow", "0")}) || len(names.Calls) != 0 {
		t.Fatalf("1025 bytes: got %#v, calls %d", got, len(names.Calls))
	}
}

func TestRefusedSpellingIsNeverADuplicate(t *testing.T) {
	// Neither entry has a canonical name, so neither can repeat the other.
	names := identity("a.example")
	got := domain.Inspect(&porttest.Source{Value: version2(allowlist("*.x", "a.example", "*.x"))}, names)
	want := []port.Issue{issue(port.HostName, "network", "allow", "0"), issue(port.HostName, "network", "allow", "2")}
	if got.Policy != nil || !reflect.DeepEqual(got.Issues, want) {
		t.Fatalf("got %#v", got)
	}
}

func TestEntriesKeepTheirTypes(t *testing.T) {
	network := object(map[string]port.Value{"mode": text("allowlist"), "allow": array(
		text("a.example"), port.Value{Kind: port.Null}, number("443", "0"), port.Value{Kind: port.Boolean}, array(text("a.example")), object(nil))})
	got := domain.Inspect(&porttest.Source{Value: version2(network)}, identity("a.example"))
	var want []port.Issue
	for _, i := range []string{"1", "2", "3", "4", "5"} {
		want = append(want, issue(port.Type, "network", "allow", i))
	}
	if got.Policy != nil || !reflect.DeepEqual(got.Issues, want) {
		t.Fatalf("got %#v", got)
	}
}

func TestModeAndListMustAgree(t *testing.T) {
	list := array(text("a.example"))
	for _, tc := range []struct {
		name    string
		network port.Value
		want    []port.Issue
	}{
		{"no list in allowlist mode", object(map[string]port.Value{"mode": text("allowlist")}), []port.Issue{issue(port.Required, "network", "allow")}},
		{"empty list", object(map[string]port.Value{"mode": text("allowlist"), "allow": array()}), []port.Issue{issue(port.AllowEmpty, "network", "allow")}},
		{"null list", object(map[string]port.Value{"mode": text("allowlist"), "allow": {}}), []port.Issue{issue(port.Type, "network", "allow")}},
		{"string list", object(map[string]port.Value{"mode": text("allowlist"), "allow": text("a.example")}), []port.Issue{issue(port.Type, "network", "allow")}},
		{"object list", object(map[string]port.Value{"mode": text("allowlist"), "allow": object(nil)}), []port.Issue{issue(port.Type, "network", "allow")}},
		{"list without mode", object(map[string]port.Value{"allow": list}), []port.Issue{issue(port.AllowOffline, "network", "allow")}},
		{"empty list without mode", object(map[string]port.Value{"allow": array()}), []port.Issue{issue(port.AllowOffline, "network", "allow")}},
		{"list in offline mode", object(map[string]port.Value{"mode": text("offline"), "allow": list}), []port.Issue{issue(port.AllowOffline, "network", "allow")}},
		{"null list in offline mode", object(map[string]port.Value{"mode": text("offline"), "allow": {}}), []port.Issue{issue(port.AllowOffline, "network", "allow")}},
		{"wrong mode beside a good list", object(map[string]port.Value{"mode": text("bridge"), "allow": list}), []port.Issue{issue(port.NetworkMode, "network", "mode")}},
		{"wrong mode beside a bad list", object(map[string]port.Value{"mode": text("bridge"), "allow": array(text("*"), number("7", "0"))}), []port.Issue{issue(port.NetworkMode, "network", "mode")}},
		{"wrong mode beside a wrong type", object(map[string]port.Value{"mode": text("bridge"), "allow": number("7", "0")}), []port.Issue{issue(port.NetworkMode, "network", "mode")}},
		{"capitalised mode", object(map[string]port.Value{"mode": text("Allowlist"), "allow": list}), []port.Issue{issue(port.NetworkMode, "network", "mode")}},
		{"empty mode", object(map[string]port.Value{"mode": text(""), "allow": list}), []port.Issue{issue(port.NetworkMode, "network", "mode")}},
		{"null mode beside a bad list", object(map[string]port.Value{"mode": {}, "allow": array(text("*"))}), []port.Issue{issue(port.Type, "network", "mode")}},
		{"list mode beside a list", object(map[string]port.Value{"mode": array(text("allowlist")), "allow": list}), []port.Issue{issue(port.Type, "network", "mode")}},
		{"unknown key beside a wrong mode", object(map[string]port.Value{"mode": text("bridge"), "egress": array()}), []port.Issue{issue(port.UnknownKey, "network", "egress"), issue(port.NetworkMode, "network", "mode")}},
		{"unknown key in allowlist mode", object(map[string]port.Value{"mode": text("allowlist"), "allow": list, "hosts": array()}), []port.Issue{issue(port.UnknownKey, "network", "hosts")}},
		{"network is not an object", array(text("a.example")), []port.Issue{issue(port.Type, "network")}},
		{"network is null", port.Value{}, []port.Issue{issue(port.Type, "network")}},
	} {
		got := domain.Inspect(&porttest.Source{Value: version2(tc.network)}, identity("a.example", "*"))
		sortIssues(got.Issues)
		sortIssues(tc.want)
		if got.Policy != nil || !reflect.DeepEqual(got.Issues, tc.want) {
			t.Errorf("%s: got %#v, want %#v", tc.name, got.Issues, tc.want)
		}
	}
}

func TestVersion2KeepsTheOtherChecksAndReportsAll(t *testing.T) {
	root := version2(object(map[string]port.Value{"mode": text("allowlist"), "allow": array(text("*"), text("a.example"), text("a.example")), "egress": array()}))
	root.Fields["manifest"] = port.Value{}
	root.Fields["accounts"] = array()
	got := domain.Inspect(&porttest.Source{Value: root}, identity("a.example"))
	want := []port.Issue{
		issue(port.Type, "manifest"), issue(port.HostName, "network", "allow", "0"), issue(port.HostDuplicate, "network", "allow", "2"),
		issue(port.UnknownKey, "network", "egress"), issue(port.UnknownKey, "accounts"),
	}
	sortIssues(got.Issues)
	sortIssues(want)
	if got.Policy != nil || !reflect.DeepEqual(got.Issues, want) {
		t.Fatalf("got %#v", got.Issues)
	}
}

func TestVersion1RefusesTheAllowlist(t *testing.T) {
	root := minimal()
	root.Fields["network"] = allowlist("a.example")
	names := identity("a.example")
	got := domain.Inspect(&porttest.Source{Value: root}, names)
	want := []port.Issue{
		{Kind: port.NetworkMode, Path: []string{"network", "mode"}, Version: 1},
		{Kind: port.UnknownKey, Path: []string{"network", "allow"}, Version: 1},
	}
	sortIssues(got.Issues)
	sortIssues(want)
	if got.Policy != nil || !reflect.DeepEqual(got.Issues, want) || len(names.Calls) != 0 {
		t.Fatalf("got %#v, calls %q", got.Issues, names.Calls)
	}
}

func TestRepresentationOfVersionDecidesTheSchema(t *testing.T) {
	// Numerically integral spellings arrive normalised, so 2, 2.0 and 20e-1 are one value.
	for _, number := range []port.Decimal{{Coefficient: "2", Exponent: "0"}} {
		root := version2(port.Value{})
		delete(root.Fields, "network")
		root.Fields["version"] = port.Value{Kind: port.Number, Number: number}
		if got := domain.Inspect(&porttest.Source{Value: root}, nil); got.Policy == nil || got.Policy.Version != 2 {
			t.Fatalf("got %#v", got)
		}
	}
	for _, number := range []port.Decimal{{Coefficient: "1", Exponent: "1"}, {Coefficient: "21", Exponent: "0"}, {Coefficient: "-2", Exponent: "0"}} {
		root := version2(port.Value{})
		root.Fields["version"] = port.Value{Kind: port.Number, Number: number}
		got := domain.Inspect(&porttest.Source{Value: root}, nil)
		if got.Policy != nil || !reflect.DeepEqual(got.Issues, []port.Issue{{Kind: port.Version, Path: []string{"version"}}}) {
			t.Fatalf("got %#v", got)
		}
	}
}
