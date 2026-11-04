package architecture_test

import (
	"os"
	"strings"
	"testing"
)

func checker(t *testing.T) string {
	t.Helper()
	path := os.Getenv("ARCHITECTURE_CHECKER")
	if path == "" {
		t.Fatal("infrastructure: no checker")
	}
	return path
}
func TestInspectionErrors(t *testing.T) {
	for _, name := range []string{"config", "empty-rules", "empty-rule", "load", "empty-targets", "no-main", "usage", "range", "missing-range"} {
		t.Run(name, func(t *testing.T) {
			dir := fixture(t)
			flags := []string{"--siblings", "--outside", "--linked", "--toplevel"}
			switch name {
			case "config":
				write(t, dir, ".golangci.yml", "[broken")
			case "empty-rules":
				write(t, dir, ".golangci.yml", "version: '2'\nlinters: {}\n")
			case "empty-rule":
				write(t, dir, ".golangci.yml", "version: '2'\nlinters:\n  settings:\n    depguard:\n      rules:\n        empty: {}\n")
			case "load":
				write(t, dir, "cmd/control/main.go", "package main\nimport _ \"does.not.exist/missing\"\nfunc main(){}\n")
			case "empty-targets":
				if err := os.RemoveAll(dir + "/cmd"); err != nil {
					t.Fatal(err)
				}
			case "no-main":
				write(t, dir, "cmd/control/main.go", "package control\n")
			case "usage":
				flags = []string{"--nonsense"}
			case "range":
				flags = []string{"--fixtures-if-changed", "unknown", "HEAD"}
			case "missing-range":
				flags = []string{"--fixtures-if-changed"}
			}
			out, code := command(t, dir, checker(t), flags...)
			if code != 2 || !strings.Contains(out, "inspection:") {
				t.Fatalf("want inspection exit 2, got %d\n%s", code, out)
			}
		})
	}
}

// --linked stands alone: a direct-import check cannot mask a transitive leak.
func TestLinkedSupport(t *testing.T) {
	for _, support := range []string{"internal/slices/count/port/porttest", "internal/engine/shared/port/porttest", "internal/release/releasetest", "internal/slices/count/fakes"} {
		t.Run(support, func(t *testing.T) {
			dir := fixture(t)
			write(t, dir, support+"/base.go", source(support+"/base.go", ""))
			write(t, dir, "internal/slices/count/disk/base.go", source("internal/slices/count/disk/base.go", module+"/"+support))
			write(t, dir, "cmd/control/main.go", "package main\nimport _ \""+module+"/internal/slices/count/disk\"\nfunc main(){}\n")
			must(t, dir, "go", "test", "-run=^$", "./...")
			out, code := command(t, dir, checker(t), "--linked")
			if code != 1 || !strings.Contains(out, "linked: "+module+"/cmd/control links "+module+"/"+support) {
				t.Fatalf("expected transitive linked refusal, exit %d\n%s", code, out)
			}
		})
	}
	t.Run("test-only-support", func(t *testing.T) {
		dir := fixture(t)
		path := "internal/slices/count/port/porttest"
		write(t, dir, path+"/base.go", source(path+"/base.go", ""))
		write(t, dir, "cmd/control/main_test.go", "package main\nimport _ \""+module+"/"+path+"\"\n")
		must(t, dir, "go", "test", "-run=^$", "./...")
		must(t, dir, checker(t), "--linked")
	})
}
