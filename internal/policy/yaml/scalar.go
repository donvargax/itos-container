package yaml

import (
	"math/big"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/donvargax/itos-container/internal/policy/port"
)

// YAML v3 also resolves YAML 1.1 numbers and timestamps. These YAML 1.2 core
// scalar patterns supplement its parser; nodes still own all YAML syntax.
// Core signs apply only to decimal integers: yaml.org/spec/1.2.2/#1032-tag-resolution.
var (
	integer   = regexp.MustCompile(`^(?:[+-]?[0-9]+|0o[0-7]+|0x[0-9a-fA-F]+)$`)
	decimal   = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)
	nonfinite = regexp.MustCompile(`^(?:[+-]?\.(?:inf|Inf|INF)|\.(?:nan|NaN|NAN))$`)
)

func scalar(node *yaml.Node) (port.Value, port.Failure) {
	text := node.Value
	tag := ""
	if node.Style&yaml.TaggedStyle != 0 {
		tag = node.Tag
	} else if node.Style&(yaml.DoubleQuotedStyle|yaml.SingleQuotedStyle|yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		tag = "!!str"
	}
	if tag == "!!str" {
		return port.Value{Kind: port.String, Text: text}, port.None
	}
	value := port.Value{Kind: port.String, Text: text}
	inferred := "!!str"
	switch {
	case text == "" || text == "~" || text == "null" || text == "Null" || text == "NULL":
		value = port.Value{Kind: port.Null}
		inferred = "!!null"
	case text == "true" || text == "True" || text == "TRUE" || text == "false" || text == "False" || text == "FALSE":
		value = port.Value{Kind: port.Boolean}
		inferred = "!!bool"
	case nonfinite.MatchString(text):
		if tag == "" || tag == "!!float" {
			return port.Value{}, port.NonFinite
		}
	case integer.MatchString(text):
		n := new(big.Int)
		base := 10
		unsigned := strings.TrimPrefix(strings.TrimPrefix(text, "-"), "+")
		if strings.HasPrefix(unsigned, "0x") || strings.HasPrefix(unsigned, "0o") {
			base = 0
		}
		n.SetString(text, base)
		value = port.Value{Kind: port.Number, Number: new(big.Rat).SetInt(n)}
		inferred = "!!int"
	case decimal.MatchString(text):
		n, ok := new(big.Rat).SetString(text)
		if !ok {
			return port.Value{}, port.Syntax
		}
		value = port.Value{Kind: port.Number, Number: n}
		inferred = "!!float"
	}
	switch tag {
	case "", inferred:
		return value, port.None
	case "!!float":
		if inferred == "!!int" && decimal.MatchString(text) {
			return value, port.None
		}
	case "!!int", "!!bool", "!!null", "!!map", "!!seq":
	default:
		// The tree reports custom tags while continuing through their children.
		return value, port.None
	}
	return port.Value{}, port.Syntax
}
