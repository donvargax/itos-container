package porttest

import (
	"crypto/sha256"
	"fmt"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// HomeCodec is a table of independent values, not a wire-format parser.
type HomeCodec struct {
	Records map[string]port.HomeScope
	Failure port.Outcome
}

func (*HomeCodec) Scope(b port.Binding) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%q", []string{b.Frozen.Project, b.UID, b.GID, b.Endpoint, b.EngineID}))))
}
func (c *HomeCodec) EncodeHome(s port.HomeScope) []byte {
	if c.Records == nil {
		c.Records = map[string]port.HomeScope{}
	}
	key := fmt.Sprintf("%#v", s)
	c.Records[key] = cloneHome(s)
	return []byte(key)
}
func (c *HomeCodec) DecodeHome(data []byte) (port.HomeScope, port.Outcome) {
	if c.Failure != "" {
		return port.HomeScope{}, c.Failure
	}
	s, ok := c.Records[string(data)]
	if !ok {
		return port.HomeScope{}, port.Invalid
	}
	return cloneHome(s), ""
}
