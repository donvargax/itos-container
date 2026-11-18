package porttest

import (
	"context"
	"slices"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// HomeStoreContract is shared by the in-memory store and protected disk adapter.
func HomeStoreContract(t Reporter, store port.HomeStore, binding port.Binding) {
	t.Helper()
	ctx := context.Background()
	key := store.Scope(binding)
	if !port.HexID(key) {
		t.Errorf("invalid scope key %q", key)
		return
	}
	if ids, f := store.List(ctx); f != "" || len(ids) != 0 {
		t.Errorf("empty catalog = %v %s", ids, f)
	}
	if tx, f := store.Open(ctx, key, false); f != "" || tx != nil {
		t.Errorf("missing read created a scope: %v %s", tx, f)
		return
	}
	tx, f := store.Open(ctx, key, true)
	if f != "" || tx == nil {
		t.Errorf("scope creation: %s", f)
		return
	}
	id := strings.Repeat("a", 64)
	s := port.HomeScope{Schema: 1, Scope: key, Project: binding.Frozen.Project, UID: binding.UID, GID: binding.GID, Engine: port.HomeEngineIdentity{Endpoint: binding.Endpoint, ID: binding.EngineID}, Current: id, Generations: []port.HomeGeneration{{ID: id, Name: "itos-container-home-" + id, Stage: "awaiting-allocation", Claims: []port.HomeClaim{}}}}
	if f = tx.Save(s); f != "" {
		tx.Close()
		t.Errorf("scope publication: %s", f)
		return
	}
	got, f := tx.Read()
	if f != "" || got.Scope != key || len(got.Generations) != 1 || got.Generations[0].ID != id {
		t.Errorf("published scope differs: %+v %s", got, f)
	}
	got.Generations[0].Name = "changed local copy"
	got, f = tx.Read()
	if f != "" || got.Generations[0].Name != s.Generations[0].Name {
		t.Errorf("read aliases protected data: %+v %s", got, f)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if other, f := store.Open(canceled, key, false); f == "" || other != nil {
		t.Errorf("cancelled lock acquired authority: %v %s", other, f)
		if other != nil {
			other.Close()
		}
	}
	tx.Close()
	tx, f = store.Open(ctx, key, false)
	if f != "" || tx == nil {
		t.Errorf("fresh controller lost scope: %s", f)
		return
	}
	if f = tx.Save(s); f != "" {
		t.Errorf("durability retry: %s", f)
	}
	tx.Close()
	if ids, f := store.List(ctx); f != "" || len(ids) != 1 || ids[0] != key {
		t.Errorf("catalog discovery: %v %s", ids, f)
	}
	image := binding
	image.ImageID = "another-image"
	image.Frozen.Generation = "2"
	if store.Scope(image) != key {
		t.Errorf("image or approval generation reset scope")
	}
	for _, kind := range []string{"uid", "gid", "checkout", "endpoint", "daemon"} {
		other := binding
		switch kind {
		case "uid":
			other.UID = "2001"
		case "gid":
			other.GID = "3001"
		case "checkout":
			other.Frozen.Project = strings.Repeat("e", 64)
		case "endpoint":
			other.Endpoint = "unix:///another.sock"
		case "daemon":
			other.EngineID = "another-daemon"
		}
		if store.Scope(other) == key {
			t.Errorf("%s did not isolate scope", kind)
		}
	}
	// An interrupted first run can leave a created scope directory holding no
	// published record. It stays discoverable and reads as an unallocated
	// scope, never as another scope's invalid catalog.
	other := binding
	other.GID = "3001"
	empty := store.Scope(other)
	created, f := store.Open(ctx, empty, true)
	if f != "" || created == nil {
		t.Errorf("unallocated scope creation: %v %s", created, f)
		return
	}
	created.Close()
	discovered, f := store.List(ctx)
	if f != "" || !slices.Contains(discovered, key) || !slices.Contains(discovered, empty) {
		t.Errorf("unallocated scope discovery: %v %s", discovered, f)
	}
	if tx, f = store.Open(ctx, empty, false); f != "" || tx == nil {
		t.Errorf("unallocated scope read: %v %s", tx, f)
		return
	}
	if unallocated, f := tx.Read(); f != "" || unallocated.Schema != 0 || unallocated.Scope != "" || len(unallocated.Generations) != 0 {
		t.Errorf("unallocated scope record: %+v %s", unallocated, f)
	}
	tx.Close()
}

// HomeEngineContract never starts a worker or reads worker file bodies.
func HomeEngineContract(t Reporter, engine port.RunEngine, binding port.Binding, key string) {
	t.Helper()
	ctx := context.Background()
	c, f := engine.OpenRun(ctx, binding)
	if f != "" || c == nil {
		t.Errorf("home engine open: %s", f)
		return
	}
	defer c.Close()
	h, ok := c.(port.HomeConnection)
	if !ok {
		t.Errorf("home capability unavailable")
		return
	}
	reference := port.HomeReference{Scope: key, ID: strings.Repeat("a", 64), Name: "itos-container-home-" + strings.Repeat("a", 64)}
	if _, state := h.InspectHome(ctx, reference.Name); state != port.NotFound {
		t.Errorf("missing volume not independently absent")
	}
	if f = h.AllocateHome(ctx, reference); f != "" {
		t.Errorf("home allocation: %s", f)
		return
	}
	observation, state := h.InspectHome(ctx, reference.Name)
	if state != port.Observed || !reference.Owns(observation) {
		t.Errorf("allocation tuple: %+v %d", observation, state)
		return
	}
	reference.Created = observation.Created
	r := binding.Intent(strings.Repeat("b", 64))
	r.Home = reference
	r.Schema = 2
	r.Anchor = port.Anchor{Session: strings.Repeat("c", 64), Name: "itos-container-anchor-" + r.NameToken(), Source: "/proc/42/fd/7", Device: binding.Checkout.Device, Inode: binding.Checkout.Inode, Stage: "intent"}
	anchor, ok := c.(port.AnchorConnection)
	if !ok {
		t.Errorf("anchor capability unavailable")
		return
	}
	if f = anchor.AllocateAnchor(ctx, r); f != "" {
		t.Errorf("anchor allocation: %s", f)
		return
	}
	a, status := anchor.InspectAnchor(ctx, r.Anchor.Name)
	if status != port.Observed {
		t.Errorf("anchor unavailable")
		return
	}
	r.Anchor.Created, r.Anchor.Stage = a.Created, "owned"
	id, f := c.CreateRun(ctx, r, []string{"probe"})
	if f != "" {
		t.Errorf("unstarted worker: %s", f)
		return
	}
	if f = h.InitializeHome(ctx, id, binding.UID, binding.GID); f != "" {
		t.Errorf("root initialization: %s", f)
		return
	}
	if f = h.VerifyHomeRoot(ctx, id, binding.UID, binding.GID); f != "" {
		t.Errorf("root verification: %s", f)
	}
	if f = h.VerifyHomeRoot(ctx, id, binding.UID, "42"); f != port.HomeRefused {
		t.Errorf("wrong root owner accepted: %s", f)
	}
	c.Remove(ctx, id)
	anchor.RemoveAnchor(ctx, r.Anchor.Name)
	if f = h.RemoveHome(ctx, reference.Name); f != "" {
		t.Errorf("consented home removal: %s", f)
	}
	if _, status = h.InspectHome(ctx, reference.Name); status != port.NotFound {
		t.Errorf("home not independently absent")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if f = h.AllocateHome(canceled, port.HomeReference{Scope: key, ID: strings.Repeat("d", 64), Name: "itos-container-home-" + strings.Repeat("d", 64)}); f == "" {
		t.Errorf("cancelled allocation succeeded")
	}
}
