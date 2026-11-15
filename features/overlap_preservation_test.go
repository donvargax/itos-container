package features

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"testing"
)

// Drive the actual preservation collector across the worker's atomic rename,
// without starting a worker, approving state or connecting to an engine.
func TestOverlapProtectedMarkerRename(t *testing.T) {
	for _, cached := range []bool{false, true} {
		name := "after_listing"
		if cached {
			name = "cached_info"
		}
		t.Run(name, func(t *testing.T) {
			f := overlapPreservationFixture(t)
			before := overlapCapture(t, f)
			r := f.runs[1] // Also encountered through the ancestor checkout.
			published := filepath.Join(r.w.doctor.checkout, r.marker)
			staging := published + ".next"
			renamed := 0
			walk := recoveryChangingWalk(t, staging, cached, &renamed, func() error {
				return os.Rename(staging, published)
			})
			after, err := f.protectedWithWalk(walk)
			if renamed != 1 {
				t.Fatalf("rename count = %d, want 1", renamed)
			}
			if data, err := os.ReadFile(published); err != nil || string(data) != "2" {
				t.Fatalf("marker did not advance: %q, %v", data, err)
			}
			if err != nil {
				t.Fatal("capture across marker rename:", err)
			}
			if !maps.Equal(before, after) {
				t.Fatal("marker rename changed protected snapshot")
			}
		})
	}
	t.Run("no_marker_metadata_reads", func(t *testing.T) {
		f := overlapPreservationFixture(t)
		before := overlapCapture(t, f)
		markers := map[string]bool{}
		for _, r := range f.runs {
			path := filepath.Join(r.w.doctor.checkout, r.marker)
			markers[path], markers[path+".next"] = true, true
		}
		delivered, inspected := 0, 0
		walk := func(root string, visit fs.WalkDirFunc) error {
			return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
				if err == nil && markers[path] {
					delivered++
					original := entry
					entry = recoveryObservedEntry{DirEntry: original, info: func() (fs.FileInfo, error) {
						inspected++
						return original.Info()
					}}
				}
				return visit(path, entry, err)
			})
		}
		after, err := f.protectedWithWalk(walk)
		if err != nil || !maps.Equal(before, after) {
			t.Fatalf("capture changed: %v", err)
		}
		if delivered != 6 || inspected != 0 {
			t.Fatalf("marker entries delivered %d, inspected %d; want 6 and 0", delivered, inspected)
		}
	})
}

func TestOverlapProtectedBoundaries(t *testing.T) {
	for _, relative := range []string{
		"sentinel", "slice-16-marker-99", "slice-16-marker-99.next",
		"slice-16-marker-1.keep", "slice-16-marker-1.next.keep",
		"child/slice-16-marker-1", "child/slice-16-marker-1.next",
		"nested/slice-16-marker-2.next", "child/sentinel",
	} {
		t.Run(relative, func(t *testing.T) {
			f := overlapPreservationFixture(t)
			path := filepath.Join(f.worlds["ancestor"].doctor.checkout, filepath.FromSlash(relative))
			recoveryWrite(t, path, "retained")
			before := overlapCapture(t, f)
			if _, exists := before[path]; !exists {
				t.Fatal("neighbour missing from snapshot:", path)
			}
			recoveryWrite(t, path, "changed")
			after := overlapCapture(t, f)
			if maps.Equal(before, after) || before[path] == after[path] {
				t.Fatal("neighbour change was not detected:", path)
			}
		})
	}
	for _, cached := range []bool{false, true} {
		name := "missing_metadata"
		op := "lstat"
		if cached {
			name, op = "missing_contents", "open"
		}
		t.Run(name, func(t *testing.T) {
			f := overlapPreservationFixture(t)
			path := filepath.Join(f.worlds["ancestor"].doctor.checkout, "unrelated.next")
			recoveryWrite(t, path, "retained")
			removed := 0
			walk := recoveryChangingWalk(t, path, cached, &removed, func() error { return os.Remove(path) })
			_, err := f.protectedWithWalk(walk)
			if removed != 1 {
				t.Fatalf("removal count = %d, want 1", removed)
			}
			recoveryRequireMissing(t, err, path, op)
		})
	}
}

func overlapPreservationFixture(t *testing.T) *overlapFixture {
	t.Helper()
	base := t.TempDir()
	f := &overlapFixture{worlds: map[string]*world{}}
	for i, name := range []string{"ancestor", "descendant"} {
		checkout := filepath.Join(base, "checkout")
		if i == 1 {
			checkout = filepath.Join(checkout, "child")
		}
		w := &world{
			doctor:   &doctorWorld{checkout: checkout, env: map[string]string{"XDG_STATE_HOME": filepath.Join(base, "state")}},
			approval: &approvalWorld{project: name, record: filepath.Join(base, "approvals", name)},
		}
		f.worlds[name] = w
		marker := "slice-16-marker-1"
		if i == 1 {
			marker = "slice-16-marker-2"
		}
		f.runs = append(f.runs, &overlapRun{w: w, marker: marker})
		for path, data := range map[string]string{
			filepath.Join(checkout, "sentinel"): "retained edit",
			filepath.Join(checkout, marker):     "1", filepath.Join(checkout, marker+".next"): "2",
			w.approval.record: "retained approval", w.sessionSnapshotPath(): "retained snapshot",
		} {
			recoveryWrite(t, path, data)
		}
	}
	return f
}

func overlapCapture(t *testing.T, f *overlapFixture) map[string]string {
	t.Helper()
	got, err := f.protected()
	if err != nil {
		t.Fatal("capture protected overlap paths:", err)
	}
	return got
}
