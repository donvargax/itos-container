package domain_test

import (
	"context"
	"strings"
	"testing"

	session "github.com/donvargax/itos-container/internal/engine/session/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	fake "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/donvargax/itos-container/internal/slices/stop/domain"
)

func TestStopValidatesCompleteIDBeforeOpeningRecovery(t *testing.T) {
	id := strings.Repeat("a", 64)
	for _, args := range [][]string{nil, {""}, {"../outside"}, {id[:63]}, {strings.ToUpper(id)}, {id, "extra"}} {
		r := domain.Stop(context.Background(), nil, args)
		if len(r.Failures) != 1 || r.Failures[0].Outcome != port.Usage {
			t.Fatal(args, r)
		}
	}
	r := domain.Stop(context.Background(), session.Recovery{Registry: &fake.Registry{}}, []string{id})
	if len(r.Failures) != 1 || r.Failures[0].Outcome != port.Refused || r.Failures[0].Session != id {
		t.Fatal(r)
	}
}
