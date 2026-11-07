package porttest

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// Account is an environment-independent account observation, without I/O.
type Account struct {
	Home    string
	Failure port.Failure
}

func (a *Account) AccountHome() (string, port.Failure) { return a.Home, a.Failure }

// Reservations retains independent per-location authority observations.
type Reservations struct {
	RootsByLocation map[string][]string
	Failure         port.Failure
}

func (r *Reservations) Roots(_ context.Context, _ port.Candidate, path string) ([]string, port.Failure) {
	return slices.Clone(r.RootsByLocation[path]), r.Failure
}

// ControlState models persistent bindings and one cancellable account exclusion.
// Closing a transaction releases the lock, never the retained protection.
type ControlState struct {
	lock       chan struct{}
	mu         sync.Mutex
	Bound      map[string]bool
	Bootstrap  bool
	WriteFault bool
	Replaced   bool
}

func NewControlState() *ControlState {
	return &ControlState{lock: make(chan struct{}, 1), Bound: map[string]bool{}}
}
func (s *ControlState) Snapshot() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprint(s.Bootstrap, s.Replaced, slices.Sorted(maps.Keys(s.Bound)))
}

type Control struct {
	State           *ControlState
	Account         *Account
	Reservations    *Reservations
	Fixed, Selected string
}

func (s Control) Begin(ctx context.Context, c port.Candidate, write bool) (port.ControlTransaction, port.Failure) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if s.Account.Failure != "" || s.Account.Home == "" {
		return nil, "state.unavailable"
	}
	select {
	case s.State.lock <- struct{}{}:
	case <-ctx.Done():
		return nil, "state.busy"
	}
	success := false
	defer func() {
		if !success {
			<-s.State.lock
		}
	}()
	if ctx.Err() != nil {
		return nil, "state.busy"
	}
	s.State.mu.Lock()
	defer s.State.mu.Unlock()
	if s.State.Replaced {
		return nil, "state.path"
	}
	exposed := func(location, root string) bool {
		return location == root || strings.HasPrefix(location, strings.TrimRight(root, "/")+"/")
	}
	if exposed(s.Fixed, c.Root) || exposed(s.Selected, c.Root) {
		return nil, "state.path"
	}
	if s.Reservations.Failure != "" {
		return nil, s.Reservations.Failure
	}
	for _, roots := range s.Reservations.RootsByLocation {
		for _, root := range roots {
			if exposed(s.Fixed, root) || exposed(s.Selected, root) {
				return nil, "state.exposed"
			}
		}
	}
	if write {
		s.State.Bootstrap = true
		if s.State.WriteFault {
			return nil, "state.write"
		}
		s.State.Bound[s.Fixed], s.State.Bound[s.Selected] = true, true
	}
	success = true
	return &controlLease{state: s.State}, ""
}

type controlLease struct{ state *ControlState }

func (l *controlLease) Close() { <-l.state.lock }
func (l *controlLease) Unchanged() bool {
	l.state.mu.Lock()
	defer l.state.mu.Unlock()
	return !l.state.Replaced
}
