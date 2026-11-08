// Adapted from donvargax/itos-template at 79301eba51b9768a27906c720e39c49b08491e67.
// T-25 retargets project identities and selects 0.1.0 for every releasable first history.
// Command release-notes writes a release's notes from its commits (T-5):
// they are generated, never committed, and published as the release's
// description by the release workflow once the archives are attested.
// Harvested from itos's tools/bin/release-notes (its T-069), less what is
// itos's own: the Changes footer, the config's schema contract, the install
// script and pin lines a consumer of itos copies, and upgrading.json.
//
//	go run ./tools/bin/release-notes -version <X.Y.Z> [-from <tag>] [-to <rev>] [-itos <bin>]
//
// The range is <from>..<to>: <from> the last release's tag (default: the
// newest vX.Y.Z tag reachable from <to> other than v<version>, which the
// release job has already made locally), <to> HEAD by default. The notes are
// Markdown on stdout:
//
//   - a title and a line saying what the range holds, and why the version is
//     what it is (a breaking change makes a major, a feat a minor, a fix a
//     patch, as tools/bin/release-version computes it); with none of those, a
//     patch cut because the binary is built from what moved beneath it
//     (decision 23), each module that moved, old to new, and the toolchain,
//     named from internal/release's BinaryMoved, the one reading
//     release-version cuts that patch on (T-16);
//   - "What changed": every commit of the range, grouped by type, from
//     git-cliff (tools/bin/release-notes/cliff.toml, run by tools/bin/pinned);
//   - "Upgrading", last: every breaking change's BREAKING-CHANGE: footer (or
//     its ! header), and every Upgrading: footer quoted whole, as `itos commit
//     footers Upgrading` lists them (those saying none left out), or, for a
//     patch cut for what moved, that there is nothing to change when no
//     footer asks for anything; then how to check the binary and its
//     attestation.
//
// What moved is read only when no commit of the range is a feat, a fix or a
// breaking change, and only from a last release's tag: go list reads each
// side, so it needs the modules of both commits, the module cache or the
// network to the module proxy, as the release job has (actions/setup-go runs
// before it). It imports nothing but the standard library and
// internal/release. Exit status: 0 written, 2 a part could not be read (a
// tool that fails).
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/donvargax/itos-container/internal/release"
)

const (
	self       = "release-notes"
	repository = "donvargax/itos-container"
)

var (
	typed  = regexp.MustCompile(`^([a-zA-Z]+)(\([^)]*\))?(!)?: `)
	semver = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
)

func main() {
	os.Exit(run())
}

func run() int {
	ver := flag.String("version", "", "the version released, X.Y.Z")
	from := flag.String("from", "", "the last release's tag (default: the newest vX.Y.Z reachable from -to but v<version>)")
	to := flag.String("to", "HEAD", "the range's end")
	itos := flag.String("itos", "itos", "the itos whose commit footers reads the range's footers")
	flag.Parse()
	fail := func(format string, a ...any) int {
		fmt.Fprintf(os.Stderr, self+": "+format+"\n", a...)
		return 2
	}
	if flag.NArg() > 0 || !semver.MatchString(*ver) {
		return fail("usage: go run ./tools/bin/release-notes -version <X.Y.Z> [-from <tag>] [-to <rev>] [-itos <bin>]")
	}
	n := notes{Version: *ver, Repository: repository, From: *from}
	var err error
	if n.From == "" {
		if n.From, err = lastRelease(*to, "v"+*ver); err != nil {
			return fail("%v", err)
		}
	}
	rng := *to
	if n.From != "" {
		rng = n.From + ".." + *to
	}
	n.Range = rng
	if n.Commits, err = commits(rng); err != nil {
		return fail("%v", err)
	}
	if err := n.readMoved(*to); err != nil {
		return fail("%v", err)
	}
	if n.Changed, err = cliff(rng); err != nil {
		return fail("%v", err)
	}
	if n.Upgrading, err = footers(*itos, "Upgrading", n.From, *to); err != nil {
		return fail("%v", err)
	}
	whole(n.Upgrading, "Upgrading", n.Commits)
	text, err := n.render()
	if err != nil {
		return fail("%v", err)
	}
	fmt.Print(text)
	return 0
}

// notes is everything the template reads. Moved is what moved beneath the
// binary, for a patch with no feat, fix or breaking change, else none.
type notes struct {
	Version, Repository, From, Range, Changed string
	Commits                                   []commit
	Upgrading                                 []footer
	Moved                                     []string
}

// readMoved reads what moved beneath the binary from the last release to
// to, by internal/release's BinaryMoved, when the range holds no feat, no
// fix and no breaking change: then what moved is why the release was cut.
// With one of those, or no release before, the notes read as they always
// have, and nothing is read.
func (n *notes) readMoved(to string) error {
	if n.From == "" || n.count("feat")+n.count("fix")+n.count("breaking") > 0 {
		return nil
	}
	moved, err := release.BinaryMoved(n.From, to)
	if err != nil {
		return fmt.Errorf("cannot tell what moved beneath the binary since %s: %v", n.From, err)
	}
	n.Moved = moved
	return nil
}

type commit struct {
	SHA, Header, Type, Breaking string // Breaking: what its footer asks, or its header for a !
	message                     string
}

type footer struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
}

func command(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...) //nolint:gosec // G204: callers select Git, the pinned launcher or the operator's itos binary, with explicit argv and no shell
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), "RUST_LOG=warn")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %v\n%s", name, strings.Join(args, " "), err, stderr.String())
	}
	return string(out), nil
}

// lastRelease is the newest release tag reachable from rev but own, or "",
// picked by release.Newest, the release cut's rule.
func lastRelease(rev, own string) (string, error) {
	out, err := command("git", "tag", "--merged", rev, "--list", "v*")
	if err != nil {
		return "", err
	}
	var tags []string
	for _, t := range strings.Split(out, "\n") {
		if t != own {
			tags = append(tags, t)
		}
	}
	return release.Newest(tags), nil
}

// commits lists the range's commits, each with its type and what it breaks.
func commits(rng string) ([]commit, error) {
	out, err := command("git", "log", "--reverse", "--format=%H%x1f%B%x1e", rng)
	if err != nil {
		return nil, err
	}
	return parseLog(out), nil
}

// parseLog reads git log's records, oldest first, into commits.
func parseLog(log string) []commit {
	var list []commit
	for _, record := range strings.Split(log, "\x1e") {
		sha, message, ok := strings.Cut(strings.TrimLeft(record, "\n"), "\x1f")
		if !ok {
			continue
		}
		message = strings.TrimSpace(message)
		header, _, _ := strings.Cut(message, "\n")
		c := commit{SHA: sha, Header: header, Breaking: breaking(message), message: message}
		if t := typed.FindStringSubmatch(header); t != nil {
			c.Type = t[1]
		}
		list = append(list, c)
	}
	return list
}

// footerKey is a footer line's start in a message's last paragraph: a capitalised token, as
// every footer here is, so a wrapped line that starts "pin: {" continues the one above.
var footerKey = regexp.MustCompile(`^([A-Z][A-Za-z-]*|BREAKING CHANGE): `)

// footerText is the text of the footer of a message's last paragraph that
// starts with key and, when starts is not "", that text: its first line and
// every line after it up to the next footer, as commits here wrap a long one;
// "" when there is none.
func footerText(message, key, starts string) string {
	_, rest, _ := strings.Cut(message, "\n")
	if strings.TrimSpace(rest) == "" {
		return ""
	}
	paragraphs := strings.Split(strings.TrimSpace(rest), "\n\n")
	var text []string
	in := false
	for _, line := range strings.Split(paragraphs[len(paragraphs)-1], "\n") {
		m := footerKey.FindStringSubmatch(line)
		switch {
		case m != nil && len(text) > 0:
			return strings.Join(text, "\n")
		case m != nil && m[1] == key && strings.HasPrefix(strings.TrimSpace(line[len(m[0]):]), starts):
			in, text = true, []string{strings.TrimSpace(line[len(m[0]):])}
		case m != nil:
			in = false
		case in:
			text = append(text, strings.TrimSpace(line))
		}
	}
	return strings.Join(text, "\n")
}

// breaking is what a commit marked as breaking asks: its BREAKING-CHANGE:
// (or BREAKING CHANGE:) footer's text in the message's last paragraph, or its
// header when only a ! marks it; "" when it is not.
func breaking(message string) string {
	for _, key := range []string{"BREAKING-CHANGE", "BREAKING CHANGE"} {
		if text := footerText(message, key, ""); text != "" {
			return text
		}
	}
	header, _, _ := strings.Cut(message, "\n")
	if t := typed.FindStringSubmatch(header); t != nil && t[3] == "!" {
		return header
	}
	return ""
}

// whole gives each footer itos commit footers listed its whole text: itos
// reads a footer's first line alone, and a commit here wraps a long one onto
// the lines below it.
func whole(listed []footer, key string, commits []commit) {
	for i, f := range listed {
		for _, c := range commits {
			if strings.HasPrefix(c.SHA, f.SHA) {
				if text := footerText(c.message, key, f.Text); text != "" {
					listed[i].Text = text //nolint:gosec // G602, a false positive: i is from range over listed, so always in range
				}
			}
		}
	}
}

// cliff is the range's commits grouped by type, from git-cliff. git-cliff
// takes a range only as <from>..<to>, so a first release, with no tag to
// start from, gives it none and it reads every commit HEAD reaches: the
// range a first release describes when <to> is HEAD, and refused otherwise.
func cliff(rng string) (string, error) {
	args := []string{"git-cliff", "--offline", "--config", "tools/bin/release-notes/cliff.toml", "--strip", "all"}
	if strings.Contains(rng, "..") {
		args = append(args, rng)
	} else if rng != "HEAD" {
		return "", fmt.Errorf("no release before %s: a first release's notes need -to HEAD, since git-cliff takes a range only from a tag", rng)
	}
	out, err := command(filepath.Join("tools", "bin", "pinned"), args...)
	return strings.TrimSpace(out), err
}

// footers lists the range's footers by name, from itos commit footers.
func footers(itos, name, from, to string) ([]footer, error) {
	out, err := command(itos, "commit", "footers", name, from, to, "--json")
	if err != nil {
		return nil, err
	}
	var listed struct {
		Footers []footer `json:"footers"`
	}
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		return nil, fmt.Errorf("itos commit footers %s --json: %v", name, err)
	}
	return listed.Footers, nil
}

// count says how many commits of the range are of a type, or breaking.
func (n notes) count(kind string) int {
	c := 0
	for _, commit := range n.Commits {
		if (kind == "breaking" && commit.Breaking != "") || (kind != "breaking" && commit.Type == kind) {
			c++
		}
	}
	return c
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func (n notes) render() (string, error) {
	funcs := template.FuncMap{
		"count":  n.count,
		"plural": plural,
		"join":   strings.Join,
		"short":  func(sha string) string { return sha[:min(7, len(sha))] },
		// A footer's text as written, its lines kept: a blockquote whose later lines continue it
		// lazily, so the text reads back word for word.
		"quote": func(text string) string {
			return "> " + strings.ReplaceAll(strings.TrimSpace(text), "\n", "\n     ")
		},
		"since": func() string {
			if n.From == "" {
				return "the first commit"
			}
			return n.From
		},
	}
	t, err := template.New("notes").Funcs(funcs).Parse(notesTemplate)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := t.Execute(&b, n); err != nil {
		return "", err
	}
	return regexp.MustCompile(`\n{3,}`).ReplaceAllString(b.String(), "\n\n"), nil
}

const notesTemplate = `# itos-container {{.Version}}

Cut by CI from the {{plural (len .Commits) "commit" "commits"}} since {{since}}: {{if .Moved}}no feat, no fix and no breaking change, but the binary is built from what moved beneath it, which makes a patch: {{join .Moved "; "}}.{{else}}{{plural (count "feat") "feat" "feats"}}, {{plural (count "fix") "fix" "fixes"}} and {{plural (count "breaking") "breaking change" "breaking changes"}}, {{if .From}}where a breaking change makes a major release, a feat a minor one and a fix a patch.{{else}}the initial version is 0.1.0 for any releasable history, including fixes and breaking changes.{{end}}{{end}}{{if .From}} Every change: https://github.com/{{.Repository}}/compare/{{.From}}...v{{.Version}}{{end}}

## What changed

{{.Changed}}

## Upgrading

1. **What the commits ask.**
{{- if and .Moved (not .Upgrading)}} Nothing to change: no commit since {{since}} is a breaking change, and none has an ` + "`Upgrading:`" + ` footer asking for anything.
{{- else}}
{{- if eq (count "breaking") 0}} No commit since {{since}} is a breaking change.{{end}}
{{- range .Commits}}{{if .Breaking}}
   - Breaking, {{short .SHA}} ` + "`{{.Header}}`" + `:
     {{quote .Breaking}}
{{- end}}{{end}}
{{- if .Upgrading}}
   Each ` + "`Upgrading:`" + ` footer since {{since}} (` + "`itos commit footers Upgrading {{.From}} v{{.Version}}`" + `):
{{- range .Upgrading}}
   - {{short .SHA}} ` + "`{{.Subject}}`" + `:
     {{quote .Text}}
{{- end}}
{{- else}} Every ` + "`feat`" + ` and ` + "`fix`" + ` since {{since}} says ` + "`Upgrading: none`" + `.{{end}}
{{- end}}

2. **Check:** ` + "`itos-container --version`" + ` prints ` + "`itos-container {{.Version}}`" + `.

The archives are attested by the workflow that built them:
` + "`gh attestation verify <archive> -R {{.Repository}}`" + ` checks one, and
` + "`sha256sum --ignore-missing -c checksums.txt`" + ` checks the ones downloaded beside it.
`
