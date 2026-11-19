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
	// 4095 bytes is the longest path Linux names: PATH_MAX counts the
	// terminator. The limit counts UTF-8 bytes, never characters.
	valid := []string{"/a", "/C:/file", "/ñ/~/$HOME/$(touch forbidden)", "/" + strings.Repeat("ñ", 2047), "/" + strings.Repeat("a", 4094)}
	invalid := []string{"", "relative", "/", "/a/", "//a", "/a//b", "/a/./b", "/a/../b", "/a\\b", "/" + strings.Repeat("ñ", 2047) + "a", "/" + strings.Repeat("a", 4095)}
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

func openCodePolicy(version string, items ...port.Value) port.Result {
	root := minimal()
	root.Fields["version"] = number(version, "0")
	root.Fields["credentials"] = port.Value{Kind: port.Array, Items: items}
	return domain.Inspect(&porttest.Source{Value: root}, nil)
}

func openCodeEntry(provider port.Value) port.Value {
	fields := map[string]port.Value{"kind": text("opencode-auth"), "source": text("/h/.local/share/opencode/auth.json")}
	if provider.Kind != port.Null {
		fields["provider"] = provider
	}
	return object(fields)
}

// Version 4 adds one kind. It is the only kind with a provider, the provider is
// required and literal, and no earlier version knows the kind.
func TestOpenCodeAuthGrant(t *testing.T) {
	got := openCodePolicy("4", openCodeEntry(text("opencode-go")))
	want := []port.Credential{{Kind: "opencode-auth", Source: "/h/.local/share/opencode/auth.json", Provider: "opencode-go"}}
	if got.Policy == nil || got.Policy.Version != 4 || !reflect.DeepEqual(got.Policy.Credentials, want) {
		t.Fatalf("%#v", got)
	}
	mixed := openCodePolicy("4",
		object(map[string]port.Value{"kind": text("github-token"), "source": text("/vault/g")}),
		openCodeEntry(text("opencode-go")))
	if mixed.Policy == nil || len(mixed.Policy.Credentials) != 2 || mixed.Policy.Credentials[0].Provider != "" {
		t.Fatalf("%#v", mixed)
	}
	// Version 3's kinds and closed fields are unchanged: it knows neither the kind nor its provider.
	want3 := []port.Issue{
		{Kind: port.UnknownKey, Path: []string{"credentials", "0", "provider"}, Version: 3},
		{Kind: port.CredentialKind, Path: []string{"credentials", "0", "kind"}, Version: 3},
	}
	if got := openCodePolicy("3", openCodeEntry(text("opencode-go"))); got.Policy != nil || !reflect.DeepEqual(got.Issues, want3) {
		t.Fatalf("%#v", got)
	}
}

func TestOpenCodeAuthProviderIsOneLiteralID(t *testing.T) {
	at := func(kind port.Failure) []port.Issue {
		return []port.Issue{{Kind: kind, Path: []string{"credentials", "0", "provider"}, Version: 4}}
	}
	for _, id := range []string{"a", "0", "zZ9A", "opencode-go", "Open_Code.Go-2", strings.Repeat("a", 64)} {
		if got := openCodePolicy("4", openCodeEntry(text(id))); got.Policy == nil || got.Policy.Credentials[0].Provider != id {
			t.Errorf("%q refused: %#v", id, got)
		}
	}
	for _, id := range []string{"", "*", "open*", "a/b", "-a", ".a", "_a", "a b", "ñ", "a\nb", "a\x00", "a{", "a`", "a@", "a[", "a:", "a+", "a,", strings.Repeat("a", 65)} {
		if got := openCodePolicy("4", openCodeEntry(text(id))); !reflect.DeepEqual(got.Issues, at(port.CredentialProvider)) {
			t.Errorf("%q accepted or mislocated: %#v", id, got)
		}
	}
	if got := openCodePolicy("4", openCodeEntry(port.Value{Kind: port.Number, Number: port.Decimal{Coefficient: "1", Exponent: "0"}})); !reflect.DeepEqual(got.Issues, at(port.Type)) {
		t.Errorf("%#v", got)
	}
	missing := object(map[string]port.Value{"kind": text("opencode-auth"), "source": text("/a/auth.json")})
	if got := openCodePolicy("4", missing); !reflect.DeepEqual(got.Issues, at(port.Required)) {
		t.Errorf("%#v", got)
	}
}

func TestOpenCodeAuthClosedEntryAndSingleGrant(t *testing.T) {
	// Another kind cannot name a provider: only opencode-auth selects by provider.
	withProvider := object(map[string]port.Value{"kind": text("claude-token"), "source": text("/vault/a"), "provider": text("x")})
	if got := openCodePolicy("4", withProvider); !reflect.DeepEqual(got.Issues, []port.Issue{{Kind: port.UnknownKey, Path: []string{"credentials", "0", "provider"}, Version: 4}}) {
		t.Errorf("%#v", got)
	}
	extra := object(map[string]port.Value{"kind": text("opencode-auth"), "source": text("/a/auth.json"), "provider": text("x"), "account": text("y")})
	if got := openCodePolicy("4", extra); !reflect.DeepEqual(got.Issues, []port.Issue{{Kind: port.UnknownKey, Path: []string{"credentials", "0", "account"}, Version: 4}}) {
		t.Errorf("%#v", got)
	}
	// A second login for the same kind is located at the later kind, whatever provider it names.
	got := openCodePolicy("4", openCodeEntry(text("opencode-go")), openCodeEntry(text("other")))
	if !reflect.DeepEqual(got.Issues, []port.Issue{{Kind: port.CredentialDuplicate, Path: []string{"credentials", "1", "kind"}, Version: 4}}) {
		t.Errorf("%#v", got)
	}
	bad := object(map[string]port.Value{"kind": text("unknown"), "source": text("/a/auth.json"), "provider": text("x")})
	if got := openCodePolicy("4", bad); len(got.Issues) != 2 {
		t.Errorf("an unknown kind keeps its own problems and its unlisted provider: %#v", got)
	}
}
