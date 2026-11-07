package features

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Compile trusted probes only; none of these programs executes on the host.
// A later fixture must not invoke the compiler again or reuse writable checkout
// bytes. The cache belongs to this suite's freshly built product binary.
func TestNativeProbeBuildReuse(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		build func(*world) error
	}{
		{"slice-10-probe", (*world).buildOfflineProbe},
		{"slice-11-probe", (*world).buildRecoveryProbe},
		{"slice-12-probe", (*world).buildLifecycleProbe},
		{"slice-14-probe", (*world).buildIsolationProbe},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			first, second := t.TempDir(), t.TempDir()
			w := &world{root: root, bin: filepath.Join(dir, "product"), dir: dir, doctor: &doctorWorld{checkout: first}}
			if err := tc.build(w); err != nil {
				t.Fatal(err)
			}
			original := filepath.Join(first, tc.name)
			want, err := os.ReadFile(original)
			if err != nil || len(want) == 0 {
				t.Fatal("missing compiled probe", err)
			}
			// Checkout files are mutable. Changing one must not poison the next.
			if err := os.WriteFile(original, []byte("changed by fixture"), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", t.TempDir()) // any redundant compilation now fails
			w.doctor.checkout = second
			if err := tc.build(w); err != nil {
				t.Fatalf("a fresh fixture redundantly compiled the same trusted probe: %v", err)
			}
			copied := filepath.Join(second, tc.name)
			got, err := os.ReadFile(copied)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatal("fresh fixture inherited modified probe bytes", err)
			}
			a, err := os.Stat(original)
			if err != nil {
				t.Fatal(err)
			}
			b, err := os.Stat(copied)
			if err != nil || os.SameFile(a, b) || b.Mode().Perm()&0100 == 0 {
				t.Fatal("probe copy is shared or not executable", err)
			}
			// A different product build cannot reuse a previous suite's cache.
			w.bin = filepath.Join(dir, "other-product")
			w.doctor.checkout = t.TempDir()
			if err := tc.build(w); err == nil {
				t.Fatal("another suite reused a stale probe despite its missing compiler")
			}
			if _, err := os.Stat(filepath.Join(w.doctor.checkout, tc.name)); !os.IsNotExist(err) {
				t.Fatal("failed compilation published a probe", err)
			}
		})
	}
}
