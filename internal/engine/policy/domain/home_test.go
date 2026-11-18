package domain_test

import (
	"github.com/donvargax/itos-container/internal/engine/policy/domain"
	"github.com/donvargax/itos-container/internal/engine/policy/port"
	"github.com/donvargax/itos-container/internal/engine/policy/port/porttest"
	"reflect"
	"strconv"
	"testing"
)

func TestHomePolicyClosedShape(t *testing.T) {
	for _, version := range []int{1, 2, 3} {
		for _, tc := range []struct {
			value port.Value
			kind  port.Failure
			path  []string
		}{
			{object(map[string]port.Value{"storage": text("volume")}), port.None, nil},
			{port.Value{}, port.Type, []string{"home"}},
			{object(nil), port.Required, []string{"home", "storage"}},
			{object(map[string]port.Value{"storage": {}}), port.Type, []string{"home", "storage"}},
			{object(map[string]port.Value{"storage": text("tmpfs")}), port.HomeStorage, []string{"home", "storage"}},
			{object(map[string]port.Value{"storage": text("volume"), "source": text("/host")}), port.UnknownKey, []string{"home", "source"}},
		} {
			root := minimal()
			root.Fields["version"] = number(strconv.Itoa(version), "0")
			root.Fields["home"] = tc.value
			result := domain.Inspect(&porttest.Source{Value: root}, nil)
			if tc.kind == port.None {
				if result.Policy == nil || result.Policy.Home == nil || result.Policy.Home.Storage != "volume" || len(result.Issues) != 0 {
					t.Fatal(result)
				}
				continue
			}
			if result.Policy != nil || !reflect.DeepEqual(result.Issues, []port.Issue{{Kind: tc.kind, Path: tc.path, Version: version}}) {
				t.Fatal(result)
			}
			d := port.Diagnostic(result.Issues[0])
			if d.Exit != 2 || d.Message == "" || d.Fix == "" {
				t.Fatal(d)
			}
		}
	}
}
