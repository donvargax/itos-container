// Adapted from donvargax/itos-template at 79301eba51b9768a27906c720e39c49b08491e67.
// Retargeted to itos-container; T-25 adds bootstrap and history regression checks.
package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/release/releasetest"
)

// What a commit marked as breaking asks: its footer's text, wrapped lines
// kept, or its header for a ! alone; a footer outside the last paragraph is
// not one.
func TestBreaking(t *testing.T) {
	cases := map[string]string{
		"feat: a thing":      "",
		"feat!: drop a flag": "feat!: drop a flag",
		"feat!: drop a flag\n\nBREAKING-CHANGE: use --x":                                        "use --x",
		"fix: a bug\n\nWhy.\n\nBREAKING CHANGE: a key goes":                                     "a key goes",
		"fix: a bug\n\nWhy.\n\nBREAKING-CHANGE: rename the\n  flag in scripts\nUpgrading: none": "rename the\nflag in scripts",
		"fix: a bug\n\nBREAKING-CHANGE: not a footer here\n\nTask: T-1":                         "",
	}
	for message, want := range cases {
		if got := breaking(message); got != want {
			t.Errorf("breaking(%q) = %q, want %q", message, got, want)
		}
	}
}

// itos commit footers lists a footer's first line; whole gives it the lines
// the commit wrapped it onto.
func TestWhole(t *testing.T) {
	commits := parseLog("abcdef1234\x1ffeat: a thing\n\nWhy.\n\nUpgrading: rename the flag\n  in your scripts\nScenarios: ID-CLI-02\n\x1e\n")
	listed := []footer{{SHA: "abcdef1", Subject: "feat: a thing", Text: "rename the flag"}}
	whole(listed, "Upgrading", commits)
	if want := "rename the flag\nin your scripts"; listed[0].Text != want {
		t.Errorf("whole: %q, want %q", listed[0].Text, want)
	}
}

// The notes: the title, the count and why, what changed, then Upgrading with
// every breaking change and every Upgrading footer quoted, and how to check.
func TestRender(t *testing.T) {
	n := notes{
		Version:    "1.0.0",
		Repository: repository,
		From:       "v0.1.0",
		Changed:    "### Features\n\n- A thing",
		Commits: parseLog("a1b2c3d4\x1ffeat!: drop a flag\n\nBREAKING-CHANGE: use --x\n\x1e" +
			"b2c3d4e5\x1ffix: a bug\n\nUpgrading: none\n\x1e" +
			"c3d4e5f6\x1fdocs: say so\n\x1e"),
		Upgrading: []footer{{SHA: "a1b2c3d", Subject: "feat!: drop a flag", Text: "Use --x."}},
	}
	got, err := n.render()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# itos-container 1.0.0\n",
		"Cut by CI from the 3 commits since v0.1.0: 1 feat, 1 fix and 1 breaking change,",
		"https://github.com/donvargax/itos-container/compare/v0.1.0...v1.0.0",
		"## What changed\n\n### Features\n\n- A thing\n",
		"   - Breaking, a1b2c3d `feat!: drop a flag`:\n     > use --x\n",
		"   - a1b2c3d `feat!: drop a flag`:\n     > Use --x.\n",
		"`itos-container --version` prints `itos-container 1.0.0`",
		"gh attestation verify <archive> -R donvargax/itos-container",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the notes lack %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "## Upgrading") < strings.Index(got, "## What changed") {
		t.Errorf("Upgrading is not after What changed:\n%s", got)
	}

	first, err := notes{Version: "0.1.0", Repository: repository, Commits: parseLog("a1\x1ffeat: a thing\n\x1e")}.render()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"since the first commit: 1 feat, 0 fixes and 0 breaking changes",
		"No commit since the first commit is a breaking change. Every `feat` and `fix` since the first commit says `Upgrading: none`.",
	} {
		if !strings.Contains(first, want) {
			t.Errorf("a first release's notes lack %q:\n%s", want, first)
		}
	}
	if strings.Contains(first, "compare/") {
		t.Errorf("a first release's notes link a comparison:\n%s", first)
	}
}

// Why a release was cut, over example repositories (decision 19): each starts
// from releasetest.Example's v0.1.0, commits one change, and reads the range
// as the notes do. With no feat, fix or breaking change, the opening line
// names what moved beneath the binary, each module old to new and the
// toolchain, and Upgrading says there is nothing to change; a fix or a feat
// beside a moved module reads as it always has, what moved never read.
func TestMovedNotes(t *testing.T) {
	const moved = "example.com/lib v1.0.0 => ./lib to v1.1.0 => ./lib; the toolchain go1.24.0 to go1.24.1"
	cases := []struct {
		name, message string
		upgrading     []footer
		says, not     []string
	}{
		{name: "a build commit moving a linked module and the toolchain", message: "build: move lib and Go",
			says: []string{
				"Cut by CI from the 1 commit since v0.1.0: no feat, no fix and no breaking change, but the binary is built from what moved beneath it, which makes a patch: " + moved + ". Every change: https://github.com/donvargax/itos-container/compare/v0.1.0...v0.1.1\n",
				"1. **What the commits ask.** Nothing to change: no commit since v0.1.0 is a breaking change, and none has an `Upgrading:` footer asking for anything.\n",
			},
			not: []string{"Every `feat` and `fix`"}},
		{name: "a build commit whose footer asks for something", message: "build: move lib and Go",
			upgrading: []footer{{SHA: "a1b2c3d", Subject: "build: move lib and Go", Text: "Rebuild your plugins."}},
			says: []string{
				"which makes a patch: " + moved + ".",
				"No commit since v0.1.0 is a breaking change.\n   Each `Upgrading:` footer since v0.1.0",
				"> Rebuild your plugins.",
			},
			not: []string{"Nothing to change"}},
		{name: "a fix beside a moved module", message: "fix: a bug\n\nUpgrading: none",
			says: []string{
				"Cut by CI from the 1 commit since v0.1.0: 0 feats, 1 fix and 0 breaking changes, where a breaking change makes a major release, a feat a minor one and a fix a patch.",
				"Every `feat` and `fix` since v0.1.0 says `Upgrading: none`.",
			},
			not: []string{"moved", "Nothing to change"}},
		{name: "a feat beside a moved module", message: "feat: a thing\n\nUpgrading: none",
			says: []string{"1 feat, 0 fixes and 0 breaking changes, where"},
			not:  []string{"moved"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := releasetest.Example(t, true)
			releasetest.Change(t, filepath.Join(repo, "go.mod"), "example.com/lib v1.0.0", "example.com/lib v1.1.0")
			releasetest.Change(t, filepath.Join(repo, "go.mod"), "toolchain go1.24.0", "toolchain go1.24.1")
			releasetest.Commit(t, repo, c.message)
			t.Chdir(repo)
			n := notes{Version: "0.1.1", Repository: repository, From: "v0.1.0", Upgrading: c.upgrading}
			var err error
			if n.Commits, err = commits("v0.1.0..HEAD"); err != nil {
				t.Fatal(err)
			}
			if err := n.readMoved("HEAD"); err != nil {
				t.Fatal(err)
			}
			got, err := n.render()
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range c.says {
				if !strings.Contains(got, want) {
					t.Errorf("the notes lack %q:\n%s", want, got)
				}
			}
			for _, unwanted := range c.not {
				if strings.Contains(got, unwanted) {
					t.Errorf("the notes hold %q:\n%s", unwanted, got)
				}
			}
		})
	}
}

// With no release before, nothing moved is read: there is no side to compare
// with, and a first release needs a feat, fix or breaking change.
func TestMovedFirstRelease(t *testing.T) {
	n := notes{Version: "0.1.0", Commits: parseLog("a1\x1fbuild: start\n\x1e")}
	if err := n.readMoved("HEAD"); err != nil || n.Moved != nil {
		t.Errorf("readMoved with no last release = %q, %v; want nothing read", n.Moved, err)
	}
}

func TestBootstrapNotes(t *testing.T) {
	for _, message := range []string{
		"feat: first feature", "fix: first fix", "refactor!: first break",
		"docs: migration\n\nBREAKING CHANGE: rename the flag\n  in scripts\nUpgrading: use the new flag\n  in scripts",
	} {
		t.Run(message, func(t *testing.T) {
			n := notes{Version: "0.1.0", Repository: repository,
				Commits:   parseLog("abc1234\x1f" + message + "\x1e"),
				Upgrading: []footer{{SHA: "abc1234", Subject: "migration", Text: "use the new flag\nin scripts"}},
			}
			got, err := n.render()
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"# itos-container 0.1.0", "initial version is 0.1.0", "> use the new flag\n     in scripts"} {
				if !strings.Contains(got, want) {
					t.Errorf("bootstrap notes lack %q:\n%s", want, got)
				}
			}
			if strings.Contains(got, "where a breaking change makes a major") || strings.Contains(got, "compare/") {
				t.Errorf("bootstrap notes describe ordinary version arithmetic or a prior tag:\n%s", got)
			}
			if strings.Contains(message, "BREAKING CHANGE:") && !strings.Contains(got, "> rename the flag\n     in scripts") {
				t.Errorf("bootstrap lost the complete breaking footer:\n%s", got)
			}
			if strings.Contains(message, "refactor!:") && !strings.Contains(got, "Breaking, abc1234 `refactor!: first break`") {
				t.Errorf("bootstrap lost its breaking header:\n%s", got)
			}
		})
	}
}

// A merge of main into an item branch is not a change of the release: the
// notes leave it out, whatever its subject says or its first parent is, and the
// merge that lands the item on main is listed under its own subject. Each case
// builds two items over an example repository, slice-13 whose sync merge is
// subjected as a feat and slice-14 whose sync merge has a feat as its first
// parent, lands both on main, and renders what git-cliff would, every commit
// under Features.
func TestSyncMergesLeftOut(t *testing.T) {
	for _, tagged := range []bool{false, true} {
		name, rng, since := "a first release", "HEAD", "the first commit"
		if tagged {
			name, rng, since = "a release after v0.1.0", "v0.1.0..HEAD", "v0.1.0"
		}
		t.Run(name, func(t *testing.T) {
			repo := releasetest.Example(t, tagged)
			empty := func(message string) {
				releasetest.Git(t, repo, "commit", "--quiet", "--allow-empty", "-m", message)
			}
			merge := func(message, branch string) {
				releasetest.Git(t, repo, "merge", "--quiet", "--no-ff", "--no-edit", "-m", message, branch)
			}
			for _, item := range []struct{ branch, thing, sync string }{
				{"slice-13", "feat: slice-13 thing", "feat: catch up"},
				{"slice-14", "feat: slice-14 thing", "Refresh the item"},
			} {
				releasetest.Git(t, repo, "checkout", "--quiet", "-b", item.branch)
				empty(item.thing)
				if item.branch == "slice-14" {
					releasetest.Git(t, repo, "checkout", "--quiet", "-b", "item-detail")
					empty("docs: describe the item")
					releasetest.Git(t, repo, "checkout", "--quiet", item.branch)
					merge("feat: integrate item detail", "item-detail")
				}
				releasetest.Git(t, repo, "checkout", "--quiet", "main")
				empty("docs: main moves on past " + item.branch)
				releasetest.Git(t, repo, "checkout", "--quiet", item.branch)
				merge(item.sync, "main")
				releasetest.Git(t, repo, "checkout", "--quiet", "main")
				merge("feat: land "+item.branch, item.branch)
			}
			t.Chdir(repo)
			list, err := commits(rng)
			if err != nil {
				t.Fatal(err)
			}
			syncs := map[string]string{} // SHA to subject
			var changed strings.Builder
			changed.WriteString("### Features\n\n")
			for _, c := range list {
				if c.Header == "feat: catch up" || c.Header == "Refresh the item" {
					syncs[c.SHA] = c.Header
				}
				fmt.Fprintf(&changed, "- %s ([`%s`](https://github.com/%s/commit/%s))\n", c.Header, c.SHA[:7], repository, c.SHA)
			}
			if len(syncs) != 2 {
				t.Fatalf("the example history holds %d sync merges, want 2: %v", len(syncs), syncs)
			}
			n := notes{Version: "0.1.0", Repository: repository, Commits: list, Changed: strings.TrimSpace(changed.String())}
			if tagged {
				n.Version, n.From = "0.2.0", "v0.1.0"
			}
			got, err := n.render()
			if err != nil {
				t.Fatal(err)
			}
			for sha, subject := range syncs {
				if strings.Contains(got, sha) || strings.Contains(got, subject) {
					t.Errorf("the notes hold the sync merge %s %q:\n%s", sha[:7], subject, got)
				}
			}
			features := ""
			if at := strings.Index(got, "### Features"); at >= 0 {
				features = got[at:]
			}
			for _, c := range list {
				if c.Header == "feat: land slice-13" || c.Header == "feat: land slice-14" || c.Header == "feat: integrate item detail" {
					if want := "- " + c.Header + " ([`" + c.SHA[:7] + "`]"; !strings.Contains(features, want) {
						t.Errorf("the notes lack the landing merge %q under Features:\n%s", want, got)
					}
				}
			}
			commitCount := "9 commits"
			if tagged {
				commitCount = "8 commits"
			}
			want := "Cut by CI from the " + commitCount + " since " + since + ": 5 feats, 0 fixes and 0 breaking changes"
			if !strings.Contains(got, want) {
				t.Errorf("the notes lack %q:\n%s", want, got)
			}
		})
	}
	t.Run("version calculation", checkSyncMergeVersionExplanation)
}

// A typed sync contributes to the version even if a dependency also moved.
// The item history has no listed feature that could explain a minor bump.
func checkSyncMergeVersionExplanation(t *testing.T) {
	repo := releasetest.Example(t, true)
	releasetest.Git(t, repo, "checkout", "--quiet", "-b", "topic")
	releasetest.Git(t, repo, "commit", "--quiet", "--allow-empty", "-m", "docs: describe the change")
	releasetest.Git(t, repo, "checkout", "--quiet", "main")
	releasetest.Change(t, filepath.Join(repo, "go.mod"), "example.com/lib v1.0.0", "example.com/lib v1.1.0")
	releasetest.Commit(t, repo, "build: update library")
	releasetest.Git(t, repo, "checkout", "--quiet", "topic")
	releasetest.Git(t, repo, "merge", "--quiet", "--no-ff", "-m", "feat: catch up", "main")
	releasetest.Git(t, repo, "checkout", "--quiet", "main")
	releasetest.Git(t, repo, "merge", "--quiet", "--no-ff", "-m", "docs: land topic", "topic")
	t.Chdir(repo)
	list, err := commits("v0.1.0..HEAD")
	if err != nil {
		t.Fatal(err)
	}
	n := notes{From: "v0.1.0", Version: "0.2.0", Repository: repository, Commits: list}
	if err := n.readMoved("HEAD"); err != nil {
		t.Fatal(err)
	}
	got, err := n.render()
	if err != nil {
		t.Fatal(err)
	}
	requireText(t, got, "0 feats, 0 fixes and 0 breaking changes", "The version calculation includes branch syncs: 1 feat, 0 fixes and 0 breaking changes.")
	if strings.Contains(got, "which makes a patch") {
		t.Errorf("described the minor release as a dependency patch:\n%s", got)
	}
}

func TestLastReleaseExcludesOwnTag(t *testing.T) {
	for _, tagged := range []bool{false, true} {
		repo := releasetest.Example(t, tagged)
		releasetest.Git(t, repo, "commit", "--quiet", "--allow-empty", "-m", "feat: new")
		version, want := "v0.1.0", ""
		if tagged {
			version, want = "v0.2.0", "v0.1.0"
		}
		t.Chdir(repo)
		for _, createTag := range []bool{false, true} {
			if createTag {
				releasetest.Git(t, repo, "tag", version)
			}
			got, err := lastRelease("HEAD", version)
			if err != nil || got != want {
				t.Errorf("previous tag, tagged=%v own=%v: got %q, %v; want %q", tagged, createTag, got, err, want)
			}
		}
	}
}
