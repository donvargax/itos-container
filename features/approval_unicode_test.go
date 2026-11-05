package features

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// Fixed adversarial vectors keep the expected visible escapes independent of the
// production renderer. Printable Unicode must remain readable alongside them.
func approvalUnicodeVector(kind string) (literal, escaped string, err error) {
	switch kind {
	case "C1":
		return "café 日本語 \u009b31m\u009d0;misleading title\u009c", `café 日本語 \u009b31m\u009d0;misleading title\u009c`, nil
	case "bidirectional":
		return "café 日本語 \u061c\u200e\u200f\u202a\u202b\u202c\u202d\u202e\u2066\u2067\u2068\u2069", `café 日本語 \u061c\u200e\u200f\u202a\u202b\u202c\u202d\u202e\u2066\u2067\u2068\u2069`, nil
	default:
		return "", "", fmt.Errorf("unknown Unicode fixture %q", kind)
	}
}

func (w *world) approvalUnicodeEnvironment(kind, source string) error {
	literal, _, err := approvalUnicodeVector(kind)
	if err != nil {
		return err
	}
	w.doctor.manifest["containerEnv"] = map[string]string{"APP": literal}
	if err := w.approvalManifest(); err != nil {
		return err
	}
	switch source {
	case "current":
	case "prior":
		if err := w.approvalSeed("1", false); err != nil {
			return err
		}
		literal = "current café 日本語"
		w.doctor.manifest["containerEnv"] = map[string]string{"APP": literal}
		if err := w.approvalManifest(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown Unicode source %q", source)
	}
	w.approval.literal = literal
	return w.approvalOracle()
}

func approvalUnicodeSafe(text string) error {
	for _, r := range text {
		if (unicode.IsControl(r) && r != '\r' && r != '\n') || unicode.Is(unicode.Cf, r) {
			return fmt.Errorf("raw terminal control U+%04X in approval output", r)
		}
	}
	return nil
}

func approvalDisplayedJSON(text string, want any) error {
	var got any
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		return fmt.Errorf("displayed settings are not JSON: %w", err)
	}
	if !bytes.Equal(approvalJSON(got), approvalJSON(want)) {
		return fmt.Errorf("displayed settings differ from independent fixture values")
	}
	return nil
}

func (w *world) approvalUnicodeReview(kind, source string) error {
	before, _, prompt := strings.Cut(w.approval.beforeReview, "Approve these settings? y/N: ")
	if !prompt || strings.Count(w.stdout, "y/N") != 1 {
		return fmt.Errorf("expected one complete review before consent; exit %d, review %q, stderr %q", w.exit, w.approval.beforeReview, w.stderr)
	}
	if err := approvalUnicodeSafe(w.stdout + w.stderr); err != nil {
		return err
	}
	_, escaped, err := approvalUnicodeVector(kind)
	if err != nil {
		return err
	}
	if !strings.Contains(before, escaped) {
		return fmt.Errorf("review lacks visible Unicode escapes before consent")
	}
	_, settings, found := strings.Cut(before, "Effective offline settings: ")
	if !found {
		return fmt.Errorf("missing effective settings before consent")
	}
	settings, _, _ = strings.Cut(settings, "\n")
	if err := approvalDisplayedJSON(settings, w.approval.settings); err != nil {
		return err
	}
	if source == "prior" {
		_, change, found := strings.Cut(before, "/manifest/containerEnv: ")
		if !found {
			return fmt.Errorf("missing environment difference before consent")
		}
		change, _, _ = strings.Cut(change, "\n")
		old, current, found := strings.Cut(change, " -> ")
		if !found {
			return fmt.Errorf("missing old-to-new values")
		}
		if err := approvalDisplayedJSON(old, approvalAt(w.approval.prior, "/manifest/containerEnv")); err != nil {
			return err
		}
		return approvalDisplayedJSON(current, approvalAt(w.approval.settings, "/manifest/containerEnv"))
	}
	return nil
}

func (w *world) approvalUnicodeStatus(kind string) error {
	if err := approvalUnicodeSafe(w.stdout + w.stderr); err != nil {
		return err
	}
	_, escaped, err := approvalUnicodeVector(kind)
	if err != nil {
		return err
	}
	if strings.Contains(w.stdout+w.stderr, "y/N") || !strings.Contains(w.stdout, escaped) || !strings.Contains(w.stdout, w.approval.fingerprint) {
		return fmt.Errorf("plain status must show escaped settings and the exact fingerprint without a prompt")
	}
	for _, line := range strings.Split(w.stdout, "\n") {
		if strings.HasPrefix(line, "{") {
			return approvalDisplayedJSON(line, w.approval.settings)
		}
	}
	return fmt.Errorf("plain status lacks complete effective settings")
}
