package main

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	ad "github.com/donvargax/itos-container/internal/engine/approval/disk"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

type homeStateSelection struct {
	sp.HomeStore
	root   string
	opened chan<- struct{}
	once   *sync.Once
}

func (s homeStateSelection) Claim(r sp.Record, id string) (sp.HomeClaim, sp.Outcome) {
	c, f := s.HomeStore.Claim(r, id)
	c.StateRoot = s.root
	return c, f
}
func (s homeStateSelection) Open(ctx context.Context, key string, create bool) (sp.HomeTransaction, sp.Outcome) {
	if s.opened != nil {
		s.once.Do(func() { close(s.opened) })
	}
	return s.HomeStore.Open(ctx, key, create)
}

// Same account and daemon, separate selected session stores. The catalog is
// shared because it is scoped to the account, independently of XDG_STATE_HOME.
func homeOtherState(t *testing.T, first *homeLocalFixture) *homeLocalFixture {
	t.Helper()
	second := newHomeLocalFixture(t, "1000", "1000")
	second.next = 1 << 32
	second.engine = first.engine
	second.m.b = first.m.b
	second.homes = first.homes
	second.sdk = first.sdk
	var settings map[string]any
	if err := json.Unmarshal([]byte(first.m.c.Canonical), &settings); err != nil {
		t.Fatal(err)
	}
	second.settings(t, settings, true)
	if second.disk != nil {
		second.disk.account, second.disk.catalog = first.disk.account, first.disk.catalog
		second.disk.store.Storage = homeFixtureCatalog{ad.Catalog{Account: homeFixtureAccount(first.disk.account)}}
	}
	return second
}

func homeSharedScope(t *testing.T) {
	homePositive(t)
	first := newHomeLocalFixture(t, "1000", "1000")
	second := homeOtherState(t, first)
	firstStore, secondStore := first.service().Store, second.service().Store
	firstRoot, secondRoot := first.stateRoot(), second.stateRoot()
	if first.disk == nil {
		secondRoot = "/fixture/other-state/itos-container"
	}
	if firstRoot == secondRoot {
		t.Fatal("state selections must differ")
	}
	first.homeStore = homeStateSelection{HomeStore: firstStore, root: firstRoot}
	attempted := make(chan struct{})
	second.homeStore = homeStateSelection{HomeStore: secondStore, root: secondRoot, opened: attempted, once: &sync.Once{}}
	reached, release := make(chan struct{}), make(chan struct{})
	var allocation sync.Once
	first.homes.BeforeSave = func(s sp.HomeScope) sp.Outcome {
		if len(s.Generations) > 0 && s.Generations[0].Stage == "intent" {
			allocation.Do(func() { close(reached); <-release })
		}
		return ""
	}
	type result struct {
		code            int
		out, diagnostic string
	}
	invoke := func(f *homeLocalFixture, done chan<- result) {
		code, out, d := f.command(t, "run", "--", "probe")
		done <- result{code, out, d}
	}
	firstDone, secondDone := make(chan result, 1), make(chan result, 1)
	go invoke(first, firstDone)
	select {
	case <-reached:
	case r := <-firstDone:
		t.Fatal("first run missed allocation barrier", r)
	case <-time.After(5 * time.Second):
		t.Fatal("first run did not reach allocation")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	second.ctx = ctx
	go invoke(second, secondDone)
	select {
	case <-attempted:
	case r := <-secondDone:
		close(release)
		<-firstDone
		t.Fatal("second state did not contend for shared home", r)
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("second run did not reach catalog")
	}
	cancel()
	blocked := <-secondDone
	close(release)
	completed := <-firstDone
	first.homes.BeforeSave = nil
	if completed.code != 0 || blocked.code == 0 {
		t.Fatal("concurrent run admission", completed, blocked)
	}
	scope := first.scope()
	if len(scope.Generations) != 1 || len(scope.Generations[0].Claims) != 0 || len(first.engine.Resources) != 0 {
		t.Fatal("first run lost home or session ownership", scope)
	}
	g := scope.Generations[0]
	root := first.engine.Roots[g.Name]
	root.Marker = "cross-state marker"
	first.engine.Roots[g.Name] = root
	second.ctx = nil
	homeRequireRun(t, second, 0, "")
	counts := map[string]int{}
	for _, event := range first.engine.HomeEvents {
		counts[event]++
	}
	if counts["allocate"] != 1 || counts["initialize"] != 1 || counts["remove"] != 0 || len(first.engine.Homes) != 1 || first.engine.Roots[g.Name].Marker != root.Marker {
		t.Fatal("state selection reset shared home", counts)
	}
	if second.scope().Current != g.ID {
		t.Fatal("second selection did not reuse same generation")
	}
	for _, s := range second.scope().Generations {
		for _, c := range s.Claims {
			if strings.Contains(c.StateRoot, "other-state") {
				t.Fatal("later run left a claim")
			}
		}
	}
}
