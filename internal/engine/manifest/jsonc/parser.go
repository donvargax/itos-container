// Package jsonc adapts the reviewed HuJSON syntax tree without losing duplicate
// members to map decoding. Parser errors never escape as source-bearing text.
package jsonc

import (
	"encoding/json"
	"slices"
	"strconv"

	"github.com/donvargax/itos-container/internal/engine/manifest/port"
	"github.com/tailscale/hujson"
)

// Parser supports JSON comments and trailing commas, or strict JSON for labels.
type Parser struct{}

// Parse checks the complete representation before returning domain values.
func (Parser) Parse(data []byte, strict bool) (port.Value, []port.Issue) {
	if strict && !json.Valid(data) {
		return port.Value{}, []port.Issue{{Kind: port.Syntax}}
	}
	tree, err := hujson.Parse(data)
	if err != nil {
		return port.Value{}, []port.Issue{{Kind: port.Syntax}}
	}
	value, issues := convert(tree, nil)
	if len(issues) != 0 {
		return port.Value{}, issues
	}
	return value, nil
}

func convert(tree hujson.Value, path []string) (port.Value, []port.Issue) {
	var value port.Value
	var issues []port.Issue
	switch item := tree.Value.(type) {
	case *hujson.Object:
		value.Kind, value.Fields = port.Object, map[string]port.Value{}
		for _, member := range item.Members {
			key := member.Name.Value.(hujson.Literal).String()
			at := append(slices.Clone(path), key)
			if _, exists := value.Fields[key]; exists {
				issues = append(issues, port.Issue{Kind: port.Duplicate, Path: at})
			}
			child, problems := convert(member.Value, at)
			value.Fields[key] = child
			issues = append(issues, problems...)
		}
	case *hujson.Array:
		value.Kind, value.Elements = port.Array, make([]port.Value, 0, len(item.Elements))
		for i, element := range item.Elements {
			child, problems := convert(element, append(slices.Clone(path), strconv.Itoa(i)))
			value.Elements = append(value.Elements, child)
			issues = append(issues, problems...)
		}
	case hujson.Literal:
		switch item.Kind() {
		case '"':
			value.Kind, value.Text = port.String, item.String()
		case '0':
			value.Kind, value.Text = port.Number, string(item)
		case 't', 'f':
			value.Kind, value.Text = port.Boolean, string(item)
		case 'n':
			value.Kind = port.Null
		}
	}
	return value, issues
}
