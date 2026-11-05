package porttest

import (
	"context"
	"fmt"
	"strings"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
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
	if status != port.Observed || observed.ID != id || observed.Name != record.Name || observed.ImageID != record.ImageID || observed.Token != record.NameToken() || observed.Project != record.Project || observed.Fingerprint != record.Fingerprint || !observed.StatePresent || observed.Status != "created" || observed.Running || observed.PID != 0 || observed.Started {
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
