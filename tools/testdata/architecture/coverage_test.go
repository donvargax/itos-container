package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDomainCoverage(t *testing.T) {
	for _, mode := range []string{"full", "low", "empty", "zero"} {
		t.Run(mode, func(t *testing.T) {
			dir := fixture(t)
			write(t, dir, "tools/bin/domain-coverage", read(t, filepath.Join(root(t), "tools/bin/domain-coverage")))
			// Use the real discovery binary, not a copied package enumeration.
			checker := os.Getenv("ARCHITECTURE_CHECKER")
			if checker == "" {
				t.Fatal("infrastructure: no checker")
			}
			write(t, dir, "tools/bin/architecture-check", "#!/bin/sh\nexec '"+strings.ReplaceAll(checker, "'", "'\\''")+"' \"$@\"\n")
			if err := os.Chmod(filepath.Join(dir, "tools/bin/architecture-check"), 0700); err != nil {
				t.Fatal(err)
			}
			domains := []string{"internal/slices/count/domain", "internal/slices/fresh/domain/nested", "internal/engine/shared/domain", "internal/engine/shared/domain/nested"}
			if mode != "empty" {
				for _, pkg := range domains {
					write(t, dir, pkg+"/value.go", "package "+filepath.Base(pkg)+"\nfunc Value(b bool) int { if b { return 1 }; return 2 }\n")
					calls := "Value(true); Value(false)"
					if mode == "low" && pkg == domains[2] {
						calls = "Value(true)"
					}
					write(t, dir, pkg+"/value_test.go", "package "+filepath.Base(pkg)+"\nimport \"testing\"\nfunc TestValue(t *testing.T) { "+calls+" }\n")
				}
				for _, pkg := range []string{"internal/slices/count/port", "internal/slices/count/port/porttest", "internal/slices/count/disk", "internal/slices/count/command", "internal/engine/shared/disk", "internal/engine/shared/domain/testdata/ignored"} {
					write(t, dir, pkg+"/untested.go", "package "+filepath.Base(pkg)+"\nfunc Untested() int { return 1 }\n")
				}
				if mode == "zero" {
					write(t, dir, "internal/engine/empty/domain/value.go", "package domain\nconst Value=1\n")
				}
			}
			must(t, dir, "go", "test", "-run=^$", "./...")
			out, code := command(t, dir, "sh", "tools/bin/domain-coverage")
			wantCode := 0
			if mode == "low" || mode == "zero" {
				wantCode = 1
			}
			if mode == "empty" {
				wantCode = 2
			}
			if code != wantCode {
				t.Fatalf("coverage exit %d want %d\n%s", code, wantCode, out)
			}
			if mode == "empty" {
				if !strings.Contains(out, "no domain packages") {
					t.Fatal(out)
				}
				return
			}
			for _, pkg := range domains {
				figure := "100.0%"
				if mode == "low" && pkg == domains[2] {
					figure = "66.6%"
				}
				if !strings.Contains(out, "./"+pkg+" "+figure) {
					t.Errorf("missing package figure %s %s\n%s", pkg, figure, out)
				}
			}
			if mode == "low" && !strings.Contains(out, "under the floor of 80%") {
				t.Fatal(out)
			}
			if mode == "zero" && !strings.Contains(out, "./internal/engine/empty/domain has no statements to measure") {
				t.Fatal(out)
			}
		})
	}
}
