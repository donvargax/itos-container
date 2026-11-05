// Package presentation renders safe approval reviews and structured outcomes.
package presentation

import (
	"bufio"
	"encoding/json"
	"fmt"
	"golang.org/x/term"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// UI binds host terminal streams and the existing prerequisite classifiers.
type UI struct {
	In       io.Reader
	Out, Err io.Writer
}

func terminal(value any) bool { f, ok := value.(*os.File); return ok && term.IsTerminal(int(f.Fd())) }

// Terminal requires both review descriptors to be terminals.
func (u UI) Terminal() bool { return terminal(u.In) && terminal(u.Out) }
func object(text string) any {
	if text == "" {
		return nil
	}
	var value any
	_ = json.Unmarshal([]byte(text), &value)
	return value
}
func jsonText(value any) string { data, _ := json.Marshal(value); return terminalText(string(data)) }

// terminalText escapes non-printing Unicode for display only. encoding/json
// already escapes ASCII controls but leaves C1 and bidi controls as literal UTF-8.
// JSON escapes (including surrogate pairs) preserve the decoded values without
// changing the canonical bytes used by approval records and fingerprints.
func terminalText(text string) string {
	var out strings.Builder
	for _, r := range text {
		if unicode.IsPrint(r) {
			out.WriteRune(r)
		} else {
			for _, unit := range utf16.Encode([]rune{r}) {
				_, _ = fmt.Fprintf(&out, `\u%04x`, unit)
			}
		}
	}
	return out.String()
}

// Report computes sorted leaf differences, comparing containerEnv as one map.
func (u UI) Report(v port.View) port.Report {
	report := port.Report{Status: v.Status, Fingerprint: v.Candidate.Fingerprint, Effective: object(v.Candidate.Canonical), Differences: []port.Change{}}
	report.EffectiveText = jsonText(report.Effective)
	if v.Record.Generation != "" {
		g := v.Record.Generation
		report.Generation = &g
		differences("", object(v.Record.Canonical), report.Effective, &report.Differences)
	}
	sort.Slice(report.Differences, func(i, j int) bool { return report.Differences[i].Path < report.Differences[j].Path })
	return report
}
func differences(path string, before, after any, out *[]port.Change) {
	if reflect.DeepEqual(before, after) {
		return
	}
	b, bok := before.(map[string]any)
	a, aok := after.(map[string]any)
	if bok && aok && path != "/manifest/containerEnv" {
		keys := map[string]bool{}
		for key := range b {
			keys[key] = true
		}
		for key := range a {
			keys[key] = true
		}
		for key := range keys {
			escaped := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
			differences(path+"/"+escaped, b[key], a[key], out)
		}
		return
	}
	*out = append(*out, port.Change{Path: path, Before: before, After: after})
}

// Problems reuses prerequisite rules and maps only approval's typed failures.
func (u UI) Problems(v port.View) []port.Problem { return port.Problems(v) }

// Print emits one JSON object or escaped plain status, never a prompt.
func (u UI) Print(v port.View, withJSON bool) int {
	report, problems := u.Report(v), u.Problems(v)
	code := 0
	for _, p := range problems {
		code = max(code, p.Exit)
		_, _ = fmt.Fprintf(u.Err, "%s: %s; %s\n", p.Rule, p.Message, p.Fix)
	}
	if withJSON {
		data, _ := json.Marshal(struct {
			Schema   int            `json:"schema"`
			OK       bool           `json:"ok"`
			Problems []port.Problem `json:"problems"`
			Approval port.Report    `json:"approval"`
		}{1, code == 0, problems, report})
		_, _ = fmt.Fprintln(u.Out, string(data))
	} else {
		_, _ = fmt.Fprintf(u.Out, "approval %s\nfingerprint: %s\n%s\n", report.Status, report.Fingerprint, terminalText(v.Candidate.Canonical))
		if report.Status != "matching" {
			_, _ = fmt.Fprintln(u.Out, "Review with itos container approve")
		}
	}
	return code
}

// Review shows all data before accepting only a complete yes line. Display-only
// escaping prevents terminal controls from altering the review or its differences.
func (u UI) Review(v port.View) bool {
	_, _ = fmt.Fprintln(u.Out, "Effective offline settings:", terminalText(v.Candidate.Canonical))
	_, _ = fmt.Fprintln(u.Out, "offline-v1: checkout-only host mount at /workspace; UID:GID override; no supplementary groups; read-only root; dropped capabilities; no-new-privileges; network none; no ports, grants or hooks; ephemeral tmpfs; pids limit 4096")
	if m := v.Inspection.Manifest; m != nil {
		for _, unused := range append(append([]string{}, m.Unused...), m.Metadata.Unused...) {
			_, _ = fmt.Fprintln(u.Out, "Unused field:", jsonText(unused))
		}
	}
	for _, change := range u.Report(v).Differences {
		_, _ = fmt.Fprintf(u.Out, "%s: %s -> %s\n", terminalText(change.Path), jsonText(change.Before), jsonText(change.After))
	}
	_, _ = fmt.Fprint(u.Out, "Approve these settings? y/N: ")
	line, err := bufio.NewReader(io.LimitReader(u.In, 4097)).ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}
