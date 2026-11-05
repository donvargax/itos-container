package yaml_test

import (
	"reflect"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/policy/port"
)

func TestDirectiveLinesAreSyntaxFailures(t *testing.T) {
	const directive = "%TAG !e! tag:example:\n---\n!e!value ignored"
	for name, input := range map[string]string{
		"start":                directive,
		"initial BOM":          "\ufeff" + directive,
		"repeated BOM":         "\ufeff\ufeff" + directive,
		"LF":                   "# comment\n" + directive,
		"CR":                   "# comment\r" + directive,
		"CRLF":                 "# comment\r\n" + directive,
		"NEL":                  "# comment\u0085" + directive,
		"LS":                   "# comment\u2028" + directive,
		"PS":                   "# comment\u2029" + directive,
		"BOM after line break": "# comment\n\ufeff" + directive,
		"second document":      "0\n...\n" + directive,
		"unused tag prefix":    "%TAG !e! tag:example:\n---\n0",
		"YAML version":         "%YAML 1.1\n---\n0",
		"quoted continuation":  "\"start\n%TAG is reserved here too\"",
	} {
		t.Run(name, func(t *testing.T) {
			value, issues := read(input)
			if !reflect.DeepEqual(issues, []port.Issue{{Kind: port.Syntax}}) {
				t.Fatalf("issues = %#v; want one root syntax refusal", issues)
			}
			if !reflect.DeepEqual(value, port.Value{}) {
				t.Fatal("directive refusal returned a partial value")
			}
		})
	}
}

func TestPercentTextRemainsLiteral(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
	}{
		{"plain", "config/100%.json", "config/100%.json"},
		{"single quoted", "'%TAG !e! literal'", "%TAG !e! literal"},
		{"double quoted", `"%YAML 1.1"`, "%YAML 1.1"},
		{"comment", "# %TAG !e! ignored\nvalue", "value"},
		{"inline comment", "value # %YAML ignored", "value"},
		{"block", "|-\n  %TAG !e! literal", "%TAG !e! literal"},
		{"folded block", ">-\n  %YAML literal", "%YAML literal"},
		{"indented continuation", "\"start\n  %TAG literal\"", "start %TAG literal"},
		{"escaped line break", `"start\n%TAG literal"`, "start\n%TAG literal"},
		{"BOM without directive", "\ufeffvalue", "value"},
		{"inline BOM", "\"before\ufeff%TAG literal\"", "before\ufeff%TAG literal"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, issues := read(tc.input)
			if len(issues) != 0 || value.Kind != port.String || value.Text != tc.want {
				t.Fatalf("read = %#v, %#v; want string %q", value, issues, tc.want)
			}
		})
	}
}
