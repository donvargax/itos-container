// Package releasetest makes the example repository the release tests run
// in, so a cut and its notes share one example rather than each keeping a
// copy of it.
package releasetest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Example makes an example repository: a module whose binary,
// ./cmd/itos-container, links example.com/lib; example.com/testonly only its
// tests import, example.com/unlinked it requires and never imports, and
// example.com/tool its tool block names, each a local module replaced in
// go.mod, so go list resolves every one with no network (GOPROXY=off); a
// workflow; and one commit, a feat tagged v0.1.0 when tagged, else a build
// commit, so no release has been cut and none is due. The go line
// and toolchain are below any Go that runs the test, and GOTOOLCHAIN=local,
// so no toolchain is fetched either.
func Example(t *testing.T, tagged bool) string {
	t.Helper()
	home := t.TempDir()
	config := filepath.Join(home, "gitconfig")
	if err := os.WriteFile(config, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{
		"GIT_CONFIG_GLOBAL": config, "GIT_CONFIG_NOSYSTEM": "1",
		"GIT_AUTHOR_NAME": "Example", "GIT_AUTHOR_EMAIL": "example@example.com",
		"GIT_COMMITTER_NAME": "Example", "GIT_COMMITTER_EMAIL": "example@example.com",
		"GOPROXY": "off", "GOTOOLCHAIN": "local", "GOFLAGS": "", "GOWORK": "off",
	} {
		t.Setenv(k, v)
	}
	repo := filepath.Join(home, "repo")
	files := map[string]string{
		"go.mod": `module example.com/app

go 1.24

toolchain go1.24.0

require (
	example.com/lib v1.0.0
	example.com/testonly v1.0.0
	example.com/tool v1.0.0
)

require example.com/unlinked v1.0.0 // indirect

replace (
	example.com/lib => ./lib
	example.com/testonly => ./testonly
	example.com/tool => ./tool
	example.com/unlinked => ./unlinked
)

tool example.com/tool/cmd/tool
`,
		"cmd/itos-container/main.go":      "package main\n\nimport \"example.com/lib\"\n\nfunc main() { println(lib.Name) }\n",
		"cmd/itos-container/main_test.go": "package main\n\nimport (\n\t\"testing\"\n\n\t\"example.com/testonly\"\n)\n\nfunc TestName(t *testing.T) { _ = testonly.Name }\n",
		"lib/go.mod":                      "module example.com/lib\n\ngo 1.24\n",
		"lib/lib.go":                      "package lib\n\nconst Name = \"lib\"\n",
		"testonly/go.mod":                 "module example.com/testonly\n\ngo 1.24\n",
		"testonly/testonly.go":            "package testonly\n\nconst Name = \"testonly\"\n",
		"unlinked/go.mod":                 "module example.com/unlinked\n\ngo 1.24\n",
		"unlinked/unlinked.go":            "package unlinked\n\nconst Name = \"unlinked\"\n",
		"tool/go.mod":                     "module example.com/tool\n\ngo 1.24\n",
		"tool/cmd/tool/main.go":           "package main\n\nfunc main() {}\n",
		".github/workflows/ci.yml":        "on: push\njobs:\n  ci:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n",
	}
	for name, text := range files {
		path := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	Git(t, repo, "init", "--quiet", "--initial-branch=main")
	Git(t, repo, "add", "--all")
	if tagged {
		Git(t, repo, "commit", "--quiet", "-m", "feat: start")
		Git(t, repo, "tag", "v0.1.0")
	} else {
		Git(t, repo, "commit", "--quiet", "-m", "build: start")
	}
	return repo
}

// Change replaces old with new in the file, or writes new whole when old is
// "", failing the test when old is not there.
func Change(t *testing.T, path, old, new string) {
	t.Helper()
	text := ""
	if old != "" {
		b, err := os.ReadFile(path) //nolint:gosec // the file is this test's own, under its temporary folder
		if err != nil {
			t.Fatal(err)
		}
		if text = string(b); !strings.Contains(text, old) {
			t.Fatalf("%s does not hold %q", path, old)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // the path is this test's own file under its temporary repository
	if err := os.WriteFile(path, []byte(strings.Replace(text, old, new, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
}

func Git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:gosec // this test names every argument
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// Commit stages every change in repo and commits it with message.
func Commit(t *testing.T, repo, message string) {
	t.Helper()
	Git(t, repo, "add", "--all")
	Git(t, repo, "commit", "--quiet", "-m", message)
}
