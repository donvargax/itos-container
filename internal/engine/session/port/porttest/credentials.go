package porttest

import (
	"context"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// CredentialStager models pathnames independently of acquired runtime references.
type CredentialStager struct {
	SourceData  map[string][]byte
	Files       map[string][]byte
	Directories map[string]port.CredentialStaging
	Fault       string
	Events      []string
	Before      func(string)
}

func (*CredentialStager) Stage(context.Context, port.Record, port.CredentialSources, func(port.CredentialStaging) port.Outcome) port.Outcome {
	return port.CredentialUnavailable
}
func (*CredentialStager) Observe(context.Context, port.Record) (string, port.Outcome) {
	return "unknown", port.CredentialUnavailable
}
func (*CredentialStager) Remove(context.Context, port.Record) port.Outcome {
	return port.CredentialUnavailable
}
