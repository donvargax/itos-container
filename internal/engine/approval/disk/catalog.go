package disk

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
)

// Catalog reuses protected directory traversal and durable compare/publication.
// The account lookup is independent of HOME and XDG_STATE_HOME.
type Catalog struct {
	Account port.Account
	Store   Store
}

func (s Catalog) root(ctx context.Context, uid string) (Store, string, port.Failure) {
	home, failure := s.Account.AccountHome()
	if failure != "" || !filepath.IsAbs(home) || home == "/" || !utf8.ValidString(home) {
		return Store{}, "", "state.unavailable"
	}
	store := s.Store
	store.Control = nil
	ops := store.operations()
	ops.lock = func(*os.File, bool) error { return nil }
	store.ops = &ops
	p, failure := store.pinPath(port.Candidate{UID: uid}, false, home, 0)
	if failure != "" {
		return Store{}, "", failure
	}
	if p == nil {
		return Store{}, "", "state.unavailable"
	}
	defer p.close()
	if !accountDirectory(p) {
		return Store{}, "", "state.unsafe"
	}
	ops.lock = func(f *os.File, _ bool) error { return lockContext(ctx, f) }
	return store, filepath.Join(home, ".local", "state", "itos-container", "homes"), ""
}
func (s Catalog) List(ctx context.Context, uid string) ([]string, port.Failure) {
	store, root, failure := s.root(ctx, uid)
	if failure != "" {
		return nil, failure
	}
	p, failure := store.pinPath(port.Candidate{UID: uid}, false, root, 2)
	if failure != "" || p == nil {
		return nil, failure
	}
	defer p.close()
	names, failure := pinnedNames(p)
	if failure != "" {
		return nil, failure
	}
	for _, name := range names {
		if !catalogKey(name) {
			return nil, "state.invalid"
		}
	}
	return names, ""
}
func catalogKey(key string) bool {
	return len(key) == 64 && strings.Trim(key, "0123456789abcdef") == ""
}
func (s Catalog) Open(ctx context.Context, uid, key string, create bool) (port.Directory, port.Failure) {
	if !catalogKey(key) {
		return nil, "state.invalid"
	}
	store, root, failure := s.root(ctx, uid)
	if failure != "" {
		return nil, failure
	}
	p, failure := store.pinPath(port.Candidate{UID: uid}, create, filepath.Join(root, key), 3)
	if failure != "" || p == nil {
		return nil, failure
	}
	return &directoryStore{pin: p, ctx: budget.Lifetime(ctx), exclusive: (*os.Root).Link}, ""
}
