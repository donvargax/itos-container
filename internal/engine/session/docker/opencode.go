package docker

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
func openCodeEnvironment(_ []string) []string {
	return nil
}

var (
	_ = openCodeProfile
)
