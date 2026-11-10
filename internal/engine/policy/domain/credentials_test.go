package domain_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/policy/domain"
	"github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
)

func credentialPolicy(value port.Value) port.Result {
	root := minimal()
	root.Fields["version"] = number("3", "0")
	root.Fields["credentials"] = value
	return domain.Inspect(&porttest.Source{Value: root}, nil)
}

func TestCredentialPolicyClosedShape(t *testing.T) {
	entry := func(kind, source port.Value) port.Value {
		return object(map[string]port.Value{"kind": kind, "source": source})
	}
	for _, tc := range []struct {
		value port.Value
		kind  port.Failure
		path  []string
	}{
		{port.Value{Kind: port.Null}, port.Type, []string{"credentials"}},
		{port.Value{Kind: port.Array}, port.CredentialsEmpty, []string{"credentials"}},
		{port.Value{Kind: port.Array, Items: []port.Value{text("bad")}}, port.Type, []string{"credentials", "0"}},
		{port.Value{Kind: port.Array, Items: []port.Value{object(map[string]port.Value{"source": text("/vault/a")})}}, port.Required, []string{"credentials", "0", "kind"}},
		{port.Value{Kind: port.Array, Items: []port.Value{object(map[string]port.Value{"kind": text("claude-token")})}}, port.Required, []string{"credentials", "0", "source"}},
		{port.Value{Kind: port.Array, Items: []port.Value{entry(port.Value{}, text("/vault/a"))}}, port.Type, []string{"credentials", "0", "kind"}},
		{port.Value{Kind: port.Array, Items: []port.Value{entry(text("claude-token"), port.Value{})}}, port.Type, []string{"credentials", "0", "source"}},
		{port.Value{Kind: port.Array, Items: []port.Value{entry(text("codex-auth"), text("/vault/a"))}}, port.CredentialKind, []string{"credentials", "0", "kind"}},
		{port.Value{Kind: port.Array, Items: []port.Value{entry(text("claude-token"), text("relative"))}}, port.CredentialSource, []string{"credentials", "0", "source"}},
	} {
		got := credentialPolicy(tc.value)
		if got.Policy != nil || !reflect.DeepEqual(got.Issues, []port.Issue{{Kind: tc.kind, Path: tc.path, Version: 3}}) {
			t.Fatalf("%#v: %#v", tc, got)
		}
	}
	for _, kind := range []string{"claude-token", "github-token"} {
		value := port.Value{Kind: port.Array, Items: []port.Value{entry(text(kind), text("/vault/a")), entry(text(kind), text("/vault/b"))}}
		if got := credentialPolicy(value); !reflect.DeepEqual(got.Issues, []port.Issue{{Kind: port.CredentialDuplicate, Path: []string{"credentials", "1", "kind"}, Version: 3}}) {
			t.Fatal(got)
		}
	}
	value := port.Value{Kind: port.Array, Items: []port.Value{entry(text("unknown"), text("/vault/a")), entry(text("unknown"), text("/vault/b"))}}
	if got := credentialPolicy(value); len(got.Issues) != 2 || got.Issues[1].Kind != port.CredentialKind {
		t.Fatal(got)
	}
}

func TestCredentialLiteralPathBoundaries(t *testing.T) {
	valid := []string{"/a", "/C:/file", "/ñ/~/$HOME/$(touch forbidden)", "/" + strings.Repeat("ñ", 2047) + "a"}
	invalid := []string{"", "relative", "/", "/a/", "//a", "/a//b", "/a/./b", "/a/../b", "/a\\b", "/" + strings.Repeat("ñ", 2048)}
	for _, c := range []rune{0, 1, 9, 10, 31, 127} {
		invalid = append(invalid, "/a"+string(c))
	}
	for _, group := range []struct {
		paths []string
		valid bool
	}{{valid, true}, {invalid, false}} {
		for _, source := range group.paths {
			got := credentialPolicy(port.Value{Kind: port.Array, Items: []port.Value{object(map[string]port.Value{"kind": text("claude-token"), "source": text(source)})}})
			if group.valid {
				if got.Policy == nil || !reflect.DeepEqual(got.Policy.Credentials, []port.Credential{{Kind: "claude-token", Source: source}}) {
					t.Fatalf("literal path lost: %#v", got)
				}
			} else if got.Policy != nil || len(got.Issues) != 1 || got.Issues[0].Kind != port.CredentialSource {
				t.Fatal(got)
			}
		}
	}
}

func TestCredentialVersionAndIndependentProblems(t *testing.T) {
	root := minimal()
	root.Fields["version"] = number("3", "0")
	root.Fields["network"] = object(map[string]port.Value{"mode": text("wrong")})
	root.Fields["credentials"] = port.Value{Kind: port.Array, Items: []port.Value{object(map[string]port.Value{"kind": text("unknown"), "source": text("relative"), "token": text("rejected")})}}
	got := domain.Inspect(&porttest.Source{Value: root}, nil)
	if got.Policy != nil || len(got.Issues) != 4 {
		t.Fatal(got)
	}
	for _, issue := range got.Issues {
		if issue.Version != 3 {
			t.Fatal(issue)
		}
	}
	delete(root.Fields, "network")
	for _, version := range []string{"1", "2"} {
		root.Fields["version"] = number(version, "0")
		got = domain.Inspect(&porttest.Source{Value: root}, nil)
		if len(got.Issues) != 1 || got.Issues[0].Kind != port.UnknownKey {
			t.Fatal(got)
		}
	}
}
