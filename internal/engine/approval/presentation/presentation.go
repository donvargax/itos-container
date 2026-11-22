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
	report.RequestedText = append(grantLines(requested(v)), harnessLines(report.Effective)...)
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
		location := p.Rule
		if p.File != "" && p.Path != nil {
			location += " " + jsonText(p.File+*p.Path)
		}
		_, _ = fmt.Fprintf(u.Err, "%s: %s; %s\n", location, p.Message, p.Fix)
	}
	if withJSON {
		type inspectionState struct {
			Ready bool `json:"ready"`
		}
		var inspection *inspectionState
		if v.Inspection.Credentials != nil && len(v.Inspection.Credentials.Issues) > 0 {
			inspection = &inspectionState{Ready: false}
		}
		data, _ := json.Marshal(struct {
			Schema      int                    `json:"schema"`
			OK          bool                   `json:"ok"`
			Problems    []port.Problem         `json:"problems"`
			Approval    port.Report            `json:"approval"`
			Credentials []port.CredentialGrant `json:"credentials,omitempty"`
			Inspection  *inspectionState       `json:"inspection,omitempty"`
		}{1, code == 0, problems, report, requested(v), inspection})
		_, _ = fmt.Fprintln(u.Out, string(data))
	} else {
		_, _ = fmt.Fprintf(u.Out, "approval %s\nfingerprint: %s\n%s\n", report.Status, report.Fingerprint, terminalText(v.Candidate.Canonical))
		if allowlist(report.Effective) != nil {
			u.networkDisclosure(report.Effective)
		}
		u.credentialDisclosure(report.Effective)
		u.homeDisclosure(report.Effective)
		for _, line := range grantLines(requested(v)) {
			_, _ = fmt.Fprintln(u.Out, line)
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
	if allowlist(effective) != nil || len(grants(effective)) > 0 {
		_, _ = fmt.Fprintln(u.Out, "Effective settings:", terminalText(v.Candidate.Canonical))
	} else {
		_, _ = fmt.Fprintln(u.Out, "Effective offline settings:", terminalText(v.Candidate.Canonical))
	}
	if allowlist(effective) != nil {
		u.networkDisclosure(effective)
	} else {
		_, _ = fmt.Fprintln(u.Out, restriction(effective))
	}
	u.credentialDisclosure(effective)
	u.homeDisclosure(effective)
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
	if s["profile"] != "allowlist-v1" && s["profile"] != "allowlist-credentials-v1" && s["profile"] != "allowlist-home-v1" && s["profile"] != "allowlist-credentials-home-v1" {
		return nil
	}
	p, _ := s["policy"].(map[string]any)
	n, _ := p["network"].(map[string]any)
	names, _ := n["allow"].([]any)
	return names
}

func (u UI) homeDisclosure(effective any) {
	s, _ := effective.(map[string]any)
	if s["home"] == nil {
		return
	}
	host, _ := s["host"].(map[string]any)
	_, _ = fmt.Fprintf(u.Out, "Harness home: private volume at /home/itos, mode 0700, owner %s:%s.\n", host["uid"], host["gid"])
	_, _ = fmt.Fprintln(u.Out, "Scoped to this checkout and retained after exit and stop; the host home is never mounted or copied.")
	_, _ = fmt.Fprintln(u.Out, "Contents are hostile worker data reused by the next run. Only a terminal-consented reset or delete discards them; disabling home does not delete it.")
	for _, line := range harnessLines(effective) {
		_, _ = fmt.Fprintln(u.Out, line)
	}
}

// harnessLines state what an approved OpenCode harness binds, from the
// validated candidate: the profile, every environment value, where OpenCode
// keeps its state, who chooses the provider and model, and what it delivers.
func harnessLines(effective any) []string {
	s, _ := effective.(map[string]any)
	home, _ := s["home"].(map[string]any)
	harnesses, _ := home["harnesses"].([]any)
	var lines []string
	for _, entry := range harnesses {
		harness, _ := entry.(map[string]any)
		if harness["name"] != "opencode" {
			continue
		}
		lines = append(lines, fmt.Sprintf("OpenCode harness: profile %s.", terminalText(fmt.Sprint(harness["profile"]))))
		for _, pair := range port.OpenCodeEnvironment() {
			lines = append(lines, pair[0]+"="+pair[1])
		}
		lines = append(lines, "OpenCode global configuration, data and state live in /home/itos.",
			"The checkout's own opencode.json and .opencode configuration also apply, loaded before this profile, which overrides them where they conflict.",
			"Any managed configuration in the approved image (/etc/opencode) loads after this profile and overrides it.",
			"The image's other OPENCODE_* defaults stay in effect and can redirect or turn off configuration layers (OPENCODE_CONFIG, OPENCODE_CONFIG_DIR, which replaces the global directory, OPENCODE_DISABLE_PROJECT_CONFIG).",
			"The argv and the checkout choose the provider and model; run never does.")
		if networkMode(s) == "offline" {
			lines = append(lines, "No provider is reachable: this policy is offline.")
		} else {
			lines = append(lines, "A provider is reachable only through the approved allowlist.")
		}
		lines = append(lines, "No OpenCode login is delivered by this setting. The profile is no network or credential grant.")
	}
	return lines
}

func networkMode(s map[string]any) any {
	p, _ := s["policy"].(map[string]any)
	n, _ := p["network"].(map[string]any)
	return n["mode"]
}

func (u UI) networkDisclosure(effective any) {
	_, _ = fmt.Fprintln(u.Out, restriction(effective))
	_, _ = fmt.Fprintln(u.Out, "This profile grants network. The worker can reach only these host names, through an owned proxy, by CONNECT on port 443:")
	for _, name := range allowlist(effective) {
		_, _ = fmt.Fprintln(u.Out, "network.allow:", name)
	}
	_, _ = fmt.Fprintln(u.Out, "No TLS interception: the proxy verifies the CONNECT name and the first ClientHello's SNI.")
	_, _ = fmt.Fprintln(u.Out, "It cannot see the HTTP Host inside TLS or anything after that hello, a second hello after a HelloRetryRequest included, so fronting remains a limit, as does a hello the upstream reads differently, and it cannot stop data sent to an allowed service.")
	_, _ = fmt.Fprintln(u.Out, "Clients that omit SNI or send ECH, and SSH over 443, are refused.")
	_, _ = fmt.Fprintln(u.Out, "The proxy logs each refusal, but a stored log is no proof that every refusal was kept: a log that ends early reads as a complete one.")
}
