package entropy

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestTokenUsesAll32BytesAndRefusesEntropyFailure(t *testing.T) {
	source := bytes.NewReader(bytes.Repeat([]byte{0xab}, 32))
	token, f := (Random{Read: source.Read}).Token()
	if f != "" || token != strings.Repeat("ab", 32) {
		t.Fatal(token, f)
	}
	for _, read := range []func([]byte) (int, error){func([]byte) (int, error) { return 0, errors.New("unavailable") }, func([]byte) (int, error) { return 31, nil }} {
		if token, f = (Random{Read: read}).Token(); token != "" || f != port.IdentityFailed {
			t.Fatal(token, f)
		}
	}
	a, f := (Random{}).Token()
	b, g := (Random{}).Token()
	if f != "" || g != "" || !port.HexID(a) || !port.HexID(b) || a == b {
		t.Fatal("independent entropy unavailable")
	}
}
