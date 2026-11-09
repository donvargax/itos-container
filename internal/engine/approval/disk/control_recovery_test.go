package disk

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
)

func TestControlRetryCompletesPendingLocation(t *testing.T) {
	control, c, _, home, selected := controlFixture(t)
	ops := control.Store.operations()
	mkdir := ops.mkdir
	ops.mkdir = func(root *os.Root, name string, mode os.FileMode) error {
		if filepath.Join(root.Name(), name) == selected {
			return os.ErrPermission
		}
		return mkdir(root, name, mode)
	}
	control.Store.ops = &ops
	tx, failure := control.Begin(t.Context(), c, true)
	if tx != nil {
		tx.Close()
		t.Fatal("interrupted enrollment returned publication authority")
	}
	if failure != "state.write" {
		t.Fatalf("initial enrollment: %s, want state.write", failure)
	}
	path := filepath.Join(home, ".local/state/itos-container/coordination", locationName(selected))
	readBinding := func() locationBinding {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var binding locationBinding
		if err := json.Unmarshal(data, &binding); err != nil {
			t.Fatal(err)
		}
		return binding
	}
	pending := readBinding()
	if pending.Complete || pending.Path != selected || len(pending.Ancestors) == 0 {
		t.Fatal("interruption did not retain the pending location claim")
	}
	if _, err := os.Stat(selected); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("selected tree exists despite failed mkdir: %v", err)
	}
	control.Store.ops = nil
	other := control
	other.Store.StateHome = filepath.Join(filepath.Dir(home), "other")
	porttest.ControlCandidateContract(t, other, c, selected, func() string {
		return controlSnapshot(t, filepath.Dir(home))
	})
	porttest.ControlCollectionContract(t, selected, c, func(path string) port.Control {
		next := control
		next.Store.StateHome = filepath.Dir(path)
		return next
	}, func() string { return controlSnapshot(t, filepath.Dir(home)) })
	tx, failure = control.Begin(t.Context(), c, true)
	if failure != "" {
		t.Fatal("retry", failure)
	}
	tx.Close()
	complete := readBinding()
	if !complete.Complete || complete.Path != pending.Path || complete.UID != pending.UID || len(complete.Ancestors) <= len(pending.Ancestors) {
		t.Fatal("successful retry left the selected location incomplete")
	}
	if !reflect.DeepEqual(complete.Ancestors[:len(pending.Ancestors)], pending.Ancestors) {
		t.Fatal("retry changed the pending claim's bound ancestors")
	}
	tx, failure = control.Begin(t.Context(), c, false)
	if failure != "" {
		t.Fatal("completed location could not be inspected", failure)
	}
	tx.Close()
}

func TestControlRefusesPendingBindingWithoutAncestors(t *testing.T) {
	for _, encoding := range []string{"null", "empty array"} {
		for _, mode := range []string{"inspect", "publish"} {
			t.Run(encoding+"/"+mode, func(t *testing.T) {
				control, c, home, selected := pending(t)
				path := filepath.Join(guardCatalog(home), locationName(selected))
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var binding locationBinding
				if err := json.Unmarshal(data, &binding); err != nil {
					t.Fatal(err)
				}
				if binding.Complete || len(binding.Ancestors) == 0 {
					t.Fatal("fixture did not retain a pending claim with pinned ancestors")
				}
				// A completed binding also has its full ancestry length checked.
				// A pending claim must reject missing pins before a retry can
				// silently bind it to whichever ancestors now occupy the path.
				binding.Ancestors = nil
				if encoding == "empty array" {
					binding.Ancestors = []locationIdentity{}
				}
				data, err = json.Marshal(binding)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
				before := controlSnapshot(t, filepath.Dir(home))
				tx, failure := control.Begin(t.Context(), c, mode == "publish")
				if tx != nil {
					tx.Close()
					t.Fatal("pending claim without ancestors returned transaction authority")
				}
				if failure != "state.invalid" {
					t.Fatalf("Begin = %q, want state.invalid", failure)
				}
				if after := controlSnapshot(t, filepath.Dir(home)); after != before {
					t.Fatal("refusal changed the pending claim or created selected state")
				}
			})
		}
	}
}

func TestControlRetryRefusesSelectedBindingSyncFailure(t *testing.T) {
	control, c, _, home, selected := controlFixture(t)
	tx, failure := control.Begin(t.Context(), c, true)
	if failure != "" {
		t.Fatal(failure)
	}
	tx.Close()
	before := controlSnapshot(t, filepath.Dir(home))
	ops := control.Store.operations()
	syncFile := ops.syncFile
	ops.syncFile = func(file *os.File) error {
		if filepath.Base(file.Name()) == locationName(selected) {
			return os.ErrPermission
		}
		return syncFile(file)
	}
	control.Store.ops = &ops
	tx, failure = control.Begin(t.Context(), c, true)
	if tx != nil {
		tx.Close()
		t.Fatal("failed location resync returned publication authority")
	}
	if failure != "state.write" {
		t.Fatalf("retry: %s, want state.write", failure)
	}
	if after := controlSnapshot(t, filepath.Dir(home)); after != before {
		t.Fatal("failed resync changed retained control state")
	}
}

func TestAdmissionRefusesExposedControlState(t *testing.T) {
	control, c, reservations, home, selected := controlFixture(t)
	tx, failure := control.Begin(t.Context(), c, true)
	if failure != "" {
		t.Fatal(failure)
	}
	tx.Close()
	reservations.RootsByLocation[selected] = []string{filepath.Dir(selected)}
	before := controlSnapshot(t, filepath.Dir(home))
	store := control.Store
	store.Control = control
	directory, failure := (Registry{Store: store}).Admission(t.Context(), c)
	if directory != nil {
		directory.Close()
		t.Fatal("exposed control state returned an admission directory")
	}
	if failure != "state.exposed" {
		t.Fatalf("admission: %s, want state.exposed", failure)
	}
	if after := controlSnapshot(t, filepath.Dir(home)); after != before {
		t.Fatal("refused admission wrote state")
	}
}

func TestControlProspectiveRequiresAnExistingNonRootAncestor(t *testing.T) {
	store, c, _ := diskFixture(t)
	// A unique absent child of / has no pinnable ancestor other than / itself.
	path := "/" + strings.ReplaceAll(strings.TrimPrefix(store.StateHome, "/"), "/", "-") + "-absent"
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fixture path is not absent: %v", err)
	}
	p, failure := store.prospective(c, path, 1)
	if p != nil {
		p.close()
		t.Fatal("missing top-level location acquired a pin")
	}
	if failure != "state.unavailable" {
		t.Fatalf("prospective location: %s, want state.unavailable", failure)
	}
}

func TestControlInspectionRefusesLostCatalog(t *testing.T) {
	control, c, reservations, _, _ := controlFixture(t)
	tx, failure := control.Begin(t.Context(), c, true)
	if failure != "" {
		t.Fatal(failure)
	}
	defer tx.Close()
	transaction := tx.(*controlTransaction)
	path := transaction.catalog.path
	if err := os.Rename(path, path+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if failure := transaction.inspect(t.Context(), reservations); failure != "state.path" {
		t.Fatalf("catalog inspection: %s, want state.path", failure)
	}
}
