package main

import (
	"fmt"
	"strings"
)

const bodyLimit = 125000 // GitHub's character limit, conservatively enforced in bytes.

// Keep the original commits for complete counts even after removing instructions.
// Selection below replaces slices; it never edits the caller's backing arrays.
type renderedNotes struct {
	notes
	Breaking         []commit
	DependencyPatch  bool
	Omitted          bool
	MigrationOmitted bool
}

func (n notes) history() string {
	base := "https://github.com/" + n.Repository
	if n.From == "" {
		return base + "/commits/v" + n.Version
	}
	return base + "/compare/" + n.From + "...v" + n.Version
}

// A controlled cliff.toml entry occupies one line. Keeping its rendered text
// avoids slicing a subject, Markdown link or UTF-8 sequence when space runs out.
type change struct {
	group int
	text  string
}

type changes struct {
	headings []string
	entries  []change
}

func readChanges(text string) changes {
	var out changes
	group := -1
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "### "):
			out.headings = append(out.headings, line)
			group++
		case strings.HasPrefix(line, "- "):
			out.entries = append(out.entries, change{group: group, text: line})
		}
	}
	return out
}

func (c changes) text() string {
	var b strings.Builder
	previous := -2
	for _, entry := range c.entries {
		if previous != entry.group {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			if entry.group >= 0 {
				b.WriteString(c.headings[entry.group])
				b.WriteByte('\n')
			}
			previous = entry.group
		}
		b.WriteString(entry.text)
		b.WriteByte('\n')
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// Git-cliff can split a history at prerelease/own tags. Select by the actual
// commit type and full SHA, consolidating the first release into two groups in
// oldest-first Git order, independently of those tag divisions.
func (c changes) first(commits []commit) changes {
	bySHA := make(map[string]string)
	for _, entry := range c.entries {
		if at := strings.LastIndex(entry.text, "/commit/"); at >= 0 {
			sha := strings.TrimSuffix(entry.text[at+len("/commit/"):], "))")
			bySHA[sha] = entry.text
		}
	}
	out := changes{headings: []string{"### Features", "### Fixes"}}
	for group, kind := range []string{"feat", "fix"} {
		for _, commit := range commits {
			if text, ok := bySHA[commit.SHA]; ok && commit.Type == kind {
				out.entries = append(out.entries, change{group: group, text: text})
				delete(bySHA, commit.SHA)
			}
		}
	}
	return out
}

func (n notes) render() (string, error) {
	view := renderedNotes{notes: n, DependencyPatch: len(n.Moved) > 0}
	for _, c := range n.Commits {
		if c.Breaking != "" {
			view.Breaking = append(view.Breaking, c)
		}
	}
	entries := readChanges(n.Changed)
	if n.From == "" {
		entries = entries.first(n.Commits)
		view.Changed = entries.text()
	}
	text, err := view.document()
	if err != nil || len(text) < bodyLimit {
		return text, err
	}

	// Reserve all mandatory framing and both notices before selecting entries.
	// The full candidate above is the only path that emits no omission notice.
	view.Omitted = true
	frame := view
	frame.Changed, frame.Moved, frame.Breaking, frame.Upgrading = "", nil, nil, nil
	frame.MigrationOmitted = len(view.Breaking)+len(view.Upgrading) > 0
	text, err = frame.document()
	if err != nil {
		return "", err
	}
	if len(text) >= bodyLimit {
		return "", fmt.Errorf("mandatory release-note framing cannot fit below %d bytes", bodyLimit)
	}

	// Discard individually impossible entries before smaller entries can be
	// displaced. Each probe renders one whole entry beside the reserved frame.
	entries.entries, err = fitting(entries.entries, func(entry change) (string, error) {
		one := frame
		one.Changed = (changes{headings: entries.headings, entries: []change{entry}}).text()
		return one.document()
	})
	if err != nil {
		return "", err
	}
	view.Moved, err = fitting(view.Moved, func(entry string) (string, error) {
		one := frame
		one.Moved = []string{entry}
		return one.document()
	})
	if err != nil {
		return "", err
	}
	migrations := len(view.Breaking) + len(view.Upgrading)
	view.Breaking, err = fitting(view.Breaking, func(entry commit) (string, error) {
		one := frame
		one.Breaking = []commit{entry}
		one.MigrationOmitted = migrations > 1
		return one.document()
	})
	if err != nil {
		return "", err
	}
	view.Upgrading, err = fitting(view.Upgrading, func(entry footer) (string, error) {
		one := frame
		one.Upgrading = []footer{entry}
		one.MigrationOmitted = migrations > 1
		return one.document()
	})
	if err != nil {
		return "", err
	}
	view.MigrationOmitted = len(view.Breaking)+len(view.Upgrading) < migrations
	for {
		view.Changed = entries.text()
		text, err = view.document()
		if err != nil || len(text) < bodyLimit {
			return text, err
		}
		switch {
		case len(entries.entries) > 0:
			entries.entries = entries.entries[:len(entries.entries)-1]
		case len(view.Moved) > 0:
			view.Moved = view.Moved[:len(view.Moved)-1]
		case len(view.Upgrading) > 0:
			view.Upgrading = view.Upgrading[:len(view.Upgrading)-1]
			view.MigrationOmitted = true
		case len(view.Breaking) > 0:
			view.Breaking = view.Breaking[:len(view.Breaking)-1]
			view.MigrationOmitted = true
		default:
			return "", fmt.Errorf("mandatory release-note framing cannot fit below %d bytes", bodyLimit)
		}
	}
}

func fitting[T any](entries []T, render func(T) (string, error)) ([]T, error) {
	var kept []T
	for _, entry := range entries {
		text, err := render(entry)
		if err != nil {
			return nil, err
		}
		if len(text) < bodyLimit {
			kept = append(kept, entry)
		}
	}
	return kept, nil
}
