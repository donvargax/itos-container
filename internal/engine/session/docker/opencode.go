package docker

import (
	"maps"
	"slices"
	"strings"
)

// openCodeProfile is the opencode-v1 worker environment: the exact bytes the
// approval's profile binds, applied over any image default. Changing one value
// needs a new profile ID and a new approval.
var openCodeProfile = map[string]string{
	"OPENCODE_CONFIG_CONTENT":       `{"share":"disabled"}`,
	"OPENCODE_DISABLE_AUTOUPDATE":   "true",
	"OPENCODE_DISABLE_MODELS_FETCH": "true",
}

// openCodeEnvironment sets the profile's values over an environment list and
// returns it sorted, as the other worker environment builders do.
func openCodeEnvironment(environment []string) []string {
	values := map[string]string{}
	for _, value := range environment {
		key, v, _ := strings.Cut(value, "=")
		values[key] = v
	}
	maps.Copy(values, openCodeProfile)
	result := make([]string, 0, len(values))
	for _, key := range slices.Sorted(maps.Keys(values)) {
		result = append(result, key+"="+values[key])
	}
	return result
}
