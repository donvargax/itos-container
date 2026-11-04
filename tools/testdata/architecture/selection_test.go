package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/tools/architecture"
)

func history(t *testing.T, dir string) string {
	t.Helper()
	tree := strings.TrimSpace(must(t, dir, "git", "write-tree"))
	args := []string{"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit-tree", tree, "-m", "fixture"}
	if out, code := command(t, dir, "git", "rev-parse", "--verify", "HEAD"); code == 0 {
		args = append(args, "-p", strings.TrimSpace(out))
	}
	head := strings.TrimSpace(must(t, dir, "git", args...))
	must(t, dir, "git", "update-ref", "HEAD", head)
	return head
}
func TestFixtureSelection(t *testing.T) {
	watched := []string{".golangci.yml", "tools/bin/architecture-check", "tools/architecture/check/main.go", "tools/testdata/architecture/cases.json", "tools/bin/domain-coverage", "tools/hooks/pre-commit", "tools/bin/pinned", "go.mod", "go.sum", "itos.yaml"}
	for _, path := range append(watched, "unrelated.go", "README.md") {
		t.Run(path, func(t *testing.T) {
			dir := t.TempDir()
			must(t, dir, "git", "init", "-q")
			write(t, dir, "base", "base")
			must(t, dir, "git", "add", "base")
			base := history(t, dir)
			want := architecture.Watched(path)
			if want != (path != "unrelated.go" && path != "README.md") {
				t.Fatal("watch list mismatch")
			}
			for _, change := range []string{"add", "modify", "rename-out", "rename-in", "delete"} {
				switch change {
				case "add", "modify":
					write(t, dir, path, change)
				case "rename-out":
					if err := os.Rename(filepath.Join(dir, path), filepath.Join(dir, "moved")); err != nil {
						t.Fatal(err)
					}
				case "rename-in":
					if err := os.Rename(filepath.Join(dir, "moved"), filepath.Join(dir, path)); err != nil {
						t.Fatal(err)
					}
				case "delete":
					if err := os.Remove(filepath.Join(dir, path)); err != nil {
						t.Fatal(err)
					}
				}
				paths := []string{path}
				if change == "rename-out" || change == "rename-in" {
					paths = append(paths, "moved")
				}
				must(t, dir, "git", append([]string{"add", "--"}, paths...)...)
				got, err := architecture.Selected(t.Context(), dir, true, "", "")
				if err != nil || got != want {
					t.Fatalf("staged %s: %t, %v want %t", change, got, err, want)
				}
				head := history(t, dir)
				got, err = architecture.Selected(t.Context(), dir, false, base, head)
				if err != nil || got != want {
					t.Fatalf("range %s: %t, %v want %t", change, got, err, want)
				}
				base = head
			}
		})
	}
}

func TestSelectionLaunchesOnlyWhenSelected(t *testing.T) {
	for _, path := range []string{"README.md", "unrelated.go", ".golangci.yml"} {
		t.Run(path, func(t *testing.T) {
			dir := fixture(t)
			must(t, dir, "git", "init", "-q")
			write(t, dir, "tools/testdata/architecture/launch_test.go", "package architecture_test\nimport \"testing\"\nfunc TestLaunch(t *testing.T){t.Log(\"fixture-matrix-launched\")}\n")
			for _, p := range []string{"go.mod", "go.sum", ".golangci.yml", "cmd/control/main.go", "tools/testdata/architecture/launch_test.go"} {
				must(t, dir, "git", "add", p)
			}
			base := history(t, dir)
			content := "changed"
			if path == "unrelated.go" {
				content = "package fixture\n"
			}
			write(t, dir, path, content)
			must(t, dir, "git", "add", path)
			out := must(t, dir, checker(t), "--fixtures-if-staged")
			want := path == ".golangci.yml"
			if strings.Contains(out, "fixture-matrix-launched") != want {
				t.Fatalf("staged selection output: %s", out)
			}
			head := history(t, dir)
			out = must(t, dir, checker(t), "--fixtures-if-changed", base, head)
			if strings.Contains(out, "fixture-matrix-launched") != want {
				t.Fatalf("range selection output: %s", out)
			}
		})
	}
}
