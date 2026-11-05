package presentation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"io"
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
	if code := ui.Print(v, false); code != 0 {
		t.Fatalf("plain status exit %d", code)
	}
	want := "approval matching\nfingerprint: sha256:fixture\n" + `{"manifest":{"containerEnv":{"APP":"café\u009b\u202e\udb40\udc01"}}}` + "\n"
	if stdout.String() != want {
		t.Errorf("plain status %q, want %q", stdout.String(), want)
	}
	stdout.Reset()
	if code := ui.Print(v, true); code != 0 {
		t.Fatalf("JSON status exit %d", code)
	}
	var report struct {
		Approval struct {
			Effective json.RawMessage `json:"effective"`
		} `json:"approval"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if string(report.Approval.Effective) != canonical || v.Candidate.Canonical != canonical {
		t.Fatal("display escaping changed JSON or canonical approval bytes")
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
