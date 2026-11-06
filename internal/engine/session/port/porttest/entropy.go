package porttest

import "github.com/donvargax/itos-container/internal/engine/session/port"

// EntropySequence consumes independently supplied tokens and faults. Exhaustion
// models an unavailable source rather than silently reusing an earlier token.
type EntropySequence struct {
	Values []Entropy
}

func (e *EntropySequence) Token() (string, port.Outcome) {
	if len(e.Values) == 0 {
		return "", port.IdentityFailed
	}
	next := e.Values[0]
	e.Values = e.Values[1:]
	return next.Token()
}
