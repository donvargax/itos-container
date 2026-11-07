package domain_test

import (
	"context"
	"testing"

	session "github.com/donvargax/itos-container/internal/engine/session/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	fake "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/donvargax/itos-container/internal/slices/status/domain"
)

func TestStatusRejectsArgumentsBeforeOpeningRecovery(t *testing.T) {
	r := domain.Inspect(context.Background(), nil, []string{"untrusted"})
	if len(r.Failures) != 1 || r.Failures[0].Outcome != port.Usage {
		t.Fatal(r)
	}
	r = domain.Inspect(context.Background(), session.Recovery{Registry: &fake.Registry{}}, nil)
	if len(r.Entries) != 0 || len(r.Failures) != 0 {
		t.Fatal(r)
	}
}
