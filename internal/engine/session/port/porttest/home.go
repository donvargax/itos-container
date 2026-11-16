package porttest

import (
	"context"
	"slices"
	"strconv"
	"sync"

	budget "github.com/donvargax/itos-container/internal/engine/budget/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// Homes models protected state and durable scope exclusion entirely in memory.
type Homes struct {
	Codec         port.HomeCodec
	Scopes        map[string]port.HomeScope
	Failure       port.Outcome
	ReadFailure   port.Outcome
	ClaimFailure  port.Outcome
	MissingOpen   bool
	ClaimsFailure port.Outcome
	Before        func(string)
	BeforeSave    func(port.HomeScope) port.Outcome
	mu            sync.Mutex
	locks         map[string]chan struct{}
}

func (s *Homes) Scope(b port.Binding) string { return s.Codec.Scope(b) }
func (s *Homes) List(ctx context.Context) ([]string, port.Outcome) {
	if s.Before != nil {
		s.Before("list")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return nil, port.Incomplete
	}
	if s.Failure != "" {
		return nil, s.Failure
	}
	var result []string
	for key := range s.Scopes {
		result = append(result, key)
	}
	slices.Sort(result)
	return result, ""
}
func (s *Homes) Open(ctx context.Context, key string, create bool) (port.HomeTransaction, port.Outcome) {
	if s.Before != nil {
		s.Before("open")
	}
	s.mu.Lock()
	if s.locks == nil {
		s.locks = map[string]chan struct{}{}
	}
	if s.Scopes == nil {
		s.Scopes = map[string]port.HomeScope{}
	}
	lock := s.locks[key]
	if lock == nil {
		lock = make(chan struct{}, 1)
		lock <- struct{}{}
		s.locks[key] = lock
	}
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, port.Incomplete
	case <-lock:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Failure != "" {
		lock <- struct{}{}
		return nil, s.Failure
	}
	if s.MissingOpen {
		lock <- struct{}{}
		return nil, ""
	}
	if _, ok := s.Scopes[key]; !ok && !create {
		lock <- struct{}{}
		return nil, ""
	}
	return &homeTransaction{store: s, key: key, lock: lock, ctx: budget.Lifetime(ctx)}, ""
}

type homeTransaction struct {
	store *Homes
	key   string
	lock  chan struct{}
	ctx   context.Context
}

func (t *homeTransaction) Close() { t.lock <- struct{}{} }
func cloneHome(s port.HomeScope) port.HomeScope {
	s.Generations = slices.Clone(s.Generations)
	for i := range s.Generations {
		s.Generations[i].Claims = slices.Clone(s.Generations[i].Claims)
	}
	return s
}
func (t *homeTransaction) Read() (port.HomeScope, port.Outcome) {
	if t.store.Before != nil {
		t.store.Before("read")
	}
	t.store.mu.Lock()
	defer t.store.mu.Unlock()
	if t.ctx.Err() != nil {
		return port.HomeScope{}, port.Incomplete
	}
	if t.store.ReadFailure != "" {
		return port.HomeScope{}, t.store.ReadFailure
	}
	return cloneHome(t.store.Scopes[t.key]), t.store.Failure
}
func (t *homeTransaction) Save(s port.HomeScope) port.Outcome {
	if t.ctx.Err() != nil {
		return port.Incomplete
	}
	if t.store.BeforeSave != nil {
		if f := t.store.BeforeSave(s); f != "" {
			return f
		}
	}
	t.store.mu.Lock()
	defer t.store.mu.Unlock()
	if t.ctx.Err() != nil {
		return port.Incomplete
	}
	if t.store.Failure != "" {
		return t.store.Failure
	}
	if _, f := t.store.Codec.DecodeHome(t.store.Codec.EncodeHome(s)); f != "" {
		return f
	}
	t.store.Scopes[t.key] = cloneHome(s)
	return ""
}
func (s *Homes) Claim(r port.Record, id string) (port.HomeClaim, port.Outcome) {
	if s.ClaimFailure != "" {
		return port.HomeClaim{}, s.ClaimFailure
	}
	return port.HomeClaim{Session: id, StateRoot: "/fixture/state/itos-container", Intent: port.Reference(r.Project, r.NameToken())}, ""
}
func (s *Homes) CheckClaims(_ context.Context, _ port.HomeScope, g port.HomeGeneration) port.Outcome {
	if s.Before != nil {
		s.Before("claims")
	}
	if s.ClaimsFailure != "" {
		return s.ClaimsFailure
	}
	if len(g.Claims) != 0 {
		return port.HomeInUse
	}
	return ""
}

type HomeRoot struct {
	UID, GID  string
	Mode      int
	Directory bool
	Marker    string
}
type HomeEngine struct {
	*RunEngine
	Homes      map[string]port.HomeObservation
	Roots      map[string]HomeRoot
	Workers    map[string]string
	HomeEvents []string
	HomeFault  string
	HomeBefore func(string)
	mu         sync.Mutex
}

func (e *HomeEngine) OpenRun(ctx context.Context, b port.Binding) (port.RunConnection, port.Outcome) {
	c, f := e.RunEngine.OpenRun(ctx, b)
	if f != "" {
		return nil, f
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.Homes == nil {
		e.Homes = map[string]port.HomeObservation{}
	}
	if e.Roots == nil {
		e.Roots = map[string]HomeRoot{}
	}
	if e.Workers == nil {
		e.Workers = map[string]string{}
	}
	return &homeConnection{RunConnection: c, home: e}, ""
}

type homeConnection struct {
	port.RunConnection
	home *HomeEngine
}

func (c *homeConnection) event(name string) {
	if c.home.HomeBefore != nil {
		c.home.HomeBefore(name)
	}
	c.home.HomeEvents = append(c.home.HomeEvents, name)
}
func (c *homeConnection) CreateRun(ctx context.Context, r port.Record, argv []string) (string, port.Outcome) {
	id, f := c.RunConnection.CreateRun(ctx, r, argv)
	if f == "" && r.Home != (port.HomeReference{}) {
		c.home.Workers[id] = r.Home.Name
		c.home.resourcesMu.Lock()
		o := c.home.Resources[id]
		o.HomeName, o.HomeExact = r.Home.Name, true
		c.home.Resources[id] = o
		c.home.resourcesMu.Unlock()
	}
	return id, f
}
func (c *homeConnection) AllocateHome(ctx context.Context, h port.HomeReference) port.Outcome {
	c.event("allocate")
	if ctx.Err() != nil {
		return port.HomeUnavailable
	}
	if _, ok := c.home.Homes[h.Name]; ok {
		return port.HomeRefused
	}
	c.home.Homes[h.Name] = port.HomeObservation{Name: h.Name, Created: "generation-" + strconv.Itoa(len(c.home.HomeEvents)), Driver: "local", Scope: "local", Token: h.ID, Key: h.Scope, ExactLabels: true, EmptyOptions: true}
	c.home.Roots[h.Name] = HomeRoot{UID: "0", GID: "0", Mode: 0755, Directory: true}
	if c.home.HomeFault == "create" {
		return port.HomeUnavailable
	}
	return ""
}
func (c *homeConnection) InspectHome(ctx context.Context, name string) (port.HomeObservation, port.Inspection) {
	c.event("inspect")
	if ctx.Err() != nil || c.home.HomeFault == "inspect" {
		return port.HomeObservation{}, port.Unavailable
	}
	o, ok := c.home.Homes[name]
	if !ok {
		return o, port.NotFound
	}
	return o, port.Observed
}
func (c *homeConnection) InitializeHome(ctx context.Context, id, uid, gid string) port.Outcome {
	c.event("initialize")
	if ctx.Err() != nil || c.home.HomeFault == "initialize" {
		return port.HomeUnavailable
	}
	name := c.home.Workers[id]
	r, ok := c.home.Roots[name]
	if !ok {
		return port.HomeRefused
	}
	r.UID, r.GID, r.Mode, r.Directory = uid, gid, 0700, true
	c.home.Roots[name] = r
	return ""
}
func (c *homeConnection) VerifyHomeRoot(ctx context.Context, id, uid, gid string) port.Outcome {
	c.event("root")
	if ctx.Err() != nil {
		return port.HomeUnavailable
	}
	r, ok := c.home.Roots[c.home.Workers[id]]
	if !ok || !r.Directory || r.Mode != 0700 || r.UID != uid || r.GID != gid {
		return port.HomeRefused
	}
	return ""
}
func (c *homeConnection) RemoveHome(ctx context.Context, name string) port.Outcome {
	c.event("remove")
	if ctx.Err() != nil || c.home.HomeFault == "remove" {
		return port.Incomplete
	}
	delete(c.home.Homes, name)
	delete(c.home.Roots, name)
	return ""
}
