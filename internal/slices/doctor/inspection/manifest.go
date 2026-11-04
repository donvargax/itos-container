package inspection

import (
	"github.com/donvargax/itos-container/internal/engine/manifest/domain"
	"github.com/donvargax/itos-container/internal/engine/manifest/port"
)

// Manifests connects doctor's request to the reusable file validator.
type Manifests struct {
	Files      port.Files
	Parser     port.Parser
	References port.References
}

// Inspect reads only the policy-selected file and any requested workspace directory.
func (m Manifests) Inspect(path string) port.Result {
	return domain.Inspect(path, m.Files, m.Parser, m.References)
}
