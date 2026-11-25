package presentation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"io"
	"slices"
	"strings"
	"testing"
)

func TestReviewContract(t *testing.T) {
	porttest.ReviewContract(t, func(line string) port.Reviewer {
		return UI{In: strings.NewReader(line), Out: io.Discard, Err: io.Discard}
	})
}

func TestTerminalTextPreservesJSONValues(t *testing.T) {
	for _, tc := range []struct{ literal, escaped string }{
		{"café 日本語 😀 <>& \\\"", "café 日本語 😀 <>& \\\""},
		{"\u007f", `\u007f`},
		{"\u061c\u200e\u200f\u202a\u202b\u202c\u202d\u202e\u2066\u2067\u2068\u2069", `\u061c\u200e\u200f\u202a\u202b\u202c\u202d\u202e\u2066\u2067\u2068\u2069`},
		{"\u2028\u2029\U000e0001", `\u2028\u2029\udb40\udc01`},
	} {
		if got := terminalText(tc.literal); got != tc.escaped {
			t.Errorf("display %q = %q, want %q", tc.literal, got, tc.escaped)
		}
		encoded, err := json.Marshal(tc.literal)
		if err != nil {
			t.Fatal(err)
		}
		var decoded string
		if err := json.Unmarshal([]byte(terminalText(string(encoded))), &decoded); err != nil || decoded != tc.literal {
			t.Errorf("display changed JSON value %q: %q, %v", tc.literal, decoded, err)
		}
	}
	for r := rune(0x80); r <= 0x9f; r++ {
		if got, want := terminalText(string(r)), fmt.Sprintf(`\u%04x`, r); got != want {
			t.Errorf("C1 U+%04X = %q, want %q", r, got, want)
		}
	}
}

func TestReviewEscapesSettingsDifferencesAndNotices(t *testing.T) {
	var stdout bytes.Buffer
	ui := UI{In: strings.NewReader("yes\n"), Out: &stdout, Err: io.Discard}
	v := port.View{
		Inspection: porttest.Ready(),
		Candidate:  port.Candidate{Canonical: "{\"manifest\":{\"containerEnv\":{\"APP\":\"current\u009b\"}},\"\u202e\":\"new\"}"},
		Record:     port.Record{Candidate: port.Candidate{Canonical: "{\"manifest\":{\"containerEnv\":{\"APP\":\"prior\u009d\u009c\"}},\"\u202e\":\"old\"}"}, Generation: "1"},
	}
	v.Inspection.Manifest.Unused = []string{"editor\u202e"}
	v.Inspection.Manifest.Metadata.Unused = []string{"label\u2066"}
	if !ui.Review(v) {
		t.Fatal("complete yes declined")
	}
	text, _, found := strings.Cut(stdout.String(), "Approve these settings? y/N: ")
	if !found {
		t.Fatal("missing prompt")
	}
	for _, want := range []string{
		`"APP":"current\u009b"`,
		`/manifest/containerEnv: {"APP":"prior\u009d\u009c"} -> {"APP":"current\u009b"}`,
		`/\u202e: "old" -> "new"`,
		`Unused field: "editor\u202e"`, `Unused field: "label\u2066"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing escaped review fragment %q in %q", want, text)
		}
	}
	if strings.ContainsAny(text, "\u009b\u009d\u009c\u202e\u2066") {
		t.Errorf("raw controls in review %q", text)
	}
}

func TestPlainEscapesAndJSONPreservesCanonicalSettings(t *testing.T) {
	canonical := "{\"manifest\":{\"containerEnv\":{\"APP\":\"café\u009b\u202e\U000e0001\"}}}"
	v := port.View{Inspection: porttest.Ready(), Status: "matching", Candidate: port.Candidate{Canonical: canonical, Fingerprint: "sha256:fixture"}}
	var stdout bytes.Buffer
	ui := UI{Out: &stdout, Err: io.Discard}
	display := `{"manifest":{"containerEnv":{"APP":"café\u009b\u202e\udb40\udc01"}}}`
	if report := ui.Report(v); report.EffectiveText != display {
		t.Errorf("plain report settings %q, want %q", report.EffectiveText, display)
	}
	if code := ui.Print(v, false); code != 0 {
		t.Fatalf("plain status exit %d", code)
	}
	want := "approval matching\nfingerprint: sha256:fixture\n" + display + "\n"
	if stdout.String() != want {
		t.Errorf("plain status %q, want %q", stdout.String(), want)
	}
	stdout.Reset()
	if code := ui.Print(v, true); code != 0 {
		t.Fatalf("JSON status exit %d", code)
	}
	var report struct {
		Approval map[string]json.RawMessage `json:"approval"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if string(report.Approval["effective"]) != canonical || v.Candidate.Canonical != canonical {
		t.Fatal("display escaping changed JSON or canonical approval bytes")
	}
	if len(report.Approval) != 5 {
		t.Fatal("display-only settings leaked into the structured approval report")
	}
}
func TestReportsAndFailureRules(t *testing.T) {
	for _, tc := range []struct {
		failure port.Failure
		exit    int
	}{{"identity.root", 1}, {"identity.unavailable", 3}, {"identity.mismatch", 1}, {"approve.confirmation-required", 1}, {"approve.declined", 1}, {"approve.fingerprint", 1}, {"approve.fingerprint-format", 2}, {"approve.changed", 1}, {"approve.conflict", 1}, {"state.location", 2}, {"state.path", 1}, {"state.unsafe", 1}, {"state.invalid", 1}, {"state.unavailable", 3}, {"state.write", 75}} {
		var stdout, stderr bytes.Buffer
		ui := UI{In: strings.NewReader(""), Out: &stdout, Err: &stderr}
		v := port.View{Inspection: porttest.Ready(), Failure: tc.failure, Status: "invalid", Candidate: port.Candidate{Canonical: `{"manifest":{"containerEnv":{"x":"new"},"name":null}}`}, Record: port.Record{Candidate: port.Candidate{Canonical: `{"manifest":{"containerEnv":{"x":"old"},"name":"prior"}}`}, Generation: "1"}}
		if code := ui.Print(v, true); code != tc.exit {
			t.Fatal(tc, code)
		}
		var obj map[string]any
		if json.Unmarshal(stdout.Bytes(), &obj) != nil || obj["ok"] != false || obj["schema"] != float64(1) {
			t.Fatal(stdout.String())
		}
		r := ui.Report(v)
		if len(r.Differences) != 2 || r.Differences[0].Path != "/manifest/containerEnv" || r.Differences[1].Path != "/manifest/name" || r.Generation == nil {
			t.Fatal(r)
		}
		if ui.Terminal() || stderr.Len() == 0 {
			t.Fatal("invalid terminal or missing diagnostic")
		}
	}
}

func TestAllowlistReviewKeepsUnusedNoticesAfterDisclosureBeforeDifferences(t *testing.T) {
	var stdout bytes.Buffer
	ui := UI{In: strings.NewReader("yes\n"), Out: &stdout, Err: io.Discard}
	v := port.View{Inspection: porttest.Ready(), Candidate: port.Candidate{Canonical: `{"profile":"allowlist-v1","policy":{"network":{"allow":["a.example"]}},"manifest":{"name":"new"}}`}, Record: port.Record{Generation: "1", Candidate: port.Candidate{Canonical: `{"profile":"allowlist-v1","policy":{"network":{"allow":["b.example"]}},"manifest":{"name":"old"}}`}}}
	v.Inspection.Manifest.Unused = []string{"editor"}
	v.Inspection.Manifest.Metadata.Unused = []string{"label"}
	if !ui.Review(v) {
		t.Fatal("declined")
	}
	text := stdout.String()
	for _, part := range []string{"Effective settings:", "network.allow: a.example", "The proxy logs each refusal", `Unused field: "editor"`, `Unused field: "label"`, `/manifest/name: "old" -> "new"`, "/policy/network/allow:", "y/N"} {
		_, rest, ok := strings.Cut(text, part)
		if !ok {
			t.Fatalf("missing ordered %q in %q", part, stdout.String())
		}
		text = rest
	}
	if strings.Contains(stdout.String(), "network.allow: b.example") || strings.Contains(stdout.String(), "network none") {
		t.Fatal(stdout.String())
	}
}

func TestReviewExplainsHomeLifetimeBeforeConsent(t *testing.T) {
	var out bytes.Buffer
	ui := UI{In: strings.NewReader("n\n"), Out: &out, Err: io.Discard}
	v := port.View{Inspection: porttest.Ready(), Candidate: port.Candidate{Canonical: `{"profile":"offline-home-v1","home":{"storage":"volume"},"host":{"uid":"2001","gid":"3001"}}`}}
	if ui.Review(v) {
		t.Fatal("declined review consented")
	}
	before, _, found := strings.Cut(out.String(), "Approve these settings? y/N: ")
	for _, text := range []string{"/home/itos", "0700", "2001:3001", "retained after exit and stop", "hostile worker data", "terminal-consented reset or delete", "disabling home does not delete it"} {
		if !found || !strings.Contains(before, text) {
			t.Fatal("missing pre-consent disclosure", text, out.String())
		}
	}
}

func TestReviewDisclosesTheOpenCodeProfileBeforeConsent(t *testing.T) {
	for _, mode := range []string{"offline", "allowlist"} {
		var out bytes.Buffer
		ui := UI{In: strings.NewReader("n\n"), Out: &out, Err: io.Discard}
		canonical := `{"profile":"` + mode + `-home-v1","home":{"storage":"volume","harnesses":[{"name":"opencode","profile":"opencode-v1"}]},"host":{"uid":"2001","gid":"3001"},"policy":{"network":{"mode":"` + mode + `","allow":["provider.example"]}}}`
		v := port.View{Inspection: porttest.Ready(), Candidate: port.Candidate{Canonical: canonical}}
		if ui.Review(v) {
			t.Fatal("declined review consented")
		}
		before, _, found := strings.Cut(out.String(), "Approve these settings? y/N: ")
		texts := []string{"OpenCode harness: profile opencode-v1.\n", `OPENCODE_CONFIG_CONTENT={"share":"disabled"}` + "\n", "OPENCODE_DISABLE_AUTOUPDATE=true\n", "OPENCODE_DISABLE_MODELS_FETCH=true\n",
			"OpenCode global configuration, data and state live in /home/itos.",
			"The checkout's own opencode.json and .opencode configuration also apply, loaded before this profile, which overrides them where they conflict.",
			"Any managed configuration in the approved image (/etc/opencode) loads after this profile and overrides it.",
			"The image's other OPENCODE_* defaults stay in effect and can redirect or turn off configuration layers (OPENCODE_CONFIG, OPENCODE_CONFIG_DIR, which replaces the global directory, OPENCODE_DISABLE_PROJECT_CONFIG).", "The argv and the checkout choose the provider and model; run never does.", "No OpenCode login is delivered by this setting."}
		offline := "No provider is reachable: this policy is offline."
		reachable := "A provider is reachable only through the approved allowlist."
		if mode == "offline" {
			texts = append(texts, offline)
		} else {
			texts = append(texts, reachable)
		}
		for _, text := range texts {
			if !found || !strings.Contains(before, text) {
				t.Fatal(mode, "missing pre-consent disclosure", text, out.String())
			}
		}
		if strings.Contains(before, reachable) == (mode == "offline") || strings.Contains(before, offline) == (mode != "offline") {
			t.Fatal(mode, "wrong provider statement", before)
		}
	}
	// A home without the OpenCode harness, or with another harness, discloses no profile.
	for _, canonical := range []string{
		`{"profile":"offline-home-v1","home":{"storage":"volume"},"host":{"uid":"2001","gid":"3001"}}`,
		`{"profile":"offline-home-v1","home":{"storage":"volume","harnesses":[{"name":"codex","profile":"codex-v1"}]},"host":{"uid":"2001","gid":"3001"}}`,
	} {
		var out bytes.Buffer
		ui := UI{In: strings.NewReader("n\n"), Out: &out, Err: io.Discard}
		ui.Review(port.View{Inspection: porttest.Ready(), Candidate: port.Candidate{Canonical: canonical}})
		if strings.Contains(out.String(), "OpenCode") || strings.Contains(out.String(), "OPENCODE") {
			t.Fatal("an OpenCode disclosure without the OpenCode harness", out.String())
		}
	}
}

func TestReportShowsTheOpenCodeProfileForPlainDoctor(t *testing.T) {
	canonical := `{"profile":"offline-home-v1","home":{"storage":"volume","harnesses":[{"name":"opencode","profile":"opencode-v1"}]},"host":{"uid":"2001","gid":"3001"},"policy":{"network":{"mode":"offline"}}}`
	report := UI{}.Report(port.View{Inspection: porttest.Ready(), Candidate: port.Candidate{Canonical: canonical}})
	text := strings.Join(report.RequestedText, "\n")
	for _, want := range []string{"profile opencode-v1", `OPENCODE_CONFIG_CONTENT={"share":"disabled"}`, "OPENCODE_DISABLE_AUTOUPDATE=true", "OPENCODE_DISABLE_MODELS_FETCH=true", "No provider is reachable"} {
		if !strings.Contains(text, want) {
			t.Fatal("doctor omits", want, text)
		}
	}
	plain := UI{}.Report(port.View{Inspection: porttest.Ready(), Candidate: port.Candidate{Canonical: `{"profile":"offline-home-v1","home":{"storage":"volume"}}`}})
	if len(plain.RequestedText) != 0 {
		t.Fatal(plain.RequestedText)
	}
}

func TestCodexHarnessIsDisclosedBeforeConsentAndByPlainDoctor(t *testing.T) {
	want := []string{
		"Codex harness: seed codex-v1 at /home/itos/.codex.",
		"Each run sets CODEX_HOME=/home/itos/.codex and rewrites only config.toml there from the fixed codex-v1 seed.",
		"The seed turns off Codex update checks, analytics and plugins, and Codex's own sandbox; this container is the sandbox.",
		"Codex sessions, history and other files under /home/itos/.codex are hostile worker data kept for the next run; run does not read, reset or remove them.",
		"CODEX_HOME is reserved: the manifest's containerEnv cannot set it.",
		"No Codex login is delivered by this setting. The seed is no network or credential grant.",
	}
	canonical := `{"profile":"offline-home-v1","home":{"harness":{"kind":"codex","seed":"codex-v1","target":"/home/itos/.codex"},"harnesses":[{"name":"opencode","profile":"opencode-v1"}],"storage":"volume"},"host":{"uid":"2001","gid":"3001"},"policy":{"network":{"mode":"offline"}}}`
	var out bytes.Buffer
	ui := UI{In: strings.NewReader("n\n"), Out: &out, Err: io.Discard}
	ui.Review(port.View{Inspection: porttest.Ready(), Candidate: port.Candidate{Canonical: canonical}})
	lines := strings.Split(strings.SplitN(out.String(), "y/N", 2)[0], "\n")
	at := slices.Index(lines, want[0])
	if at < 3 || !strings.HasPrefix(lines[at-3], "Harness home:") || !slices.Equal(lines[at:at+len(want)], want) || !strings.HasPrefix(lines[at+len(want)], "OpenCode harness:") {
		t.Fatal("the review omits or misplaces the Codex disclosure", out.String())
	}
	report := UI{}.Report(port.View{Inspection: porttest.Ready(), Candidate: port.Candidate{Canonical: canonical}})
	if !slices.Equal(report.RequestedText[:len(want)], want) {
		t.Fatal("doctor omits the Codex disclosure", report.RequestedText)
	}
	// Without the harness nothing is said about Codex.
	plain := UI{}.Report(port.View{Inspection: porttest.Ready(), Candidate: port.Candidate{Canonical: `{"profile":"offline-home-v1","home":{"storage":"volume"}}`}})
	if len(plain.RequestedText) != 0 {
		t.Fatal(plain.RequestedText)
	}
}
