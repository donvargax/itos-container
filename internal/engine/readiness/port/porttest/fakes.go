// Package porttest supplies memory-only readiness fakes and shared contracts.
package porttest

import (
	"slices"
	"time"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// Checkout represents a pinned identity and observed replacement in memory.
type Checkout struct {
	Identity        port.CheckoutIdentity
	Failure         port.Failure
	InspectFailure  port.Failure
	Changed, Opened bool
	Roots           []string
	LinkCounts      map[string]uint64
	Unreadable      bool
}

func (c *Checkout) Pin(root string) port.Failure {
	c.Roots = append(c.Roots, root)
	c.Opened = c.Failure == port.None
	return c.Failure
}
func (c *Checkout) Inspect() (port.CheckoutIdentity, port.Failure) {
	if !c.Opened {
		return port.CheckoutIdentity{}, port.CheckoutUnavailable
	}
	if c.InspectFailure != port.None {
		return port.CheckoutIdentity{}, c.InspectFailure
	}
	if failure := c.independence(); failure != port.None {
		return port.CheckoutIdentity{}, failure
	}
	return c.Identity, port.None
}
func (c *Checkout) independence() port.Failure {
	if c.Unreadable {
		return port.CheckoutUnavailable
	}
	for _, links := range c.LinkCounts {
		if links != 1 {
			return port.CheckoutHardlink
		}
	}
	return port.None
}
func (c *Checkout) Unchanged() bool { return c.Opened && !c.Changed && c.independence() == port.None }
func (c *Checkout) Close()          { c.Opened = false }

// Engine records selection and budget without dialing or measuring real time.
type Engine struct {
	Session         *Session
	Failure         port.Failure
	Selection, Root string
	Budget          time.Duration
	Opens           int
}

func (e *Engine) Open(selection, root string, budget time.Duration) (port.Session, port.Failure) {
	e.Selection, e.Root, e.Budget = selection, root, budget
	e.Opens++
	if e.Failure != port.None {
		return nil, e.Failure
	}
	return e.Session, port.None
}

// Session holds observed data independently from the domain's decisions. The
// shared contract checks the same mapping and error outcomes on the SDK adapter.
type Session struct {
	Address                                   string
	VersionData                               port.Version
	InfoData                                  port.Info
	Images                                    map[string]port.Image
	VersionFailure, InfoFailure, ImageFailure port.Failure
	Changed, Closed                           bool
	LaterInfo                                 *port.Info
	LaterInfoFailure                          port.Failure
	InfoReads                                 int
	ImageReads                                []string
}

func (s *Session) Endpoint() string { return s.Address }
func (s *Session) Version() (port.Version, port.Failure) {
	if s.VersionFailure != port.None {
		return port.Version{}, s.VersionFailure
	}
	return s.VersionData, port.None
}
func (s *Session) Info() (port.Info, port.Failure) {
	s.InfoReads++
	if s.InfoReads > 1 && s.LaterInfoFailure != port.None {
		return port.Info{}, s.LaterInfoFailure
	}
	if s.InfoFailure != port.None {
		return port.Info{}, s.InfoFailure
	}
	if s.InfoReads > 1 && s.LaterInfo != nil {
		return *s.LaterInfo, port.None
	}
	return s.InfoData, port.None
}
func (s *Session) Image(reference string) (port.Image, port.Failure) {
	s.ImageReads = append(s.ImageReads, reference)
	if s.ImageFailure != port.None {
		return port.Image{}, s.ImageFailure
	}
	image, exists := s.Images[reference]
	if !exists {
		return port.Image{}, port.ImageMissing
	}
	image.RepoDigests = slices.Clone(image.RepoDigests)
	return image, port.None
}
func (s *Session) Unchanged() bool { return !s.Changed }
func (s *Session) Close()          { s.Closed = true }
