package disk

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
)

// ReadOriginal pins a recorded managed location without consulting the current
// checkout. It creates nothing and exposes no publication capability, and it
// waits for no session operation lock: management reads claim evidence while
// holding the home scope lock, whose declared order is session operation first,
// so taking the intents entry lock here would deadlock with any launch, stop or
// status that holds the entry and prepares or reconciles the home. The pinned
// directory and record identity revalidation before and after the read, the
// same protection unlocked Locations discovery relies on, keep the evidence
// safe without the lock.
func (s Storage) ReadOriginal(ctx context.Context, c port.Candidate, collection, name string) ([]byte, port.Failure) {
	if collection != "intents" && collection != "snapshots" || len(c.Project) != 64 || strings.Trim(c.Project, "0123456789abcdef") != "" || !filepath.IsAbs(s.Store.StateHome) {
		return nil, "state.invalid"
	}
	if ctx.Err() != nil {
		return nil, "state.unavailable"
	}
	store := s.Store
	ops := store.operations()
	ops.lock = func(*os.File, bool) error { return nil }
	store.ops = &ops
	p, f := store.pinPath(c, false, filepath.Join(store.StateHome, "itos-container", collection, c.Project), 3)
	if f != "" || p == nil {
		return nil, f
	}
	d := directoryStore{pin: p, ctx: budget.Lifetime(ctx)}
	defer d.Close()
	return d.Read(name)
}
