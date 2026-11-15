package porttest

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func CredentialRecoveryStoreContract(t Reporter, create func(port.Binding, port.Record) port.Store) {
	t.Helper()
	b, _, _ := Fixture()
	b.Credentials = port.CredentialKinds{Claude: true, GitHub: true}
	r := RunIntent(b)
	r.Stage = "reserved"
	store := create(b, r)
	ctx := context.Background()
	ref := port.Reference(r.Project, r.NameToken())
	steps := []port.Record{}
	r.Schema = 4
	r.Credentials = CredentialIntent(b.Credentials).Credentials
	steps = append(steps, r)
	r.Credentials.Device = "1"
	r.Credentials.Inode = "2"
	r.Credentials.Stage = "owned"
	steps = append(steps, r)
	r.Credentials.Stage = "absent"
	steps = append(steps, r)
	for _, next := range steps {
		i, fail := store.OpenCleanup(ctx, ref)
		if fail != "" {
			t.Errorf("credential recovery reopen refused: %s", fail)
			return
		}
		fail = i.Save(next)
		i.Close()
		if fail != "" {
			t.Errorf("credential recovery publication refused: %s", fail)
			return
		}
		i, fail = store.OpenCleanup(ctx, ref)
		if fail != "" {
			t.Errorf("credential recovery reconstruction refused: %s", fail)
			return
		}
		if i.Record() != next || i.Validate() != "" {
			t.Errorf("reopened credential authority changed")
		}
		i.Close()
	}
}
