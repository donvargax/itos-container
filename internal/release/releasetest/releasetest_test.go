package releasetest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type fixtureFailure string

// fixtureTest retains testing.T's cleanup and environment handling, but gives
// fixtures a chosen root and records a fatal failure by stopping that call.
type fixtureTest struct {
	*testing.T
	root string
}

func (t fixtureTest) TempDir() string   { return t.root }
func (t fixtureTest) Fatal(args ...any) { panic(fixtureFailure(fmt.Sprint(args...))) }
func (t fixtureTest) Fatalf(format string, args ...any) {
	panic(fixtureFailure(fmt.Sprintf(format, args...)))
}

func wantFatal(t *testing.T, want string, run func()) {
	t.Helper()
	defer func() {
		failure, ok := recover().(fixtureFailure)
		if !ok || !strings.Contains(string(failure), want) {
			t.Fatalf("fixture failure = %q, want %q", failure, want)
		}
	}()
	run()
}

func TestExampleReportsFilesystemFailures(t *testing.T) {
	for _, tc := range []struct {
		name, blocked string
		directory     bool
	}{
		{"git config cannot be written", "gitconfig", true},
		{"repository path is a file", "repo", false},
		{"fixture file is a directory", "repo/go.mod", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, filepath.FromSlash(tc.blocked))
			if tc.directory {
				if err := os.MkdirAll(path, 0o750); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, []byte("occupied"), 0o600); err != nil {
				t.Fatal(err)
			}
			wantFatal(t, path, func() { Example(fixtureTest{t, root}, true) })
		})
	}
}

func TestExampleWithoutATagCreatesOnlyABuildCommit(t *testing.T) {
	repo := Example(t, false)
	cmd := exec.Command("git", "log", "-1", "--format=%s")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil || string(out) != "build: start\n" {
		t.Fatalf("initial commit = %q, %v", out, err)
	}
	cmd = exec.Command("git", "tag", "--list")
	cmd.Dir = repo
	out, err = cmd.Output()
	if err != nil || len(out) != 0 {
		t.Fatalf("untagged fixture tags = %q, %v", out, err)
	}
}

func TestChangeReportsReadAndReplacementFailures(t *testing.T) {
	root := t.TempDir()
	ft := fixtureTest{t, root}
	path := filepath.Join(root, "text")
	wantFatal(t, path, func() { Change(ft, path, "old", "new") })
	if err := os.WriteFile(path, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	wantFatal(t, `does not hold "old"`, func() { Change(ft, path, "old", "new") })
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "unchanged" {
		t.Fatalf("failed replacement changed file: %q, %v", data, err)
	}
}

func TestChangeReportsWriteFailures(t *testing.T) {
	root := t.TempDir()
	ft := fixtureTest{t, root}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	wantFatal(t, file, func() { Change(ft, filepath.Join(file, "child"), "", "new") })
	wantFatal(t, root, func() { Change(ft, root, "", "new") })
}

func TestChangeWritesWholeFilesAndReplacesOnlyTheFirstOccurrence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "text")
	Change(t, path, "", "old old")
	Change(t, path, "old", "new")
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "new old" {
		t.Fatalf("replacement = %q, %v", data, err)
	}
}

func TestGitReportsItsCommandAndFailureOutput(t *testing.T) {
	root := t.TempDir()
	wantFatal(t, "git T-8-invalid-subcommand:", func() {
		Git(fixtureTest{t, root}, root, "T-8-invalid-subcommand")
	})
}
