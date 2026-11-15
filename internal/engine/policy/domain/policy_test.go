package domain_test

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/policy/domain"
	"github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
)

func object(fields map[string]port.Value) port.Value {
	return port.Value{Kind: port.Object, Fields: fields}
}
func text(s string) port.Value { return port.Value{Kind: port.String, Text: s} }
func number(coefficient, exponent string) port.Value {
	return port.Value{Kind: port.Number, Number: port.Decimal{Coefficient: coefficient, Exponent: exponent}}
}
func minimal() port.Value {
	return object(map[string]port.Value{"version": number("1", "0"), "manifest": text("a.json")})
}

func TestEffectivePolicy(t *testing.T) {
	for _, network := range []map[string]port.Value{nil, {}, {"mode": text("offline")}} {
		root := minimal()
		if network != nil {
			root.Fields["network"] = object(network)
		}
		s := &porttest.Source{Value: root}
		got := domain.Inspect(s, nil)
		want := &port.Policy{Version: 1, Manifest: "a.json", Network: port.Network{Mode: "offline"}}
		if !reflect.DeepEqual(got.Policy, want) || len(got.Issues) != 0 || s.Reads != 1 {
			t.Fatalf("result %#v, reads %d", got, s.Reads)
		}
	}
}

func TestRepresentationFailureStopsProjection(t *testing.T) {
	s := &porttest.Source{Value: minimal(), Issues: []port.Issue{{Kind: port.DuplicateKey, Path: []string{"version"}}}}
	got := domain.Inspect(s, nil)
	if got.Policy != nil || !reflect.DeepEqual(got.Issues, s.Issues) {
		t.Fatalf("got %#v", got)
	}
}

func TestVersionIsRequiredAndExact(t *testing.T) {
	cases := []struct {
		value   port.Value
		failure port.Failure
	}{
		{port.Value{}, port.Type}, {text("1"), port.Type}, {port.Value{Kind: port.Boolean}, port.Type},
		{object(nil), port.Type}, {port.Value{Kind: port.Array}, port.Type},
		{number("100000000000000000000000000001", "-29"), port.Type},
		{number("-1", "0"), port.Version}, {number("0", "0"), port.Version}, {number("4", "0"), port.Version},
		{number("1", "29"), port.Version},
		{number("1", "999999999999999999999"), port.Version},
		{number("1", "-999999999999999999999"), port.Type},
	}
	for _, tc := range cases {
		root := minimal()
		root.Fields["version"] = tc.value
		root.Fields["extra"] = text("ignored until version valid")
		got := domain.Inspect(&porttest.Source{Value: root}, nil)
		want := []port.Issue{{Kind: tc.failure, Path: []string{"version"}}}
		if got.Policy != nil || !reflect.DeepEqual(got.Issues, want) {
			t.Fatalf("version %#v: %#v", tc.value, got)
		}
	}
	for _, root := range []port.Value{object(nil), object(map[string]port.Value{"manifest": text("a.json")})} {
		got := domain.Inspect(&porttest.Source{Value: root}, nil)
		if !reflect.DeepEqual(got.Issues, []port.Issue{{Kind: port.Required, Path: []string{"version"}}}) {
			t.Fatalf("%#v", got)
		}
	}
	got := domain.Inspect(&porttest.Source{Value: text("not an object")}, nil)
	if !reflect.DeepEqual(got.Issues, []port.Issue{{Kind: port.Type}}) {
		t.Fatalf("%#v", got)
	}
}

func TestIndependentFieldsAndNoPartialPolicy(t *testing.T) {
	root := minimal()
	root.Fields["manifest"] = port.Value{}
	root.Fields["network"] = object(map[string]port.Value{"mode": text("host"), "egress": {Kind: port.Array}})
	root.Fields["accounts"] = port.Value{Kind: port.Array}
	got := domain.Inspect(&porttest.Source{Value: root}, nil)
	want := []port.Issue{
		{Kind: port.Type, Path: []string{"manifest"}, Version: 1}, {Kind: port.NetworkMode, Path: []string{"network", "mode"}, Version: 1},
		{Kind: port.UnknownKey, Path: []string{"network", "egress"}, Version: 1}, {Kind: port.UnknownKey, Path: []string{"accounts"}, Version: 1},
	}
	sortIssues(got.Issues)
	sortIssues(want)
	if got.Policy != nil || !reflect.DeepEqual(got.Issues, want) {
		t.Fatalf("%#v", got)
	}
}

func sortIssues(issues []port.Issue) {
	sort.Slice(issues, func(i, j int) bool { return strings.Join(issues[i].Path, "/") < strings.Join(issues[j].Path, "/") })
}

func TestFieldRequirements(t *testing.T) {
	root := minimal()
	delete(root.Fields, "manifest")
	got := domain.Inspect(&porttest.Source{Value: root}, nil)
	if !reflect.DeepEqual(got.Issues, []port.Issue{{Kind: port.Required, Path: []string{"manifest"}, Version: 1}}) {
		t.Fatalf("%#v", got)
	}
	for _, key := range []string{"manifest", "network"} {
		root = minimal()
		root.Fields[key] = port.Value{Kind: port.Array}
		got = domain.Inspect(&porttest.Source{Value: root}, nil)
		if !reflect.DeepEqual(got.Issues, []port.Issue{{Kind: port.Type, Path: []string{key}, Version: 1}}) {
			t.Fatalf("%#v", got)
		}
	}
	root = minimal()
	root.Fields["network"] = object(map[string]port.Value{"mode": {Kind: port.Number}})
	got = domain.Inspect(&porttest.Source{Value: root}, nil)
	if !reflect.DeepEqual(got.Issues, []port.Issue{{Kind: port.Type, Path: []string{"network", "mode"}, Version: 1}}) {
		t.Fatalf("%#v", got)
	}
}

func TestManifestPathBoundaries(t *testing.T) {
	valid := []string{"a", " ", "a/b", ".hidden", "...", " a ", "ab:c", "0:foo", "@:foo", "[:foo", "`:foo", "{:foo", "a\u0080", "ñ/文", "${HOME}/x", "~/x"}
	invalid := []string{"", ".", "..", "/a", "a/", "a//b", "a/./b", "a/../b", "a\\b", "a:", "z:", "A:", "Z:", "c:foo", "C:/foo"}
	for _, c := range []rune{0, 1, 9, 10, 13, 31, 127} {
		invalid = append(invalid, "a"+string(c)+"b")
	}
	for _, group := range []struct {
		paths []string
		valid bool
	}{{valid, true}, {invalid, false}} {
		for _, path := range group.paths {
			t.Run(path, func(t *testing.T) {
				root := minimal()
				root.Fields["manifest"] = text(path)
				got := domain.Inspect(&porttest.Source{Value: root}, nil)
				if group.valid {
					if got.Policy == nil || got.Policy.Manifest != path || len(got.Issues) != 0 {
						t.Fatalf("%#v", got)
					}
				} else if got.Policy != nil || !reflect.DeepEqual(got.Issues, []port.Issue{{Kind: port.Path, Path: []string{"manifest"}, Version: 1}}) {
					t.Fatalf("%#v", got)
				}
			})
		}
	}
}
