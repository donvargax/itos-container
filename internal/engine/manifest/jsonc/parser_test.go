package jsonc_test

import (
	"github.com/donvargax/itos-container/internal/engine/manifest/jsonc"
	"github.com/donvargax/itos-container/internal/engine/manifest/port"
	"github.com/donvargax/itos-container/internal/engine/manifest/port/porttest"
	"testing"
)

func TestParserContract(t *testing.T) {
	porttest.ParserContract(t, func(porttest.ParserFixture) port.Parser { return jsonc.Parser{} })
}
