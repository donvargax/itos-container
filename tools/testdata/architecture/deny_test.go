// Adapted from donvargax/itos-cc, bcb081778820602cbc968f8759c7b5a2d0effdc6.
// This copied test retains the source's AGPL-3.0 license; see licenses/itos-cc.txt.
package architecture_test

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/tools/architecture"
)

// nestedDenies refuses a depguard deny list holding an entry that prefixes
// another (T-167). depguard sorts a list and looks an import up as the one
// entry sorting just before it, by prefix, an entry ending in $ matching
// only itself (T-159): a deny prefixing a longer one is hidden from every
// import sorting after the longer, so those imports are let by. An entry
// ending in $ prefixes nothing, and the longer's trailing $ is left off, as
// depguard leaves it off when it sorts.
func nestedDenies(rule string, denies []string) []string {
	sorted := slices.Sorted(slices.Values(denies))
	var refusals []string
	for _, shorter := range sorted {
		if strings.HasSuffix(shorter, "$") {
			continue
		}
		for _, longer := range sorted {
			trimmed := strings.TrimRight(longer, "$")
			if trimmed != shorter && strings.HasPrefix(trimmed, shorter) {
				refusals = append(refusals, fmt.Sprintf("depguard rule %q denies %q, which prefixes its %q: depguard looks a deny up as the one entry sorting just before the import (T-159), so imports sorting after %q are let by; give the longer a rule of its own, as slice-adapters is", rule, shorter, longer, longer))
			}
		}
	}
	return refusals
}

// TestNoDenyEntryPrefixesAnother refuses, with no linter or environment
// variable, a depguard rule of the repository's .golangci.yml whose deny
// list nests (T-167), its table proving the refusal first.
func TestNoDenyEntryPrefixesAnother(t *testing.T) {
	module := "github.com/donvargax/itos-container"
	for _, c := range []struct {
		name     string
		denies   []string
		refusals []string
	}{
		{"nested", []string{module, module + "/internal/slices/mutation/adapters"}, []string{fmt.Sprintf("%q, which prefixes its %q", module, module+"/internal/slices/mutation/adapters")}},
		{"nested exact longer", []string{module + "/internal/slices/", module + "/internal/slices/units$"}, []string{fmt.Sprintf("%q, which prefixes its %q", module+"/internal/slices/", module+"/internal/slices/units$")}},
		{"exact shorter", []string{module + "/project$", module + "/project/sub"}, nil},
		{"slice-fakes", []string{module + "/internal/selection/port/porttest", module + "/internal/lang/port/porttest", module + "/internal/snapshot/port/porttest", module + "/internal/coverage/port/porttest", module + "/internal/slices/mutation/domain/porttest", module + "/internal/coverage/porttest"}, nil},
		{"mocks", []string{"github.com/golang/mock", "go.uber.org/mock", "github.com/stretchr/testify/mock", "github.com/vektra/mockery", "github.com/maxbrunsfeld/counterfeiter"}, nil},
		{"kong", []string{"github.com/alecthomas/kong", "github.com/donvargax/kong"}, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			refusals := nestedDenies(c.name, c.denies)
			if len(refusals) != len(c.refusals) {
				t.Fatalf("refusals: got %q, want ones saying %q", refusals, c.refusals)
			}
			for i, text := range c.refusals {
				for _, said := range []string{fmt.Sprintf("depguard rule %q", c.name), text, "T-159"} {
					if !strings.Contains(refusals[i], said) {
						t.Errorf("the refusal does not say %q: %s", said, refusals[i])
					}
				}
			}
		})
	}
	rules, err := architecture.Rules(filepath.Join(root(t), ".golangci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	denied := 0
	for name, rule := range rules {
		var denies []string
		for _, deny := range rule.Deny {
			denies = append(denies, deny.Pkg)
		}
		denied += len(denies)
		for _, refusal := range nestedDenies(name, denies) {
			t.Error(refusal)
		}
	}
	if denied == 0 {
		t.Fatal(".golangci.yml's depguard rules deny nothing where this test reads them, each rule's deny[].pkg")
	}
}
