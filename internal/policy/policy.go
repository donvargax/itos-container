// Package policy exposes the shared policy engine to command adapters.
package policy

import (
	"github.com/donvargax/itos-container/internal/policy/domain"
	"github.com/donvargax/itos-container/internal/policy/port"
)

// Inspect validates a request without approving or executing anything.
func Inspect(source port.Source) port.Result { return domain.Inspect(source) }
