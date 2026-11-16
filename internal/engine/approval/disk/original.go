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
// checkout. It creates nothing and exposes no publication capability.
func (s Storage) ReadOriginal(ctx context.Context, c port.Candidate, collection, name string) ([]byte, port.Failure) {
	if collection != "intents" && collection != "snapshots" || len(c.Project) != 64 || strings.Trim(c.Project, "0123456789abcdef") != "" || !filepath.IsAbs(s.Store.StateHome) {
		return nil, "state.invalid"
	}
	if ctx.Err() != nil {
		return nil, "state.unavailable"
	}
	store := s.Store
	ops := store.operations()
	ops.lock = func(f *os.File, _ bool) error { return lockContext(ctx, f) }
	store.ops = &ops
	p, f := store.pinPath(c, false, filepath.Join(store.StateHome, "itos-container", collection, c.Project), 3)
	if f != "" || p == nil {
		return nil, f
	}
	d := directoryStore{pin: p, ctx: budget.Lifetime(ctx)}
	defer d.Close()
	return d.Read(name)
}
