// Package entropy allocates unpredictable operation tokens without ambient input.
package entropy

import (
	"crypto/rand"
	"encoding/hex"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

type Random struct{ Read func([]byte) (int, error) }

func (r Random) Token() (string, port.Outcome) {
	read := r.Read
	if read == nil {
		read = rand.Read
	}
	var data [32]byte
	n, err := read(data[:])
	if err != nil || n != len(data) {
		return "", port.IdentityFailed
	}
	return hex.EncodeToString(data[:]), ""
}

// StagingToken is independent of the 256-bit session and intent identities.
func (r Random) StagingToken() (string, port.Outcome) {
	read := r.Read
	if read == nil {
		read = rand.Read
	}
	var data [16]byte
	n, err := read(data[:])
	if err != nil || n != len(data) {
		return "", port.IdentityFailed
	}
	return hex.EncodeToString(data[:]), ""
}
