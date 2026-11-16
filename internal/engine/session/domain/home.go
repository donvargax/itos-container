package domain

import (
	"context"
	"slices"
	"strings"
	"time"

	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

type Homes struct {
	Store   port.HomeStore
	Engine  port.RunEngine
	Entropy port.Entropy
}

func homeGeneration(s port.HomeScope, id string) int {
	return slices.IndexFunc(s.Generations, func(g port.HomeGeneration) bool { return g.ID == id })
}
func newHome(id string) port.HomeGeneration {
	return port.HomeGeneration{ID: id, Name: "itos-container-home-" + id, Stage: "awaiting-allocation", Claims: []port.HomeClaim{}}
}
func homeFailure(f port.Outcome) port.Outcome {
	if f == port.Refused || f == port.Unsafe || f == port.Invalid || f == port.HomeRefused {
		return port.HomeRefused
	}
	if f != "" {
		return port.HomeUnavailable
	}
	return ""
}

func (h Homes) open(ctx context.Context, scope string, create bool) (port.HomeTransaction, port.Outcome) {
	type result struct {
		t port.HomeTransaction
		f port.Outcome
	}
	r, err := budget.Acquire(ctx, "home scope lock", func(call context.Context) result { t, f := h.Store.Open(call, scope, create); return result{t, f} }, func(r result) {
		if r.t != nil {
			r.t.Close()
		}
	})
	if err != nil {
		return nil, port.HomeUnavailable
	}
	if r.t == nil {
		return nil, r.f
	}
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	return &finiteHomeTransaction{HomeTransaction: r.t, ctx: ctx, gate: gate}, r.f
}

// prepare records the session reference and durable claim before allocation.
// An uncertain allocation is only reconciled by stop, never retried by launch.
func (h Homes) prepare(ctx context.Context, entry port.Session, c port.RunConnection) port.Outcome {
	i := entry.Intent()
	b := i.Binding()
	key := h.Store.Scope(b)
	t, f := h.open(ctx, key, true)
	if f != "" {
		return homeFailure(f)
	}
	if t == nil {
		return port.HomeUnavailable
	}
	defer t.Close()
	s, f := t.Read()
	if f != "" {
		return homeFailure(f)
	}
	if s.Schema == 0 {
		id, f := h.Entropy.Token()
		if f != "" || !port.HexID(id) {
			return port.HomeUnavailable
		}
		s = port.HomeScope{Schema: 1, Scope: key, Project: b.Frozen.Project, UID: b.UID, GID: b.GID,
			Engine: port.HomeEngineIdentity{Endpoint: b.Endpoint, ID: b.EngineID}, Current: id, Generations: []port.HomeGeneration{newHome(id)}}
		if f = t.Save(s); f != "" {
			return homeFailure(f)
		}
	}
	if s.Project != b.Frozen.Project || s.UID != b.UID || s.GID != b.GID || s.Engine.Endpoint != b.Endpoint || s.Engine.ID != b.EngineID {
		return port.HomeRefused
	}
	n := homeGeneration(s, s.Current)
	if n < 0 {
		return port.HomeRefused
	}
	g := &s.Generations[n]
	if state, ok := ctx.Value(startupKey{}).(*startupState); ok {
		state.homeID = g.ID
	}
	if g.Stage == "deleted" || g.Stage == "reset" {
		return port.HomeRefused
	}
	if strings.HasSuffix(g.Stage, "-pending") || g.Stage == "intent" || len(g.Claims) != 0 {
		return port.HomeUnavailable
	}
	r := i.Record()
	r.Home = g.Reference(key)
	if f = i.Save(r); f != "" {
		return homeFailure(f)
	}
	claim, f := h.Store.Claim(r, entry.Record().ID)
	if f != "" {
		return homeFailure(f)
	}
	g.Claims = append(g.Claims, claim)
	if f = t.Save(s); f != "" {
		return homeFailure(f)
	}
	hc, ok := c.(port.HomeConnection)
	if !ok {
		return port.HomeUnavailable
	}
	o, status := hc.InspectHome(ctx, g.Name)
	if g.Stage == "awaiting-allocation" {
		if status == port.Observed || status == port.Changed {
			return port.HomeRefused
		}
		if status != port.NotFound {
			return port.HomeUnavailable
		}
		g.Stage = "intent"
		if f = t.Save(s); f != "" {
			return homeFailure(f)
		}
		if f = hc.AllocateHome(ctx, r.Home); f != "" {
			return homeFailure(f)
		}
		o, status = hc.InspectHome(ctx, g.Name)
		if status != port.Observed {
			return port.HomeUnavailable
		}
		if !r.Home.Owns(o) {
			return port.HomeRefused
		}
		g.Created, g.Stage = o.Created, "retained"
		if f = t.Save(s); f != "" {
			return homeFailure(f)
		}
	} else if status == port.NotFound || status == port.Changed {
		return port.HomeRefused
	} else if status != port.Observed {
		return port.HomeUnavailable
	}
	if !g.Reference(key).Owns(o) {
		return port.HomeRefused
	}
	r.Home = g.Reference(key)
	return homeFailure(i.Save(r))
}

func (h Homes) verify(ctx context.Context, entry port.Session, c port.RunConnection, initialize bool) port.Outcome {
	r := entry.Intent().Record()
	t, f := h.open(ctx, r.Home.Scope, false)
	if f != "" {
		return homeFailure(f)
	}
	if t == nil {
		return port.HomeRefused
	}
	defer t.Close()
	s, f := t.Read()
	if f != "" {
		return homeFailure(f)
	}
	n := homeGeneration(s, r.Home.ID)
	if n < 0 || s.Current != r.Home.ID {
		return port.HomeRefused
	}
	g := &s.Generations[n]
	if g.Stage != "retained" || g.Reference(s.Scope) != r.Home || len(g.Claims) != 1 || g.Claims[0].Session != entry.Record().ID {
		return port.HomeRefused
	}
	hc, ok := c.(port.HomeConnection)
	if !ok {
		return port.HomeUnavailable
	}
	o, status := hc.InspectHome(ctx, g.Name)
	if status == port.NotFound || status == port.Changed {
		return port.HomeRefused
	}
	if status != port.Observed {
		return port.HomeUnavailable
	}
	if !r.Home.Owns(o) {
		return port.HomeRefused
	}
	if !g.Initialized {
		if !initialize {
			return port.HomeRefused
		}
		if f = hc.InitializeHome(ctx, r.ContainerID, r.UID, r.GID); f != "" {
			return homeFailure(f)
		}
	}
	if f = hc.VerifyHomeRoot(ctx, r.ContainerID, r.UID, r.GID); f != "" {
		return homeFailure(f)
	}
	if !g.Initialized {
		g.Initialized = true
		if f = t.Save(s); f != "" {
			return homeFailure(f)
		}
	}
	return ""
}

// reconcile never removes home storage. Concrete ownership must be accounted
// for even after a delayed create, before a completed session releases its claim.
func (h Homes) reconcile(ctx context.Context, entry port.Session, c port.RunConnection, release bool) (string, port.Outcome) {
	r := entry.Intent().Record()
	t, f := h.open(ctx, r.Home.Scope, false)
	if f != "" {
		return "unknown", f
	}
	if t == nil {
		return "unknown", port.Invalid
	}
	defer t.Close()
	s, f := t.Read()
	if f != "" {
		return "unknown", f
	}
	n := homeGeneration(s, r.Home.ID)
	if n < 0 {
		return "unknown", port.Invalid
	}
	g := &s.Generations[n]
	if g.Name != r.Home.Name || (r.Home.Created != "" && r.Home.Created != g.Created) || s.Project != r.Project {
		return "unknown", port.Refused
	}
	hc, ok := c.(port.HomeConnection)
	if !ok {
		return "unknown", port.Incomplete
	}
	o, status := hc.InspectHome(ctx, g.Name)
	state := "unknown"
	switch status {
	case port.Changed:
		return state, port.Refused
	case port.Unavailable:
		return state, port.Incomplete
	case port.Observed:
		if !g.Reference(s.Scope).Owns(o) {
			return state, port.Refused
		}
		if g.Stage == "awaiting-allocation" {
			return state, port.Refused
		}
		state = "present"
		if g.Stage == "intent" {
			if !release {
				return state, port.Incomplete
			}
			g.Created, g.Stage = o.Created, "retained"
			if f = t.Save(s); f != "" {
				return state, f
			}
		}
	case port.NotFound:
		state = "absent"
		if g.Stage == "intent" {
			return state, port.Incomplete
		}
	}
	if release && r.Home.Created == "" && g.Created != "" {
		r.Home.Created = g.Created
		if f = entry.Intent().Save(r); f != "" {
			return state, f
		}
	}
	if release && entry.Intent().Record().Completed() {
		g.Claims = slices.DeleteFunc(g.Claims, func(c port.HomeClaim) bool { return c.Session == entry.Record().ID })
		if f = t.Save(s); f != "" {
			return state, f
		}
	}
	return state, ""
}

func homeEntry(s port.HomeScope, g port.HomeGeneration) port.HomeEntry {
	e := port.HomeEntry{ID: g.ID, Scope: s.Scope, Project: s.Project, UID: s.UID, GID: s.GID, Engine: s.Engine, Name: g.Name, State: "unknown", Disposition: g.Stage, References: []string{}}
	if g.Successor != "" {
		value := g.Successor
		e.Successor = &value
	}
	for _, claim := range g.Claims {
		e.References = append(e.References, claim.Session)
	}
	slices.Sort(e.References)
	if strings.HasSuffix(g.Stage, "-pending") {
		e.Retry = []string{"itos", "container", "stop", "--home", g.ID, "--" + strings.TrimSuffix(g.Stage, "-pending")}
	}
	return e
}
func homeResult(e *port.HomeEntry, f port.Outcome) port.HomeResult {
	r := port.HomeResult{Homes: []port.HomeEntry{}, Failures: []port.RecoveryFailure{}}
	if e != nil {
		r.Homes = append(r.Homes, *e)
	}
	if f != "" {
		r.Failures = append(r.Failures, port.RecoveryFailure{Outcome: f})
	}
	return r
}
func (h Homes) observe(ctx context.Context, s port.HomeScope, g port.HomeGeneration, selecting bool) (port.HomeEntry, port.Outcome) {
	e := homeEntry(s, g)
	c, f := (finiteEngine{h.Engine}).OpenRun(ctx, s.Binding())
	if f != "" {
		if selecting && f == port.Refused {
			f = port.Environment
		}
		return e, f
	}
	defer c.Close()
	if f = c.Check(ctx); f != "" {
		return e, f
	}
	// OpenRun above always supplies the finite wrapper. It reports a missing
	// underlying home capability as an unavailable observation.
	hc := c.(finiteConnection)
	o, status := hc.InspectHome(ctx, g.Name)
	switch status {
	case port.NotFound:
		e.State = "absent"
		if g.Stage == "intent" {
			return e, port.Incomplete
		}
	case port.Observed:
		if g.Created == "" || !g.Reference(s.Scope).Owns(o) {
			return e, port.Refused
		}
		e.State = "present"
	case port.Changed:
		return e, port.Refused
	default:
		return e, port.Incomplete
	}
	return e, ""
}
func (h Homes) ListHomes(ctx context.Context) port.HomeResult {
	ctx, cancel := context.WithTimeout(ctx, budget.CallDuration)
	defer cancel()
	ids, f := h.list(ctx)
	if f != "" {
		return homeResult(nil, f)
	}
	r := homeResult(nil, "")
	for _, id := range ids {
		t, f := h.open(ctx, id, false)
		if f != "" {
			return homeResult(nil, f)
		}
		if t == nil {
			return homeResult(nil, port.Invalid)
		}
		s, f := t.Read()
		t.Close()
		if f != "" {
			return homeResult(nil, f)
		}
		if s.Schema == 0 {
			return homeResult(nil, port.Invalid)
		}
		for _, g := range s.Generations {
			e, f := h.observe(ctx, s, g, true)
			r.Homes = append(r.Homes, e)
			if f != "" {
				r.Failures = append(r.Failures, port.RecoveryFailure{Outcome: f})
			}
		}
	}
	slices.SortFunc(r.Homes, func(a, b port.HomeEntry) int { return strings.Compare(a.ID, b.ID) })
	return r
}

func (h Homes) locate(ctx context.Context, id string) (port.HomeScope, port.HomeGeneration, port.Outcome) {
	keys, f := h.list(ctx)
	if f != "" {
		return port.HomeScope{}, port.HomeGeneration{}, f
	}
	for _, key := range keys {
		t, f := h.open(ctx, key, false)
		if f != "" {
			return port.HomeScope{}, port.HomeGeneration{}, f
		}
		if t == nil {
			return port.HomeScope{}, port.HomeGeneration{}, port.Invalid
		}
		s, f := t.Read()
		t.Close()
		if f != "" {
			return s, port.HomeGeneration{}, f
		}
		if s.Schema == 0 {
			return s, port.HomeGeneration{}, port.Invalid
		}
		if n := homeGeneration(s, id); n >= 0 {
			return s, s.Generations[n], ""
		}
	}
	return port.HomeScope{}, port.HomeGeneration{}, port.Refused
}
func (h Homes) Retire(ctx context.Context, id, action string, consent port.HomeConsent) port.HomeResult {
	if !port.HexID(id) || (action != "reset" && action != "delete") {
		return homeResult(nil, port.Usage)
	}
	review, end := context.WithTimeout(ctx, budget.CallDuration)
	s, g, f := h.locate(review, id)
	if f != "" {
		end()
		return homeResult(nil, f)
	}
	e, f := h.observe(review, s, g, true)
	end()
	if f != "" {
		return homeResult(&e, f)
	}
	if g.Stage == "intent" || g.Stage == "awaiting-allocation" {
		return homeResult(&e, port.Incomplete)
	}
	completed := g.Stage == action || action == "delete" && g.Stage == "deleted"
	if g.Stage == "reset" && action != "reset" || strings.HasSuffix(g.Stage, "-pending") && g.Stage != action+"-pending" {
		return homeResult(&e, port.Refused)
	}
	if !completed && (consent == nil || !consent.Review(ctx, e, action)) {
		return homeResult(&e, port.ConsentRequired)
	}
	mutation, finish := context.WithTimeout(ctx, 30*time.Second)
	defer finish()
	return h.retire(mutation, s, g, e, action, completed)
}

func (h Homes) retire(ctx context.Context, reviewed port.HomeScope, old port.HomeGeneration, e port.HomeEntry, action string, completed bool) port.HomeResult {
	t, f := h.open(ctx, reviewed.Scope, false)
	if f != "" {
		return homeResult(&e, f)
	}
	if t == nil {
		return homeResult(&e, port.Refused)
	}
	defer t.Close()
	s, f := t.Read()
	if f != "" {
		return homeResult(&e, f)
	}
	n := homeGeneration(s, old.ID)
	if n < 0 || s.Current != reviewed.Current || s.Project != reviewed.Project || s.UID != reviewed.UID || s.GID != reviewed.GID || s.Engine != reviewed.Engine {
		return homeResult(&e, port.Refused)
	}
	g := &s.Generations[n]
	if g.Reference(s.Scope) != old.Reference(s.Scope) || g.Stage != old.Stage || g.Successor != old.Successor {
		return homeResult(&e, port.Refused)
	}
	if f = h.checkClaims(ctx, s, *g); f != "" {
		current := homeEntry(s, *g)
		current.State = e.State
		result := homeResult(&current, f)
		result.Failures[0].HomeClaims = slices.Clone(g.Claims)
		return result
	}
	fresh, f := h.observe(ctx, s, *g, false)
	if f != "" {
		return homeResult(&e, f)
	}
	if fresh.State != e.State {
		return homeResult(&e, port.Refused)
	}
	if completed {
		if fresh.State != "absent" {
			return homeResult(&e, port.Refused)
		}
		return homeResult(&fresh, t.Save(s))
	}
	if !strings.HasSuffix(g.Stage, "-pending") {
		if action == "reset" {
			id, f := h.Entropy.Token()
			if f != "" || !port.HexID(id) || homeGeneration(s, id) >= 0 {
				return homeResult(&e, port.Internal)
			}
			g.Successor = id
		}
		g.Stage = action + "-pending"
		if f = t.Save(s); f != "" {
			return homeResult(&e, f)
		}
	}
	e = homeEntry(s, *g)
	c, f := (finiteEngine{h.Engine}).OpenRun(ctx, s.Binding())
	if f != "" {
		return homeResult(&e, f)
	}
	defer c.Close()
	if f = c.Check(ctx); f != "" {
		return homeResult(&e, f)
	}
	hc := c.(finiteConnection)
	o, status := hc.InspectHome(ctx, g.Name)
	if status == port.Observed {
		if !g.Reference(s.Scope).Owns(o) {
			return homeResult(&e, port.Refused)
		}
		latest, failure := t.Read()
		if failure != "" {
			return homeResult(&e, failure)
		}
		if !sameHomeScope(s, latest) {
			return homeResult(&e, port.Refused)
		}
		if f = h.checkClaims(ctx, latest, latest.Generations[n]); f != "" {
			return homeResult(&e, f)
		}
		if f = c.Check(ctx); f != "" {
			return homeResult(&e, f)
		}
		_ = hc.RemoveHome(ctx, g.Name)
		_, status = hc.InspectHome(ctx, g.Name)
	}
	if status == port.Changed {
		return homeResult(&e, port.Refused)
	}
	if status != port.NotFound || ctx.Err() != nil {
		return homeResult(&e, port.Incomplete)
	}
	if action == "reset" {
		g.Stage = "reset"
		s.Current = g.Successor
		s.Generations = append(s.Generations, newHome(g.Successor))
	} else {
		g.Stage = "deleted"
	}
	e.State = "absent"
	if f = t.Save(s); f != "" {
		return homeResult(&e, f)
	}
	e = homeEntry(s, s.Generations[n])
	e.State = "absent"
	return homeResult(&e, "")
}

func sameHomeScope(a, b port.HomeScope) bool {
	if a.Schema != b.Schema || a.Scope != b.Scope || a.Project != b.Project || a.UID != b.UID || a.GID != b.GID || a.Engine != b.Engine || a.Current != b.Current || len(a.Generations) != len(b.Generations) {
		return false
	}
	for i, x := range a.Generations {
		y := b.Generations[i]
		if x.ID != y.ID || x.Name != y.Name || x.Created != y.Created || x.Stage != y.Stage || x.Initialized != y.Initialized || x.Successor != y.Successor || !slices.Equal(x.Claims, y.Claims) {
			return false
		}
	}
	return true
}
