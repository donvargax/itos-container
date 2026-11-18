package porttest

import (
	"context"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// HomeConsent is an in-memory answer at the human-review boundary.
type HomeConsent struct {
	Yes    bool
	Before func()
}

func (c HomeConsent) Review(ctx context.Context, _ port.HomeEntry, _ string) bool {
	if ctx.Err() != nil {
		return false
	}
	if c.Before != nil {
		c.Before()
	}
	return c.Yes && ctx.Err() == nil
}

// HomeConsentContract checks the answers shared by the fake and terminal adapter.
// Terminal ownership, prompt content and redirection are adapter checks.
func HomeConsentContract(t Reporter, factory func(bool) port.HomeConsent) {
	t.Helper()
	for _, action := range []string{"reset", "delete"} {
		for _, answer := range []bool{false, true} {
			consent := factory(answer)
			if got := consent.Review(context.Background(), port.HomeEntry{ID: "reviewed-home"}, action); got != answer {
				t.Errorf("%s answer %v became %v", action, answer, got)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if factory(answer).Review(ctx, port.HomeEntry{ID: "reviewed-home"}, action) {
				t.Errorf("%s accepted canceled consent", action)
			}
		}
	}
}
