package features

import (
	"fmt"
	"strings"
)

// Build raw hostile input without invoking any product parser or validator.
func (w *world) manifestBudgetInput(source, budget string) error {
	var value string
	switch budget {
	case "bytes":
		value = "[]"
		if source == "manifest" {
			value = manifestBytes
		}
		value += strings.Repeat(" ", 65537-len(value))
	case "array depth", "unfinished depth":
		value = strings.Repeat("[", 33) + "0" + strings.Repeat("]", 33)
	case "object depth":
		value = strings.Repeat(`{"a":`, 33) + "0" + strings.Repeat("}", 33)
	case "array nodes":
		value = "[" + strings.Repeat("0,", 2048) + "0]"
	case "object nodes":
		var members []string
		for i := range 1024 {
			members = append(members, fmt.Sprintf(`"k%d":0`, i))
		}
		value = "{" + strings.Join(members, ",") + "}"
	case "paths":
		value = `{"` + strings.Repeat("~/", 500) + `":[` + strings.Repeat("0,", 255) + "0]}"
	default:
		return fmt.Errorf("unknown manifest budget %q", budget)
	}
	if budget != "bytes" {
		value = `{"customizations":{"vscode":{"settings":{"data":` + value + `}}}}`
		if source == "manifest" {
			value = `{"image":"` + manifestImage + `",` + value[1:]
		} else {
			value = "[" + value + "]"
		}
		if budget == "unfinished depth" {
			value, _, _ = strings.Cut(value, "0")
		}
	}
	if source == "manifest" {
		return w.manifestText(value)
	}
	w.manifest.label = &value
	return nil
}
