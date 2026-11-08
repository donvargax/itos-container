// Adapted from donvargax/itos-template at 79301eba51b9768a27906c720e39c49b08491e67.
// Retargeted to itos-container; T-25 adds bootstrap and history regression checks.
package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/release/releasetest"
)

// The bump the commits since the last release ask for, the cases itos's
// tools/selftest/release-cut.ts proves over scratch histories.
func TestBumpOf(t *testing.T) {
	cases := []struct {
		name     string
		messages []string
		kind     string
	}{
		{"a feat", []string{"feat: a thing"}, "minor"},
		{"a fix", []string{"fix: a bug"}, "patch"},
		{"a scoped fix", []string{"fix(cli): a bug"}, "patch"},
		{"a feat and a fix", []string{"fix: a bug", "feat: a thing", "docs: say so"}, "minor"},
		{"a BREAKING-CHANGE: footer", []string{"feat: a thing", "fix: a bug\n\nWhy.\n\nBREAKING-CHANGE: a key goes"}, "major"},
		{"a BREAKING CHANGE: footer", []string{"fix: a bug\n\nBREAKING CHANGE: a key goes\nTask: T-1"}, "major"},
		{"a feat! header", []string{"feat!: a thing"}, "major"},
		{"a refactor! header", []string{"refactor(config)!: a key goes"}, "major"},
		{"docs, ci, test and build commits", []string{"docs: say so", "ci: run it", "test: prove it", "build: pin it"}, "none"},
		{"a BREAKING-CHANGE line outside the last paragraph", []string{"fix: a bug\n\nBREAKING-CHANGE: not a footer here\n\nTask: T-1"}, "patch"},
		{"no commit", nil, "none"},
	}
	for _, c := range cases {
		b := bumpOf(c.messages)
		if b.kind != c.kind || b.commits != len(c.messages) {
			t.Errorf("%s: bumpOf = %s over %d commit(s), want %s over %d", c.name, b.kind, b.commits, c.kind, len(c.messages))
		}
	}
}

// Exercise the entry point against complete histories. No fixture needs network,
// a Docker socket, or the project's hooks; the shared fixture pins Go locally.
func TestReleaseHistory(t *testing.T) {
	const first = "last=\nnext=0.1.0\nbump=minor\nrange=HEAD\n"
	const none = "last=\nnext=\nbump=none\nrange=HEAD\n"
	cases := []struct {
		name     string
		tagged   bool
		messages []string
		edits    map[string][2]string
		setup    func(*testing.T, string) string
		want     string
		code     int
		says     string
	}{
		{name: "first feat", messages: []string{"feat: start"}, want: first},
		{name: "first fix", messages: []string{"fix: repair"}, want: first},
		{name: "first breaking header", messages: []string{"refactor!: change"}, want: first},
		{name: "first breaking footer", messages: []string{"docs: change\n\nBREAKING CHANGE: migrate"}, want: first},
		{name: "untagged no release", messages: []string{"docs: explain", "test: check"}, want: none},
		{name: "range excludes released feat", tagged: true, messages: []string{"fix: repair"},
			want: "last=v0.1.0\nnext=0.1.1\nbump=patch\nrange=v0.1.0..HEAD\n"},
		{name: "feat precedes fix", tagged: true, messages: []string{"fix: repair", "feat: extend"},
			want: "last=v0.1.0\nnext=0.2.0\nbump=minor\nrange=v0.1.0..HEAD\n"},
		{name: "breaking header precedes feat", tagged: true, messages: []string{"refactor!: change", "feat: extend"},
			want: "last=v0.1.0\nnext=1.0.0\nbump=major\nrange=v0.1.0..HEAD\n"},
		{name: "breaking hyphen footer precedes feat", tagged: true, messages: []string{"feat: extend", "docs: explain\n\nBREAKING-CHANGE: migrate"},
			want: "last=v0.1.0\nnext=1.0.0\nbump=major\nrange=v0.1.0..HEAD\n"},
		{name: "breaking space footer precedes fix", tagged: true, messages: []string{"fix: repair\n\nBREAKING CHANGE: migrate"},
			want: "last=v0.1.0\nnext=1.0.0\nbump=major\nrange=v0.1.0..HEAD\n"},
		{name: "body is not a footer", tagged: true, messages: []string{"fix: repair\n\nBREAKING-CHANGE: quoted text\n\nTask: T-1"},
			want: "last=v0.1.0\nnext=0.1.1\nbump=patch\nrange=v0.1.0..HEAD\n"},
		{name: "no commits since tag", tagged: true,
			want: "last=v0.1.0\nnext=\nbump=none\nrange=v0.1.0..HEAD\n"},
		{name: "prose only", tagged: true, messages: []string{"docs: explain"},
			want: "last=v0.1.0\nnext=\nbump=none\nrange=v0.1.0..HEAD\n"},
		{name: "numeric reachable stable tag", tagged: true, setup: func(t *testing.T, repo string) string {
			releasetest.Git(t, repo, "tag", "v0.9.0")
			releasetest.Git(t, repo, "tag", "v0.10.0")
			releasetest.Git(t, repo, "tag", "v8.0.0-rc.1")
			releasetest.Git(t, repo, "tag", "unrelated")
			releasetest.Git(t, repo, "checkout", "--quiet", "-b", "other")
			releasetest.Git(t, repo, "commit", "--quiet", "--allow-empty", "-m", "feat: unreachable")
			releasetest.Git(t, repo, "tag", "v99.0.0")
			releasetest.Git(t, repo, "checkout", "--quiet", "main")
			return repo
		}, messages: []string{"fix: repair"},
			want: "last=v0.10.0\nnext=0.10.1\nbump=patch\nrange=v0.10.0..HEAD\n"},
		{name: "linked module patch", tagged: true,
			edits: map[string][2]string{"go.mod": {"example.com/lib v1.0.0", "example.com/lib v1.1.0"}},
			want:  "last=v0.1.0\nnext=0.1.1\nbump=patch\nrange=v0.1.0..HEAD\n", says: "example.com/lib"},
		{name: "toolchain patch", tagged: true,
			edits: map[string][2]string{"go.mod": {"toolchain go1.24.0", "toolchain go1.24.1"}},
			want:  "last=v0.1.0\nnext=0.1.1\nbump=patch\nrange=v0.1.0..HEAD\n", says: "the toolchain"},
		{name: "module major refusal", tagged: true, setup: func(t *testing.T, repo string) string {
			releasetest.Git(t, repo, "tag", "v1.0.0")
			return repo
		}, messages: []string{"feat!: incompatible"}, code: 1, says: "refusing v2.0.0"},
		{name: "bootstrap module major refusal", messages: []string{"fix: repair"},
			edits: map[string][2]string{"go.mod": {"module example.com/app", "module example.com/app/v2"}},
			code:  1, says: "refusing v0.1.0"},
		{name: "shallow refusal", tagged: true, setup: func(t *testing.T, repo string) string {
			clone := filepath.Join(t.TempDir(), "shallow")
			releasetest.Git(t, repo, "clone", "--quiet", "--depth", "1", "file://"+filepath.ToSlash(repo), clone)
			return clone
		}, code: 2, says: "shallow clone"},
		{name: "Git read error", setup: func(t *testing.T, _ string) string { return t.TempDir() }, code: 2, says: "git"},
		{name: "module read error", messages: []string{"feat: start"},
			edits: map[string][2]string{"go.mod": {"", "go 1.24\n"}}, code: 2, says: "no module line"},
		{name: "Go read error", tagged: true,
			edits: map[string][2]string{"cmd/itos-container/main.go": {"example.com/lib", "example.com/missing"}},
			code:  2, says: "cannot tell whether the binary"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := releasetest.Example(t, c.tagged)
			if c.setup != nil {
				repo = c.setup(t, repo)
			}
			for path, edit := range c.edits {
				releasetest.Change(t, filepath.Join(repo, path), edit[0], edit[1])
			}
			if len(c.edits) > 0 {
				releasetest.Commit(t, repo, "build: change inputs")
			}
			for _, message := range c.messages {
				releasetest.Git(t, repo, "commit", "--quiet", "--allow-empty", "-m", message)
			}
			t.Chdir(repo)
			before := historyState(t)
			var stdout, stderr bytes.Buffer
			code := run(nil, &stdout, &stderr)
			if code != c.code || stdout.String() != c.want || !strings.Contains(stderr.String(), c.says) {
				t.Errorf("run = exit %d, stdout %q, stderr %q; want exit %d, stdout %q, diagnostic %q",
					code, stdout.String(), stderr.String(), c.code, c.want, c.says)
			}
			if after := historyState(t); after != before {
				t.Errorf("version calculation changed repository state:\nbefore %s\nafter %s", before, after)
			}
		})
	}
}

func historyState(t *testing.T) string {
	t.Helper()
	var state strings.Builder
	for _, args := range [][]string{
		{"rev-parse", "HEAD"}, {"for-each-ref", "--format=%(refname) %(objectname)"},
		{"status", "--porcelain", "--untracked-files=all"}, {"diff", "--binary", "HEAD"},
	} {
		cmd := exec.Command("git", args...)
		out, err := cmd.CombinedOutput()
		if err != nil && !strings.Contains(strings.ToLower(string(out)), "not a git repository") {
			t.Fatalf("snapshot: %v: %s", err, out)
		}
		state.Write(out)
	}
	return state.String()
}

// The next version from the last release tag; every releasable first cut is 0.1.0.
func TestBumped(t *testing.T) {
	cases := []struct{ tag, kind, want string }{
		{"", "minor", "0.1.0"},
		{"", "patch", "0.1.0"},
		{"", "major", "0.1.0"},
		{"v1.2.3", "minor", "1.3.0"},
		{"v1.2.3", "patch", "1.2.4"},
		{"v1.2.3", "major", "2.0.0"},
		{"v1.9.9", "minor", "1.10.0"},
	}
	for _, c := range cases {
		if got := bumped(c.tag, c.kind); got != c.want {
			t.Errorf("bumped(%q, %s) = %s, want %s", c.tag, c.kind, got, c.want)
		}
	}
}

// A version whose major go.mod's module path does not match is refused: v0
// and v1 from a path with no /vN suffix, vN from v2 from one ending in /vN.
func TestMismatch(t *testing.T) {
	const module = "github.com/donvargax/itos-container"
	cases := []struct {
		next, module string
		refused      bool
	}{
		{"0.1.0", module, false},
		{"1.4.0", module, false},
		{"2.0.0", module, true},
		{"2.0.0", module + "/v2", false},
		{"3.0.0", module + "/v2", true},
		{"1.0.0", module + "/v2", true},
		{"5.0.0", "", false},
	}
	for _, c := range cases {
		problem := mismatch(c.next, c.module)
		if (problem != "") != c.refused {
			t.Errorf("mismatch(%s, %q) = %q, want refused %v", c.next, c.module, problem, c.refused)
		}
	}
	if problem := mismatch("2.0.0", module); !strings.Contains(problem, module+"/v2") {
		t.Errorf("the refusal does not name the path a v2 needs: %s", problem)
	}
}

// git log's records split into each commit's message, trimmed.
func TestMessages(t *testing.T) {
	log := "aaa\x1ffeat: a thing\n\nWhy.\n\x1e\nbbb\x1ffix: a bug\n\x1e\n"
	got := messages(log)
	if len(got) != 2 || got[0] != "feat: a thing\n\nWhy." || got[1] != "fix: a bug" {
		t.Errorf("messages = %q", got)
	}
}

// The rule of what the binary is built from (decision 23), over example
// repositories (decision 19): each starts from releasetest.Example's v0.1.0, commits one
// change, and runs release-version in it. A module the binary links, or the
// toolchain, moved is a patch naming it; whatever leaves the binary as it
// was is nothing; a feat beside a moved module is a minor, as before; and
// with no tag the rule does not run.
func TestBinaryMoved(t *testing.T) {
	cases := []struct {
		name     string
		message  string
		edits    map[string][2]string // file: old, new ("" old writes the file whole)
		untagged bool
		bump     string
		says     string
	}{
		{name: "a module the binary links moved",
			edits: map[string][2]string{"go.mod": {"example.com/lib v1.0.0", "example.com/lib v1.1.0"}},
			bump:  "patch", says: "but the binary is built from what moved: example.com/lib v1.0.0 => ./lib to v1.1.0 => ./lib: 0.1.1"},
		{name: "a module only tests import moved",
			edits: map[string][2]string{"go.mod": {"example.com/testonly v1.0.0", "example.com/testonly v1.1.0"}},
			bump:  "none", says: "the binary links the same modules with the same toolchain: nothing to release"},
		{name: "a tool of the tool block moved",
			edits: map[string][2]string{"go.mod": {"example.com/tool v1.0.0", "example.com/tool v1.1.0"}},
			bump:  "none", says: "nothing to release"},
		{name: "a required module the binary never links moved",
			edits: map[string][2]string{"go.mod": {"example.com/unlinked v1.0.0", "example.com/unlinked v1.1.0"}},
			bump:  "none", says: "nothing to release"},
		{name: "the toolchain line moved",
			edits: map[string][2]string{"go.mod": {"toolchain go1.24.0", "toolchain go1.24.1"}},
			bump:  "patch", says: "but the binary is built from what moved: the toolchain go1.24.0 to go1.24.1: 0.1.1"},
		{name: "the toolchain line went and the go line moved",
			edits: map[string][2]string{"go.mod": {"go 1.24\n\ntoolchain go1.24.0\n", "go 1.25\n"}},
			bump:  "patch", says: "the toolchain go1.24.0 to go1.25: 0.1.1"},
		{name: "go.sum alone moved",
			edits: map[string][2]string{"go.sum": {"", "example.com/other v1.0.0 h1:47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU=\n"}},
			bump:  "none", says: "nothing to release"},
		{name: "a workflow's action moved",
			edits: map[string][2]string{".github/workflows/ci.yml": {"actions/checkout@v4", "actions/checkout@v5"}},
			bump:  "none", says: "nothing to release"},
		{name: "a feat beside a moved module", message: "feat: a thing",
			edits: map[string][2]string{"go.mod": {"example.com/lib v1.0.0", "example.com/lib v1.1.0"}},
			bump:  "minor", says: "1 feat(s), no breaking change: 0.2.0"},
		{name: "a moved module with no release yet", untagged: true,
			edits: map[string][2]string{"go.mod": {"example.com/lib v1.0.0", "example.com/lib v1.1.0"}},
			bump:  "none", says: "since no release (0.0.0), none a feat, a fix or a breaking change: nothing to release"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := releasetest.Example(t, !c.untagged)
			for file, edit := range c.edits {
				releasetest.Change(t, filepath.Join(repo, file), edit[0], edit[1])
			}
			message := c.message
			if message == "" {
				message = "build: move it"
			}
			releasetest.Commit(t, repo, message)
			t.Chdir(repo)
			var stdout, stderr bytes.Buffer
			if code := run(nil, &stdout, &stderr); code != 0 {
				t.Fatalf("exit %d: %s", code, stderr.String())
			}
			if !strings.Contains(stdout.String(), "bump="+c.bump+"\n") {
				t.Errorf("stdout is not bump=%s:\n%s", c.bump, stdout.String())
			}
			if !strings.Contains(stderr.String(), c.says) {
				t.Errorf("stderr does not say %q:\n%s", c.says, stderr.String())
			}
		})
	}
}

// A shallow clone stops it before any rule: exit 2.
func TestShallow(t *testing.T) {
	repo := releasetest.Example(t, true)
	clone := filepath.Join(t.TempDir(), "clone")
	releasetest.Git(t, repo, "clone", "--quiet", "--depth", "1", "file://"+filepath.ToSlash(repo), clone)
	t.Chdir(clone)
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "shallow clone") {
		t.Errorf("exit %d, stdout %q, stderr %q; want 2, nothing and the shallow clone named", code, stdout.String(), stderr.String())
	}
}
