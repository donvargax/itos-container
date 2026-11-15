package domain

import (
	"slices"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// Differences compares decoded JSON values. Maps recurse except containerEnv;
// arrays (including the allowlist) are one value. Paths are sorted JSON pointers.
func Differences(before, after any) []port.Change {
	out := []port.Change{}
	differences("", before, after, &out)
	slices.SortFunc(out, func(a, b port.Change) int { return strings.Compare(a.Path, b.Path) })
	return out
}

func differences(path string, before, after any, out *[]port.Change) {
	if equalJSON(before, after) {
		return
	}
	b, bok := before.(map[string]any)
	a, aok := after.(map[string]any)
	if bok && aok && path != "/manifest/containerEnv" {
		keys := map[string]bool{}
		for key := range b {
			keys[key] = true
		}
		for key := range a {
			keys[key] = true
		}
		for key := range keys {
			escaped := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
			differences(path+"/"+escaped, b[key], a[key], out)
		}
		return
	}
	*out = append(*out, port.Change{Path: path, Before: before, After: after})
}

func equalJSON(before, after any) bool {
	switch b := before.(type) {
	case nil:
		return after == nil
	case string:
		a, ok := after.(string)
		return ok && a == b
	case float64:
		a, ok := after.(float64)
		return ok && a == b
	case bool:
		a, ok := after.(bool)
		return ok && a == b
	case []any:
		a, ok := after.([]any)
		if !ok || len(a) != len(b) {
			return false
		}
		for i, value := range b {
			if !equalJSON(value, a[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		a, ok := after.(map[string]any)
		if !ok || len(a) != len(b) {
			return false
		}
		for key, value := range b {
			other, ok := a[key]
			if !ok || !equalJSON(value, other) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
