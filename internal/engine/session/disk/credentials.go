package disk

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// Credentials stages token copies only on the host's tmpfs. Test observation
// hooks are private; production has no alternative staging root.
type Credentials struct {
	OS    string
	root  string
	tmpfs func(string) bool
}

func (Credentials) Stage(context.Context, port.Record, port.CredentialSources, func(port.CredentialStaging) port.Outcome) port.Outcome {
	return port.CredentialUnavailable
}
func (Credentials) Observe(context.Context, port.Record) (string, port.Outcome) {
	return "unknown", port.CredentialUnavailable
}
func (Credentials) Remove(context.Context, port.Record) port.Outcome {
	return port.CredentialUnavailable
}
