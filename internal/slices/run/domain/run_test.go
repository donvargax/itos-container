package domain_test

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/donvargax/itos-container/internal/slices/run/domain"
)

func TestRefusalsBeforeStartup(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request domain.Invocation
		want    error
	}{
		{"absent separator", domain.Invocation{}, domain.ErrCommandRequired},
		{"positional command", domain.Invocation{Argv: []string{"git", "status"}}, domain.ErrCommandRequired},
		{"separator alone", domain.Invocation{Separated: true}, domain.ErrCommandRequired},
		{"empty executable", domain.Invocation{Separated: true, Argv: []string{""}}, domain.ErrCommandRequired},
		{"later word cannot supply executable", domain.Invocation{Separated: true, Argv: []string{"", "git"}}, domain.ErrCommandRequired},
		{"JSON wins without separator", domain.Invocation{JSON: true}, domain.ErrJSONUnsupported},
		{"JSON wins without executable", domain.Invocation{JSON: true, Separated: true}, domain.ErrJSONUnsupported},
		{"JSON wins with empty executable", domain.Invocation{JSON: true, Separated: true, Argv: []string{"", "--no-json"}}, domain.ErrJSONUnsupported},
		{"JSON wins with command", domain.Invocation{JSON: true, Separated: true, Argv: []string{"git"}}, domain.ErrJSONUnsupported},
		{"valid invocation still refuses", domain.Invocation{Separated: true, Argv: []string{"git", "status"}}, domain.ErrStartupUnavailable},
		{"flag executable", domain.Invocation{Separated: true, Argv: []string{"--json"}}, domain.ErrStartupUnavailable},
		{"separator executable", domain.Invocation{Separated: true, Argv: []string{"--"}}, domain.ErrStartupUnavailable},
		{"literal workload arguments", domain.Invocation{Separated: true, Argv: []string{"git", "", "two words", "--", "--help", "--no-json"}}, domain.ErrStartupUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := slices.Clone(tc.request.Argv)
			if err := domain.Refuse(tc.request); !errors.Is(err, tc.want) {
				t.Fatalf("refusal = %v, want %v", err, tc.want)
			}
			if !reflect.DeepEqual(tc.request.Argv, before) {
				t.Fatal("validation changed the workload arguments")
			}
		})
	}
}
