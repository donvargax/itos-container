package porttest

import (
	"context"
	"time"

	budget "github.com/donvargax/itos-container/internal/engine/budget/port"
	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

func (e *Engine) OpenContext(ctx context.Context, selection, root string) (port.Session, port.Failure) {
	if ctx.Err() != nil {
		return nil, port.EngineTimeout
	}
	d, _ := ctx.Deadline()
	s, f := e.Open(selection, root, time.Until(d))
	if s == nil {
		return nil, f
	}
	return contextualSession{s, budget.Lifetime(ctx)}, f
}

type contextualSession struct {
	port.Session
	ctx context.Context
}

func (s contextualSession) ForContext(ctx context.Context) port.Session {
	return contextualSession{s.Session, ctx}
}
func (s contextualSession) Version() (port.Version, port.Failure) {
	if s.ctx.Err() != nil {
		return port.Version{}, port.EngineTimeout
	}
	return s.Session.Version()
}
func (s contextualSession) Info() (port.Info, port.Failure) {
	if s.ctx.Err() != nil {
		return port.Info{}, port.EngineTimeout
	}
	return s.Session.Info()
}
func (s contextualSession) Image(ref string) (port.Image, port.Failure) {
	if s.ctx.Err() != nil {
		return port.Image{}, port.EngineTimeout
	}
	return s.Session.Image(ref)
}

// ContextEngineContract verifies establishment does not expire a retained
// session, and each request honors its own cancellation on fake and SDK alike.
func ContextEngineContract(t Reporter, create func(*Session) (port.ContextEngine, string, string)) {
	t.Helper()
	e, endpoint, root := create(HealthySession())
	phase, end := context.WithTimeout(context.Background(), time.Second)
	defer end()
	call, cancel := context.WithTimeout(phase, 500*time.Millisecond)
	s, f := e.OpenContext(budget.WithLifetime(call, phase), endpoint, root)
	cancel()
	if f != port.None || s == nil {
		t.Errorf("context open: %v", f)
		return
	}
	defer s.Close()
	contextual, ok := s.(port.ContextSession)
	if !ok {
		t.Errorf("session lacks finite request contexts")
		return
	}
	for _, cancelled := range []bool{false, true} {
		request, stop := context.WithCancel(phase)
		if cancelled {
			stop()
		}
		view := contextual.ForContext(request)
		_, vf := view.Version()
		_, inf := view.Info()
		_, imf := view.Image(Reference)
		stop()
		if (vf != port.None) != cancelled || (inf != port.None) != cancelled || (imf != port.None) != cancelled {
			t.Errorf("request cancellation=%v: %v %v %v", cancelled, vf, inf, imf)
		}
	}
	ended, stop := context.WithCancel(context.Background())
	stop()
	if s, f := e.OpenContext(ended, endpoint, root); f == port.None || s != nil {
		t.Errorf("expired establishment succeeded")
	}
}
