package disk

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// A directory that a creating pin has just made, or found a peer making, can be
// removed before the pin looks at it again: by the peer that lost a race, or by a
// same-user removal of the tree while the first approval runs. The pin has then
// neither a directory to hold nor a reason to report, and callers dereference
// what it returns whenever no failure comes with it. Creation must refuse.
func TestCreatingPinRefusesDirectoryThatVanishesBeforeItsLookup(t *testing.T) {
	for _, kind := range []string{"made then removed", "peer made then removed"} {
		t.Run(kind, func(t *testing.T) {
			s, c, _ := diskFixture(t)
			path := filepath.Join(s.StateHome, "itos-container", "coordination")
			ops := s.operations()
			mkdir := ops.mkdir
			ops.mkdir = func(root *os.Root, name string, mode os.FileMode) error {
				if filepath.Join(root.Name(), name) != path {
					return mkdir(root, name, mode)
				}
				if kind == "peer made then removed" {
					return os.ErrExist
				}
				if err := mkdir(root, name, mode); err != nil {
					return err
				}
				return os.Remove(path)
			}
			s.ops = &ops
			p, failure := s.pinPath(c, true, path, 2)
			if p != nil {
				p.close()
			}
			if p != nil || failure != "state.path" {
				t.Fatalf("pin = %v, %q; want no pin and state.path", p, failure)
			}
			// The lookup that finds nothing is still how a pin that creates nothing
			// reports an absent directory.
			s.ops = nil
			if p, failure = s.pinPath(c, false, path, 2); p != nil || failure != "" {
				t.Fatalf("absent directory: pin = %v, %q; want none and no failure", p, failure)
			}
		})
	}
}

// The first approval of an account reaches the same lookup twice: for the
// coordination catalog, and for the selected state tree it then enrolls. Neither
// may leave the transaction with a nil directory.
func TestControlBeginRefusesCreatedDirectoryThatVanishes(t *testing.T) {
	for _, kind := range []string{"catalog", "selected tree"} {
		t.Run(kind, func(t *testing.T) {
			control, c, _, home, selected := controlFixture(t)
			path := guardCatalog(home)
			if kind == "selected tree" {
				path = selected
			}
			ops := control.Store.operations()
			mkdir := ops.mkdir
			removed := false
			ops.mkdir = func(root *os.Root, name string, mode os.FileMode) error {
				err := mkdir(root, name, mode)
				if err == nil && filepath.Join(root.Name(), name) == path {
					removed = true
					return os.Remove(path)
				}
				return err
			}
			control.Store.ops = &ops
			var tx port.ControlTransaction
			var failure port.Failure
			func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						t.Fatalf("Begin panicked: %v", recovered)
					}
				}()
				tx, failure = control.Begin(t.Context(), c, true)
			}()
			if tx != nil {
				tx.Close()
			}
			if !removed {
				t.Fatal("the fixture did not reach the creation of " + path)
			}
			if tx != nil || failure != "state.path" {
				t.Fatalf("Begin = %v, %q; want no transaction and state.path", tx, failure)
			}
		})
	}
}
