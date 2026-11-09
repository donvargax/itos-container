package disk

import (
	"bytes"
	"encoding/json"
)

// Encoding/json supplies syntax and types; require explicit fields and unique
// keys as well, so a truncated or ambiguous binding cannot become a pending claim.
func bindingShape(data []byte) bool {
	if !uniqueJSONValue(json.NewDecoder(bytes.NewReader(data))) {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || len(fields) != 5 {
		return false
	}
	for _, name := range []string{"schema", "uid", "path", "complete", "ancestors"} {
		if fields[name] == nil {
			return false
		}
	}
	return bytes.Equal(bytes.TrimSpace(fields["complete"]), []byte("true")) || bytes.Equal(bytes.TrimSpace(fields["complete"]), []byte("false"))
}

// uniqueJSONValue runs only after Decode accepted data as one valid JSON value, so
// Token cannot fail on it; duplicate keys are all that remains to detect.
func uniqueJSONValue(d *json.Decoder) bool {
	token, _ := d.Token()
	switch token {
	case json.Delim('{'):
		seen := map[string]bool{}
		for d.More() {
			key, _ := d.Token()
			name, ok := key.(string)
			if !ok || seen[name] {
				return false
			}
			seen[name] = true
			if !uniqueJSONValue(d) {
				return false
			}
		}
		end, err := d.Token()
		return err == nil && end == json.Delim('}')
	case json.Delim('['):
		for d.More() {
			if !uniqueJSONValue(d) {
				return false
			}
		}
		end, err := d.Token()
		return err == nil && end == json.Delim(']')
	default:
		return true
	}
}
