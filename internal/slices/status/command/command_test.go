package command_test

import (
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	fake "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/donvargax/itos-container/internal/slices/status/command"
)

func TestHomeStatusArgumentsRefuseBeforeDiscovery(t *testing.T) {
	for _, json := range []bool{false, true} {
		output := &fake.HomePresentation{}
		// No service is installed: an invalid invocation has no authority to
		// discover protected state or the engine.
		c := command.CLI{Homes: true, JSON: json, Args: []string{"unexpected"}, HomePresentation: output}
		if code := c.Run(); code != 2 || len(output.Result.Failures) != 1 || output.Result.Failures[0].Outcome != port.Usage || output.JSON != json {
			t.Fatal(code, output)
		}
	}
}
