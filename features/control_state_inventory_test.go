package features

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Earlier session scenarios start from current approved state. Seed the new
// catalog independently with that state; CONTROL scenarios explicitly cover
// legacy enrollment, absent catalogs and bootstrap instead.
func (w *world) seedControlLocations() error {
	if w.control != nil || w.approval == nil || w.doctor == nil {
		return nil
	}
	if _, err := os.Lstat(w.approval.record); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	home := w.doctor.env["SLICE_21_ACCOUNT_HOME"]
	if home == "" {
		return errors.New("fixture lacks isolated account home")
	}
	selected := filepath.Dir(filepath.Dir(filepath.Dir(w.approval.record)))
	fixed := filepath.Join(home, ".local/state/itos-container")
	catalog := filepath.Join(fixed, "coordination")
	if err := os.MkdirAll(catalog, 0700); err != nil {
		return err
	}
	for _, path := range []string{fixed, selected} {
		name := approvalHash(path) + ".json"
		if path == fixed {
			name = "self.json"
		}
		filename := filepath.Join(catalog, name)
		if _, err := os.Lstat(filename); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		binding, err := controlBinding(path, home, true)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filename, approvalJSON(binding), 0600); err != nil {
			return err
		}
	}
	return nil
}

// Successful first approval may add only the fixed chain and independently
// verified self/selected bindings. Never exempt arbitrary coordination entries.
func (w *world) approvalControlPaths() (map[string]bool, error) {
	home := w.doctor.env["SLICE_21_ACCOUNT_HOME"]
	fixed := filepath.Join(home, ".local/state/itos-container")
	catalog := filepath.Join(fixed, "coordination")
	selected := filepath.Dir(filepath.Dir(filepath.Dir(w.approval.record)))
	allowed := map[string]bool{}
	for _, path := range []string{filepath.Join(home, ".local"), filepath.Join(home, ".local/state"), fixed, catalog} {
		allowed[path] = true
	}
	for _, path := range []string{fixed, selected} {
		name := approvalHash(path) + ".json"
		if path == fixed {
			name = "self.json"
		}
		name = filepath.Join(catalog, name)
		want, err := controlBinding(path, home, true)
		if err != nil {
			return nil, err
		}
		record, err := sessionReadRecord(name)
		if err != nil {
			return nil, err
		}
		if !recoveryJSONEqual(record.Bytes, want) {
			return nil, fmt.Errorf("unbound control location %s", name)
		}
		allowed[name] = true
	}
	for path := range allowed {
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		mode := os.FileMode(0600)
		if info.IsDir() {
			mode = 0700
		}
		if info.Mode().Perm() != mode || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("unsafe control publication")
		}
	}
	entries, err := os.ReadDir(catalog)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !allowed[filepath.Join(catalog, entry.Name())] {
			return nil, errors.New("unrelated coordination publication: " + entry.Name())
		}
	}
	// Directories above the fixed tree keep their identities; doctorTree observes
	// mode/owner/content, so only the exact new fixed-chain entries are allowed.
	return allowed, nil
}
