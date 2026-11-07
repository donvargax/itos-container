package porttest

import (
	"context"
	"fmt"
	"strings"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

type Reporter interface {
	Helper()
	Errorf(string, ...any)
}

// StoreContract runs unchanged against the memory fake and composed disk store.
func StoreContract(t Reporter, open func() (port.Store, approval.Durable)) {
	t.Helper()
	store, d := open()
	ctx := context.Background()
	token := strings.Repeat("1", 64)
	ref := port.Reference(d.Frozen.Project, token)
	if _, f := store.Open(ctx, "../escape"); f != port.Invalid {
		t.Errorf("unsafe lookup accepted: %s", f)
	}
	if _, f := store.Open(ctx, ref); f != port.Refused {
		t.Errorf("missing ownership accepted: %s", f)
	}
	intent, f := store.New(ctx, d, token)
	if f != "" {
		t.Errorf("intent refused: %s", f)
		return
	}
	if intent.Record().Stage != "intent" || intent.Record().ContainerID != "" {
		t.Errorf("intent invented ID")
	}
	original := intent.Record()
	bad := original
	bad.Stage = "owned"
	if f = intent.Save(bad); f != port.Invalid {
		t.Errorf("owned without ID accepted: %s", f)
	}
	bad.ContainerID = strings.Repeat("2", 64)
	bad.EngineID = "foreign"
	if f = intent.Save(bad); f != port.Invalid {
		t.Errorf("identity rewrite accepted: %s", f)
	}
	owned := original
	owned.Stage = "owned"
	owned.ContainerID = strings.Repeat("2", 64)
	if f = intent.Save(owned); f != "" {
		t.Errorf("owned update: %s", f)
	}
	intent.Close()
	intent, f = store.Open(ctx, ref)
	if f != "" {
		t.Errorf("reconstruction: %s", f)
		return
	}
	if intent.Record() != owned || intent.Validate() != "" {
		t.Errorf("reconstruction lost owned identity")
	}
	absent := owned
	absent.Stage = "absent"
	if f = intent.Save(absent); f != "" {
		t.Errorf("absence update: %s", f)
	}
	if f = intent.Save(absent); f != "" {
		t.Errorf("repeated durable absence: %s", f)
	}
	if f = intent.Save(owned); f != port.Invalid {
		t.Errorf("tombstone regressed: %s", f)
	}
	intent.Close()
	if _, f = store.New(ctx, d, token); f != port.Refused {
		t.Errorf("occupied operation replaced: %s", f)
	}
	next, f := store.New(ctx, d, strings.Repeat("3", 64))
	if f != "" {
		t.Errorf("new independent operation: %s", f)
	} else {
		if next.Record().Snapshot != original.Snapshot {
			t.Errorf("independent operation changed frozen binding")
		}
		next.Close()
	}
	preparedContract(t, store, d)
}

func preparedContract(t Reporter, store port.Store, d approval.Durable) {
	t.Helper()
	for n, stage := range []string{"cancelled", "intent"} {
		token := strings.Repeat(fmt.Sprint(n+4), 64)
		i, failure := store.Prepare(context.Background(), d, token)
		if failure != "" {
			t.Errorf("prepare: %s", failure)
			return
		}
		r := i.Record()
		if r.Stage != "reserved" || r.ContainerID != "" || r.Completed() {
			t.Errorf("new preparation acquired create/completion authority")
		}
		bad := r
		bad.Stage, bad.ContainerID = "owned", strings.Repeat("e", 64)
		if failure = i.Save(bad); failure != port.Invalid {
			t.Errorf("preparation skipped durable create boundary: %s", failure)
		}
		r.Stage = stage
		if failure = i.Save(r); failure != "" {
			t.Errorf("prepared transition to %s: %s", stage, failure)
		}
		i.Close()
		i, failure = store.OpenCleanup(context.Background(), port.Reference(d.Frozen.Project, token))
		if failure != "" {
			t.Errorf("reopen prepared transition: %s", failure)
			return
		}
		if i.Record() != r || r.Completed() != (stage == "cancelled") {
			t.Errorf("prepared transition lost durable authority")
		}
		bad = r
		bad.Stage = "reserved"
		if failure = i.Save(bad); failure != port.Invalid {
			t.Errorf("create/cancellation boundary rolled back: %s", failure)
		}
		if stage == "cancelled" {
			if failure = i.Save(r); failure != "" {
				t.Errorf("repeat cancellation durability: %s", failure)
			}
			bad.Stage = "intent"
			if failure = i.Save(bad); failure != port.Invalid {
				t.Errorf("cancelled intent acquired create authority: %s", failure)
			}
		} else {
			bad.Stage = "cancelled"
			if failure = i.Save(bad); failure != port.Invalid {
				t.Errorf("uncertain create became cancellation: %s", failure)
			}
		}
		i.Close()
	}
}

// EngineContract checks unstarted identity, exact lookup and confirmed absence.
func EngineContract(t Reporter, open func() (port.Engine, port.Binding)) {
	t.Helper()
	engine, b := open()
	ctx := context.Background()
	connection, f := engine.Open(ctx, b)
	if f != "" {
		t.Errorf("engine open: %s", f)
		return
	}
	defer connection.Close()
	if f = connection.Check(ctx); f != "" {
		t.Errorf("engine identity: %s", f)
	}
	record := b.Intent(strings.Repeat("1", 64))
	id, f := connection.Create(ctx, record)
	if f != "" || !port.HexID(id) {
		t.Errorf("create identity: %q %s", id, f)
		return
	}
	observed, status := connection.Inspect(ctx, id)
	if status != port.Observed || observed.ID != id || observed.Name != record.Name || observed.ImageID != record.ImageID || observed.Token != record.NameToken() || observed.Project != record.Project || observed.Fingerprint != record.Fingerprint || !observed.StatePresent || observed.Status != "created" || observed.Running || observed.PID != 0 || observed.Started || observed.Mounts || observed.Volumes {
		t.Errorf("create not independently unstarted: %#v %v", observed, status)
	}
	byName, status := connection.Inspect(ctx, record.Name)
	if status != port.Observed || byName != observed {
		t.Errorf("name and ID disagree")
	}
	connection.Remove(ctx, id)
	if _, status = connection.Inspect(ctx, id); status != port.NotFound {
		t.Errorf("remove not absent: %v", status)
	}
}

// Codec is an in-memory table for domain tests, not a competing wire codec.
type Codec struct {
	Value   port.Binding
	Records map[string]port.Record
}

// Project selects the supplied binding only for its checkout and host user.
func (c *Codec) Project(checkout readiness.CheckoutIdentity, uid string) string {
	if checkout != c.Value.Checkout || uid != c.Value.UID {
		return ""
	}
	return c.Value.Frozen.Project
}

func (c *Codec) Encode(r port.Record) []byte {
	if c.Records == nil {
		c.Records = map[string]port.Record{}
	}
	key := fmt.Sprintf("%#v", r)
	c.Records[key] = r
	return []byte(key)
}
func (c *Codec) Decode(data []byte) (port.Record, port.Outcome) {
	r, ok := c.Records[string(data)]
	if !ok {
		return port.Record{}, port.Invalid
	}
	return r, ""
}
func (c *Codec) Frozen(f approval.Frozen) (port.Binding, port.Outcome) {
	if f != c.Value.Frozen {
		return port.Binding{}, port.Invalid
	}
	return c.Value, ""
}
func (c *Codec) Snapshot(data []byte) (port.Binding, port.Outcome) {
	if string(data) != "snapshot" {
		return port.Binding{}, port.Invalid
	}
	return c.Value, ""
}
func Fixture() (port.Binding, *Store, *Engine) {
	b := port.Binding{Frozen: approval.Frozen{Project: strings.Repeat("a", 64), Generation: "1", Fingerprint: "sha256:" + strings.Repeat("b", 64), Settings: "frozen"}, UID: "1000", GID: "1000", Endpoint: "unix:///fixture.sock", EngineID: "engine-A", ImageReference: "fixture.test/worker@sha256:" + strings.Repeat("c", 64), ImageID: "sha256:" + strings.Repeat("d", 64)}
	b.Checkout.Root = "/checkout"
	codec := &Codec{Value: b}
	store := &Store{Codec: codec, Records: map[string][]byte{}, Snapshots: map[string][]byte{b.Intent(strings.Repeat("1", 64)).Snapshot: []byte("snapshot")}, UID: b.UID, GID: b.GID, Root: b.Checkout.Root}
	engine := &Engine{Store: store, Endpoint: b.Endpoint, DaemonID: b.EngineID, CreateID: strings.Repeat("c", 64), Resources: map[string]port.Observation{}}
	return b, store, engine
}

type Entropy struct {
	Value   string
	Failure port.Outcome
}

func (e Entropy) Token() (string, port.Outcome) { return e.Value, e.Failure }

// EngineImageContract requires image evidence before any create can succeed.
func EngineImageContract(t Reporter, open func(string) (port.Engine, port.Binding)) {
	t.Helper()
	for _, condition := range []string{"declared volume", "different image config ID", "missing Config", "unavailable", "malformed response"} {
		engine, b := open(condition)
		ctx := context.Background()
		connection, failure := engine.Open(ctx, b)
		if failure != "" {
			t.Errorf("%s engine open: %s", condition, failure)
			continue
		}
		record := b.Intent(strings.Repeat("1", 64))
		id, failure := connection.Create(ctx, record)
		want := port.Uncertain
		if condition == "declared volume" || condition == "different image config ID" {
			want = port.Refused
		}
		if id != "" || failure != want {
			t.Errorf("%s permitted create: %q %s", condition, id, failure)
		}
		if _, status := connection.Inspect(ctx, record.Name); status != port.NotFound {
			t.Errorf("%s created a resource without safe image evidence", condition)
		}
		connection.Close()
	}
}
