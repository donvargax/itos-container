// Package port defines doctor's inspection capabilities.
package port

import policy "github.com/donvargax/itos-container/internal/policy/port"

// Policies inspects the effective request without changing host or checkout state.
type Policies interface{ Inspect() policy.Result }
