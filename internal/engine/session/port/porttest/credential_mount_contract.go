package porttest

import (
	"bytes"
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

type CredentialEngineFixture struct {
	Engine   port.RunEngine
	Binding  port.Binding
	Record   port.Record
	Mounts   func() []port.CredentialMount
	Traffic  func() [][]byte
	Unlink   func()
	Acquired func() []byte
	Finish   func()
}

func CredentialMountContract(t Reporter, create func() CredentialEngineFixture) {
	t.Helper()
	f := create()
	ctx := context.Background()
	c, fail := f.Engine.OpenRun(ctx, f.Binding)
	if fail != "" {
		t.Errorf("engine setup refused: %s", fail)
		return
	}
	defer c.Close()
	argv := []string{"literal", "a b", ";$x"}
	id, fail := c.CreateRun(ctx, f.Record, argv)
	if fail != "" {
		t.Errorf("exact credential mount positive control refused: %s", fail)
		return
	}
	expected := f.Record.Credentials.Mounts()
	got := f.Mounts()
	if len(got) != len(expected) {
		t.Errorf("create did not request every individual credential bind")
		return
	}
	for i, m := range expected {
		if got[i] != m {
			t.Errorf("create changed credential mount %d", i)
		}
	}
	if fail := c.VerifyRun(ctx, id, argv); fail != "" {
		t.Errorf("exact credential inspect refused: %s", fail)
	}
	observation, status := c.Inspect(ctx, id)
	if status != port.Observed || !observation.AnchorExact || observation.AnchorName != f.Record.Anchor.Name {
		t.Errorf("credential binds lost recovery anchor ownership")
	}
}
func CredentialUncertainStartContract(t Reporter, create func() CredentialEngineFixture) {
	t.Helper()
	f := create()
	ctx := context.Background()
	c, fail := f.Engine.OpenRun(ctx, f.Binding)
	if fail != "" {
		t.Errorf("engine setup refused")
		return
	}
	defer c.Close()
	id, fail := c.CreateRun(ctx, f.Record, []string{"probe"})
	if fail != "" {
		t.Errorf("uncertain-start create refused: %s", fail)
		return
	}
	if fail = c.Start(ctx, id); fail == "" {
		t.Errorf("lost acknowledgement reported success")
	}
	before := append([]byte(nil), f.Acquired()...)
	if len(before) == 0 {
		t.Errorf("runtime acquired no credential reference")
	}
	f.Unlink()
	f.Finish()
	if !bytes.Equal(f.Acquired(), before) || len(f.Acquired()) == 0 {
		t.Errorf("pathname unlink destroyed the acquired credential reference")
	}
	o, status := c.Inspect(ctx, id)
	if status != port.Observed || o.ID != id || !o.Started {
		t.Errorf("original full ID did not reconcile delayed start")
	}
	c.RemoveRun(ctx, id)
	if len(f.Acquired()) != 0 {
		t.Errorf("confirmed worker removal retained acquired credential bytes")
	}
}
