package disk

import (
	"bytes"
	"encoding/json"
)

// Encoding/json supplies syntax and types; require explicit fields and unique
// keys as well, so a truncated or ambiguous binding cannot become a pending claim.
// It runs only after Decode accepted data as one JSON object (or null), so the
// token and unmarshal calls below cannot fail on it.
func bindingShape(data []byte) bool {
	if !uniqueJSONValue(json.NewDecoder(bytes.NewReader(data))) {
		return false
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(data, &fields)
	if len(fields) != 5 {
		return false
	}
	for _, name := range []string{"schema", "uid", "path", "complete", "ancestors"} {
		if fields[name] == nil {
			return false
		}
	}
	return bytes.Equal(bytes.TrimSpace(fields["complete"]), []byte("true")) || bytes.Equal(bytes.TrimSpace(fields["complete"]), []byte("false"))
}

// uniqueJSONValue reports whether no object in the value repeats a key. Every
// token read is well formed and every key a string, so only duplicates remain.
func uniqueJSONValue(d *json.Decoder) bool {
	token, _ := d.Token()
	switch token {
	case json.Delim('{'):
		seen := map[string]bool{}
		for d.More() {
			key, _ := d.Token()
			name, _ := key.(string)
			if seen[name] {
				return false
			}
			seen[name] = true
			if !uniqueJSONValue(d) {
				return false
			}
		}
		_, _ = d.Token()
	case json.Delim('['):
		for d.More() {
			if !uniqueJSONValue(d) {
				return false
			}
		}
		_, _ = d.Token()
	}
	return true
}
