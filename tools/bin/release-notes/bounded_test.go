package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/donvargax/itos-container/internal/release/releasetest"
)

const notesLimit = 125000

func changeEntry(id int, subject string) string {
	return fmt.Sprintf("- %s ([`%07d`](https://github.com/%s/commit/%040d))", subject, id, repository, id)
}

func boundedFixture(first bool) notes {
	n := notes{Version: "0.2.0", Repository: repository, From: "v0.1.0",
		Commits: []commit{{SHA: fmt.Sprintf("%040d", 1), Header: "feat: stable change", Type: "feat"}},
		Changed: "### Features\n" + changeEntry(1, "Stable change"),
	}
	if first {
		n.Version, n.From = "0.1.0", ""
	}
	return n
}

func renderNotes(t *testing.T, n notes) string {
	t.Helper()
	body, err := n.render()
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func requireText(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("notes (%d bytes) lack %q", len(body), want)
		}
	}
}

func checkBounded(t *testing.T, n notes, body string) {
	t.Helper()
	if len(body) == 0 || len(body) >= notesLimit || !utf8.ValidString(body) {
		t.Errorf("notes must be nonempty UTF-8 below %d bytes; got %d bytes, valid=%v", notesLimit, len(body), utf8.ValidString(body))
	}
	requireText(t, body, "# itos-container "+n.Version+"\n", "## What changed\n", "## Upgrading\n",
		"`itos-container --version` prints `itos-container "+n.Version+"`",
		"gh attestation verify <archive> -R "+repository,
		"sha256sum --ignore-missing -c checksums.txt")
	if !strings.HasSuffix(body, "\n") {
		t.Error("notes lack their final newline")
	}
	if n.From == "" {
		requireText(t, body, "initial version is 0.1.0", "https://github.com/"+repository+"/commits/v"+n.Version,
			"Host sockets created or swapped into the checkout during a run remain reachable.",
			"Network mode `none` and a private IPC namespace do not prevent this access.",
			"The startup scan does not provide continuous enforcement.",
			"Approval records what was approved, not who.",
			"Neither route proves human presence or authenticates a separate approver.",
			"https://github.com/"+repository+"/blob/v"+n.Version+"/docs/security.md")
		if strings.Index(body, "## Known limits") > strings.Index(body, "## Upgrading") || strings.Contains(body, "compare/") {
			t.Error("first release has misplaced limits or an invented comparison")
		}
	} else {
		requireText(t, body, "https://github.com/"+repository+"/compare/"+n.From+"...v"+n.Version)
	}
	if again := renderNotes(t, n); again != body {
		t.Error("rendering is not deterministic or mutated its input")
	}
}

func TestBoundedReleaseNotes(t *testing.T) {
	t.Run("later document stays verbatim", func(t *testing.T) {
		n := boundedFixture(false)
		n.Changed = "### Features\n\n- Stable change"
		n.Upgrading = []footer{{SHA: "abcdef1234", Subject: "feat: stable change", Text: "Use café.\nKeep 日本語."}}
		const want = "# itos-container 0.2.0\n\n" +
			"Cut by CI from the 1 commit since v0.1.0: 1 feat, 0 fixes and 0 breaking changes, where a breaking change makes a major release, a feat a minor one and a fix a patch. Every change: https://github.com/donvargax/itos-container/compare/v0.1.0...v0.2.0\n\n" +
			"## What changed\n\n### Features\n\n- Stable change\n\n## Upgrading\n\n" +
			"1. **What the commits ask.** No commit since v0.1.0 is a breaking change.\n" +
			"   Each `Upgrading:` footer since v0.1.0 (`itos commit footers Upgrading v0.1.0 v0.2.0`):\n" +
			"   - abcdef1 `feat: stable change`:\n     > Use café.\n     Keep 日本語.\n\n" +
			"2. **Check:** `itos-container --version` prints `itos-container 0.2.0`.\n\n" +
			"The archives are attested by the workflow that built them:\n" +
			"`gh attestation verify <archive> -R donvargax/itos-container` checks one, and\n" +
			"`sha256sum --ignore-missing -c checksums.txt` checks the ones downloaded beside it.\n"
		if got := renderNotes(t, n); got != want {
			t.Errorf("under-budget later document changed:\n%s", got)
		}
	})

	for _, first := range []bool{false, true} {
		for _, source := range []string{"changes", "breaking", "upgrading", "dependencies"} {
			if first && source == "dependencies" {
				continue
			}
			for _, size := range []int{124998, 124999, 125000, 125001} {
				t.Run(fmt.Sprintf("first=%v/%s/%d", first, source, size), func(t *testing.T) {
					n := boundedFixture(first)
					const marker = "PADDING"
					set := func(value string) {
						switch source {
						case "changes":
							n.Changed = "### Features\n" + changeEntry(1, "Café 日本語 "+value)
						case "breaking":
							n.Commits[0].Breaking = "Use café 日本語 " + value
						case "upgrading":
							n.Upgrading = []footer{{SHA: "1234567", Subject: "feat: stable change", Text: "Use café 日本語 " + value}}
						case "dependencies":
							n.Commits[0].Type, n.Commits[0].Header = "build", "build: move modules"
							n.Moved = []string{"example.com/lib v1 to v2 " + value}
						}
					}
					set(marker)
					base := renderNotes(t, n)
					padding := strings.Repeat("x", size-len(base)+len(marker))
					set(padding)
					got := renderNotes(t, n)
					checkBounded(t, n, got)
					if size < notesLimit {
						if want := strings.Replace(base, marker, padding, 1); got != want || len(got) != size {
							t.Errorf("candidate of %d bytes was changed while it fits (got %d)", size, len(got))
						}
					} else {
						requireText(t, got, "omitted")
						if strings.Contains(got, strings.Repeat("x", 32)) {
							t.Error("oversized entry was retained or clipped instead of omitted whole")
						}
						if source == "breaking" || source == "upgrading" {
							_, upgrade, _ := strings.Cut(got, "## Upgrading\n")
							requireText(t, upgrade, "migration instructions", "omitted", "history")
							if strings.Contains(upgrade, "Upgrading: none") || strings.Contains(upgrade, "Nothing to change") || strings.Contains(upgrade, "Each `Upgrading:`") {
								t.Error("omitted migration instructions are described as complete or unnecessary")
							}
						}
						if source == "dependencies" {
							requireText(t, got, "no feat, no fix and no breaking change", "which makes a patch")
						}
					}
				})
			}
		}
	}

	t.Run("whole changes from the end before migrations", func(t *testing.T) {
		n := boundedFixture(false)
		one := changeEntry(1, "First café "+strings.Repeat("界", 16000))
		two := changeEntry(2, "Second "+strings.Repeat("é", 24000))
		three := changeEntry(3, "Last "+strings.Repeat("x", 48000))
		n.Changed = "### Features\n" + one + "\n" + two + "\n\n### Fixes\n" + three
		n.Upgrading = []footer{{SHA: "1234567", Subject: "feat: migrate", Text: "Keep this migration.\nAnd its continuation."}}
		got := renderNotes(t, n)
		checkBounded(t, n, got)
		requireText(t, got, one, two, "> Keep this migration.\n     And its continuation.", "omitted")
		if strings.Contains(got, "Last ") || strings.Contains(got, "### Fixes") || strings.Index(got, one) > strings.Index(got, two) {
			t.Error("changes were not removed whole from the end with empty headings dropped")
		}
	})

	t.Run("many changes retain the fitting prefix", func(t *testing.T) {
		for _, first := range []bool{false, true} {
			n := boundedFixture(first)
			n.Commits, n.Changed = nil, "### Features"
			var entries []string
			for id := 1; id <= 300; id++ {
				entry := changeEntry(id, fmt.Sprintf("Change %d %s", id, strings.Repeat("é", 500)))
				entries = append(entries, entry)
				n.Changed += "\n" + entry
				n.Commits = append(n.Commits, commit{SHA: fmt.Sprintf("%040d", id), Header: fmt.Sprintf("feat: change %d", id), Type: "feat"})
			}
			got := renderNotes(t, n)
			checkBounded(t, n, got)
			requireText(t, got, "300 commits", "300 feats", "omitted", entries[0])
			kept, ended := 0, false
			for _, entry := range entries {
				if strings.Contains(got, entry) {
					kept++
					if ended {
						t.Error("an earlier fitting change was displaced by a later change")
					}
				} else {
					ended = true
				}
			}
			if kept < 100 || kept >= len(entries) {
				t.Errorf("expected a substantial fitting prefix, got %d entries", kept)
			}
		}
	})

	t.Run("impossible early entries do not displace fitting ones", func(t *testing.T) {
		n := boundedFixture(false)
		n.Changed = "### Features\n" + changeEntry(9, "Huge "+strings.Repeat("界", notesLimit)) + "\n" + changeEntry(1, "Keep this small change")
		n.Commits = append(n.Commits, commit{SHA: "9999999", Header: "refactor!: giant migration", Breaking: strings.Repeat("é", notesLimit)})
		n.Upgrading = []footer{
			{SHA: "9999999", Subject: "oversized", Text: strings.Repeat("界", notesLimit)},
			{SHA: "1234567", Subject: "small", Text: "Keep the small migration.\nIncluding this line."},
		}
		got := renderNotes(t, n)
		checkBounded(t, n, got)
		requireText(t, got, changeEntry(1, "Keep this small change"), "> Keep the small migration.\n     Including this line.", "omitted")
		if strings.Contains(got, "Huge ") || strings.Contains(got, "giant migration") || strings.Contains(got, "`oversized`") {
			t.Error("an individually impossible entry survived")
		}
	})

	t.Run("migration entries removed from the end", func(t *testing.T) {
		n := boundedFixture(false)
		n.Commits[0].Breaking = "Keep breaking " + strings.Repeat("x", 45000)
		n.Upgrading = []footer{
			{SHA: "1111111", Subject: "first", Text: "Keep upgrading " + strings.Repeat("é", 22000)},
			{SHA: "2222222", Subject: "last", Text: "Remove upgrading " + strings.Repeat("界", 15000)},
		}
		got := renderNotes(t, n)
		checkBounded(t, n, got)
		requireText(t, got, "> "+n.Commits[0].Breaking, "> "+n.Upgrading[0].Text, "migration instructions", "omitted")
		if strings.Contains(got, "Remove upgrading") || strings.Contains(got, changeEntry(1, "Stable change")) {
			t.Error("migration truncation did not follow change truncation and preserve earlier instructions")
		}
	})

	t.Run("dependency details drop whole from the end", func(t *testing.T) {
		n := boundedFixture(false)
		n.Commits[0].Type = "build"
		n.Moved = []string{"first-module " + strings.Repeat("x", 50000), "second-module " + strings.Repeat("é", 25000), "last-toolchain " + strings.Repeat("界", 17000)}
		n.Upgrading = []footer{{SHA: "1234567", Subject: "build: upgrade", Text: "Keep build guidance."}}
		got := renderNotes(t, n)
		checkBounded(t, n, got)
		requireText(t, got, n.Moved[0], n.Moved[1], "which makes a patch", "Keep build guidance.", "omitted")
		if strings.Contains(got, "last-toolchain") {
			t.Error("last dependency entry was not removed whole")
		}
	})

	t.Run("impossible dependency does not displace small details", func(t *testing.T) {
		n := boundedFixture(false)
		n.Commits[0].Type = "build"
		n.Moved = []string{"giant-module " + strings.Repeat("界", notesLimit), "small-module v1 to v2", "the toolchain go1.24.0 to go1.24.1"}
		got := renderNotes(t, n)
		checkBounded(t, n, got)
		requireText(t, got, n.Moved[1], n.Moved[2], changeEntry(1, "Stable change"), "omitted")
		if strings.Contains(got, "giant-module") {
			t.Error("individually impossible dependency survived")
		}
	})

	t.Run("first groups consolidate in commit order", func(t *testing.T) {
		n := boundedFixture(true)
		n.Commits = nil
		for i, kind := range []string{"fix", "feat", "feat", "fix", "feature"} {
			n.Commits = append(n.Commits, commit{SHA: fmt.Sprintf("%040d", i+1), Type: kind})
		}
		n.Changed = "### Features\n" + changeEntry(3, "Third") + "\n" + changeEntry(5, "Not a feat") +
			"\n\n### Fixes\n" + changeEntry(4, "Fourth") + "\n\n### Features\n" + changeEntry(2, "Second") +
			"\n" + changeEntry(3, "Third") + "\n\n### Fixes\n" + changeEntry(1, "First")
		got := renderNotes(t, n)
		checkBounded(t, n, got)
		want := "### Features\n" + changeEntry(2, "Second") + "\n" + changeEntry(3, "Third") +
			"\n\n### Fixes\n" + changeEntry(1, "First") + "\n" + changeEntry(4, "Fourth")
		requireText(t, got, want)
		if strings.Count(got, "### Features") != 1 || strings.Count(got, "### Fixes") != 1 || strings.Contains(got, "Not a feat") {
			t.Error("first groups contain duplicates or an inexact commit type")
		}
	})

	t.Run("first groups and complete counts", func(t *testing.T) {
		for _, kind := range []string{"feat", "fix", "refactor"} {
			t.Run(kind, func(t *testing.T) {
				n := boundedFixture(true)
				n.Commits = parseLog(fmt.Sprintf("%040d\x1f%s(scope)!: first\n\nBREAKING CHANGE: Use café\n  then 日本語\nUpgrading: Keep the continuation\n  and this line\n\x1e", 1, kind) +
					fmt.Sprintf("%040d\x1fdocs: other\n\x1e", 2))
				n.Upgrading = []footer{{SHA: "0000000", Subject: kind + "(scope)!: first", Text: "Keep the continuation"}}
				whole(n.Upgrading, "Upgrading", n.Commits)
				group := map[string]string{"feat": "Features", "fix": "Fixes", "refactor": "Refactoring"}[kind]
				n.Changed = "### " + group + "\n" + changeEntry(1, "**scope:** First") + "\n\n### Documentation\n" + changeEntry(2, "Other")
				got := renderNotes(t, n)
				checkBounded(t, n, got)
				requireText(t, got, "2 commits since the first commit", "> Use café\n     then 日本語", "> Keep the continuation\n     and this line")
				if strings.Contains(got, "### Documentation") || strings.Contains(got, "### Refactoring") || strings.Contains(got, changeEntry(2, "Other")) {
					t.Error("first release lists a kind other than feat/fix")
				}
				if kind != "refactor" {
					requireText(t, got, "### "+group, changeEntry(1, "**scope:** First"))
				} else if strings.Contains(got, "### ") {
					t.Error("breaking-only history has a change group")
				}
			})
		}
	})

	t.Run("framing cannot fit", func(t *testing.T) {
		for _, first := range []bool{false, true} {
			n := boundedFixture(first)
			n.Version = strings.Repeat("1", notesLimit) + ".0.0"
			body, err := n.render()
			if err == nil || body != "" {
				t.Errorf("oversized framing must fail without output; got %d bytes, error %v", len(body), err)
			}
		}
	})

	t.Run("invalid UTF-8 cannot be successful output", func(t *testing.T) {
		n := boundedFixture(false)
		n.Changed += "\xff"
		body, err := n.render()
		if err == nil || body != "" {
			t.Errorf("invalid UTF-8 must fail without output; got %d bytes, error %v", len(body), err)
		}
	})
}

// Build only this tool. All failure probes execute its real entry point, in a
// disposable repository; no feature suite, engine or publication is involved.
func TestBoundedReleaseNotesCommandFailures(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "release-notes")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	repo := releasetest.Example(t, false)
	releasetest.Git(t, repo, "commit", "--quiet", "--allow-empty", "-m", "feat: first")
	// A native fixture executable also works on Windows, where a shell script
	// cannot stand in for the launcher. Its output is fixed, synthetic data.
	helperDir := t.TempDir()
	helperSource, helper := filepath.Join(helperDir, "fixture.go"), filepath.Join(helperDir, "fixture.exe")
	releasetest.Change(t, helperSource, "", `package main
import ("fmt"; "os")
func main() {
 if len(os.Args) > 1 && os.Args[1] == "git-cliff" {
  fmt.Println("### Features\n- First")
 } else {
  fmt.Println("{\"footers\":[]}")
 }
}
`)
	if out, err := exec.Command("go", "build", "-o", helper, helperSource).CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v\n%s", err, out)
	}
	helperBytes, err := os.ReadFile(helper)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, dir, itos, diagnostic string
		args                        []string
	}{
		{name: "Git", dir: t.TempDir(), itos: "unused", diagnostic: "git tag"},
		{name: "pinned", dir: repo, itos: "unused", diagnostic: "pinned"},
		{name: "itos", dir: repo, itos: filepath.Join(t.TempDir(), "missing-itos"), diagnostic: "missing-itos"},
		{name: "framing", dir: repo, itos: helper, diagnostic: "framing", args: []string{"-version", strings.Repeat("1", 32000) + ".0.0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "itos" || tc.name == "framing" {
				if err := os.MkdirAll(filepath.Join(repo, "tools/bin"), 0o700); err != nil {
					t.Fatal(err)
				}
				for _, name := range []string{"pinned", "pinned.exe"} {
					path := filepath.Join(repo, "tools/bin", name)
					// Replace the file instead of rewriting it in place: the previous probe
					// executed it, and Linux can still refuse writes to that inode (ETXTBSY).
					if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, helperBytes, 0o700); err != nil {
						t.Fatal(err)
					}
				}
			}
			args := append([]string{"-version", "0.1.0", "-itos", tc.itos}, tc.args...)
			cmd := exec.Command(binary, args...)
			cmd.Dir = tc.dir
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			if err == nil || cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), tc.diagnostic) {
				t.Errorf("want exit 2, empty stdout and useful stderr; got %v, stdout=%d bytes, stderr=%q", err, stdout.Len(), stderr.String())
			}
		})
	}
}
