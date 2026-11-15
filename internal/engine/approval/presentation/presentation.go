// Package presentation renders safe approval reviews and structured outcomes.
package presentation

import (
	"bufio"
	"encoding/json"
	"fmt"
	"golang.org/x/term"
	"io"
	"os"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/donvargax/itos-container/internal/engine/approval/domain"
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
		report.Differences = domain.Differences(object(v.Record.Canonical), report.Effective)
	}
	return report
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
		if allowlist(report.Effective) != nil {
			u.networkDisclosure(report.Effective)
		}
		if report.Status != "matching" {
			_, _ = fmt.Fprintln(u.Out, "Review with itos container approve")
		}
	}
	return code
}

// Review shows all data before accepting only a complete yes line. Display-only
// escaping prevents terminal controls from altering the review or its differences.
func (u UI) Review(v port.View) bool {
	effective := object(v.Candidate.Canonical)
	if allowlist(effective) != nil {
		_, _ = fmt.Fprintln(u.Out, "Effective settings:", terminalText(v.Candidate.Canonical))
		u.networkDisclosure(effective)
	} else {
		_, _ = fmt.Fprintln(u.Out, "Effective offline settings:", terminalText(v.Candidate.Canonical))
		_, _ = fmt.Fprintln(u.Out, "offline-v1: checkout-only host mount at /workspace; UID:GID override; no supplementary groups; read-only root; dropped capabilities; no-new-privileges; network none; no ports, grants or hooks; ephemeral tmpfs; pids limit 4096")
	}
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

// allowlist reads only the validated candidate, never a previous approval.
func allowlist(effective any) []any {
	s, _ := effective.(map[string]any)
	if s["profile"] != "allowlist-v1" {
		return nil
	}
	p, _ := s["policy"].(map[string]any)
	n, _ := p["network"].(map[string]any)
	names, _ := n["allow"].([]any)
	return names
}

func (u UI) networkDisclosure(effective any) {
	_, _ = fmt.Fprintln(u.Out, "allowlist-v1: checkout-only host mount at /workspace; UID:GID override; no supplementary groups; read-only root; dropped capabilities; no-new-privileges; internal network with no route out; no ports, grants or hooks; ephemeral tmpfs; pids limit 4096")
	_, _ = fmt.Fprintln(u.Out, "This profile grants network. The worker can reach only these host names, through an owned proxy, by CONNECT on port 443:")
	for _, name := range allowlist(effective) {
		_, _ = fmt.Fprintln(u.Out, "network.allow:", name)
	}
	_, _ = fmt.Fprintln(u.Out, "No TLS interception: the proxy verifies the CONNECT name and the first ClientHello's SNI.")
	_, _ = fmt.Fprintln(u.Out, "It cannot see the HTTP Host inside TLS or anything after that hello, a second hello after a HelloRetryRequest included, so fronting remains a limit, as does a hello the upstream reads differently, and it cannot stop data sent to an allowed service.")
	_, _ = fmt.Fprintln(u.Out, "Clients that omit SNI or send ECH, and SSH over 443, are refused.")
	_, _ = fmt.Fprintln(u.Out, "The proxy logs each refusal, but a stored log is no proof that every refusal was kept: a log that ends early reads as a complete one.")
	_, _ = fmt.Fprintln(u.Out, "Run cannot start this profile yet: it refuses every version 2 policy for now.")
}
