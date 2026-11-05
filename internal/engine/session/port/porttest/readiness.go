package porttest

import (
	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"time"
)

// Readiness models a pinned native socket for SDK adapter unit tests.
type Readiness struct {
	EndpointValue string
	VersionValue  rp.Version
	InfoValue     rp.Info
	Failure       rp.Failure
	Current       bool
	Budget        time.Duration
}

func (r *Readiness) Open(_, _ string, budget time.Duration) (rp.Session, rp.Failure) {
	r.Budget = budget
	if r.Failure != rp.None {
		return nil, r.Failure
	}
	return r, rp.None
}
func (r *Readiness) Endpoint() string                  { return r.EndpointValue }
func (r *Readiness) Version() (rp.Version, rp.Failure) { return r.VersionValue, r.Failure }
func (r *Readiness) Info() (rp.Info, rp.Failure)       { return r.InfoValue, r.Failure }
func (*Readiness) Image(string) (rp.Image, rp.Failure) { return rp.Image{}, rp.ImageMissing }
func (r *Readiness) Unchanged() bool                   { return r.Current }
func (*Readiness) Close()                              {}

// Checkout pins only supplied identity values, with no filesystem access.
type Checkout struct {
	Identity rp.CheckoutIdentity
	Failure  rp.Failure
	Changed  bool
}

func (c *Checkout) Pin(string) rp.Failure                      { return c.Failure }
func (c *Checkout) Inspect() (rp.CheckoutIdentity, rp.Failure) { return c.Identity, c.Failure }
func (c *Checkout) Unchanged() bool                            { return !c.Changed }
func (*Checkout) Close()                                       {}
