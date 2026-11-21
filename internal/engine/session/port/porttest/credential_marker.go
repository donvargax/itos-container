package porttest

import (
	"bytes"
	"encoding/base64"
)

// CredentialMarkerPresent searches raw bytes and base64 at all three offsets.
// Callers seed both detector forms before searching their real output surfaces.
func CredentialMarkerPresent(surface, marker []byte) bool {
	if bytes.Contains(surface, marker) {
		return true
	}
	for offset := 0; offset < 3; offset++ {
		encoded := base64.StdEncoding.EncodeToString(append(make([]byte, offset), marker...))
		start := (offset*8 + 5) / 6
		end := (offset + len(marker)) * 8 / 6
		if bytes.Contains(surface, []byte(encoded[start:end])) {
			return true
		}
	}
	return false
}
