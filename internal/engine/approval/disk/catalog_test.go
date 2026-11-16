package disk

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	f "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
)

func TestHomeCatalogContract(t *testing.T) {
	t.Run("memory", func(t *testing.T) { f.CatalogContract(t, &f.Catalog{}, "1000") })
	t.Run("disk", func(t *testing.T) {
		control, c, _, _, _ := controlFixture(t)
		f.CatalogContract(t, Catalog{Account: control.Account, Store: control.Store}, c.UID)
	})
}

func TestHomeCatalogRefusesUnsafeAccountAndEntries(t *testing.T) {
	for _, fault := range []string{"lookup", "relative", "root", "encoding", "absent", "symlink", "writable", "stat", "invalid entry", "unsafe catalog", "read directory", "invalid uid"} {
		t.Run(fault, func(t *testing.T) {
			control, c, _, home, _ := controlFixture(t)
			account := control.Account.(*f.Account)
			catalog := Catalog{Account: account, Store: control.Store}
			want := port.Failure("state.unavailable")
			switch fault {
			case "lookup":
				account.Failure = "state.unavailable"
			case "relative":
				account.Home = "relative"
			case "root":
				account.Home = "/"
			case "encoding":
				account.Home = home + "\xff"
			case "absent":
				account.Home = filepath.Join(home, "absent")
			case "symlink":
				account.Home = home + "-link"
				if err := os.Symlink(home, account.Home); err != nil {
					t.Fatal(err)
				}
				want = "state.path"
			case "writable":
				if err := os.Chmod(home, 0777); err != nil {
					t.Fatal(err)
				}
				want = "state.unsafe"
			case "stat":
				ops := catalog.Store.operations()
				ops.statFile = func(*os.File) (os.FileInfo, error) { return nil, errors.New("stat") }
				catalog.Store.ops = &ops
				want = "state.unsafe"
			case "invalid uid":
				c.UID = "invalid"
				want = "state.unsafe"
			default:
				root := filepath.Join(home, ".local/state/itos-container/homes")
				if err := os.MkdirAll(root, 0700); err != nil {
					t.Fatal(err)
				}
				switch fault {
				case "invalid entry":
					if err := os.Mkdir(filepath.Join(root, "foreign"), 0700); err != nil {
						t.Fatal(err)
					}
					want = "state.invalid"
				case "unsafe catalog":
					if err := os.Chmod(root, 0755); err != nil {
						t.Fatal(err)
					}
					want = "state.unsafe"
				case "read directory":
					ops := catalog.Store.operations()
					ops.openDir = func(r *os.Root, n string) (*os.File, error) {
						if r.Name() == root {
							return nil, errors.New("open")
						}
						return os.Open(filepath.Join(r.Name(), n))
					}
					catalog.Store.ops = &ops
				}
			}
			if _, failure := catalog.List(t.Context(), c.UID); failure != want {
				t.Fatalf("list %s want %s", failure, want)
			}
			if fault != "invalid entry" && fault != "read directory" {
				if d, failure := catalog.Open(t.Context(), c.UID, strings.Repeat("a", 64), false); failure != want || d != nil {
					t.Fatalf("open %v %s want %s", d, failure, want)
				}
			}
		})
	}
}

func TestOriginalHomeClaimReadOnlyStorage(t *testing.T) {
	control, c, _, _, _ := controlFixture(t)
	storage := Storage{Store: control.Store}
	ctx := context.Background()
	for _, collection := range []string{"intents", "snapshots"} {
		if data, failure := storage.ReadOriginal(ctx, c, collection, "1.json"); failure != "" || data != nil {
			t.Fatal(data, failure)
		}
		path := filepath.Join(control.Store.StateHome, "itos-container", collection, c.Project)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "1.json"), []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}
		if data, failure := storage.ReadOriginal(ctx, c, collection, "1.json"); failure != "" || string(data) != "original" {
			t.Fatal(string(data), failure)
		}
	}
	for _, kind := range []string{"collection", "project", "relative", "cancelled", "unsafe"} {
		t.Run(kind, func(t *testing.T) {
			s := storage
			candidate := c
			collection := "intents"
			ctx := ctx
			want := port.Failure("state.invalid")
			switch kind {
			case "collection":
				collection = "approvals"
			case "project":
				candidate.Project = "invalid"
			case "relative":
				s.Store.StateHome = "relative"
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				want = "state.unavailable"
			case "unsafe":
				if err := os.Chmod(filepath.Join(s.Store.StateHome, "itos-container/intents", c.Project), 0777); err != nil {
					t.Fatal(err)
				}
				want = "state.unsafe"
			}
			if data, failure := s.ReadOriginal(ctx, candidate, collection, "1.json"); failure != want || data != nil {
				t.Fatal(string(data), failure, want)
			}
		})
	}
}
