package command

import (
	"errors"
	"fmt"
	"github.com/donvargax/itos-container/internal/cli"
	"github.com/donvargax/itos-container/internal/slices/count/port"
)

// Classify maps count's sealed read failures to their existing CLI contract.
// Unknown errors return zero and nil so the shared UI supplies its fallback.
func Classify(err error) (int, []cli.Problem) {
	var failure port.ReadFailure
	if errors.As(err, &failure) {
		switch e := failure.(type) {
		case *port.Missing:
			return cli.CodeEnvironment, []cli.Problem{{Rule: "COUNT_INPUT_MISSING",
				Message: fmt.Sprintf("the file %s is not there", e.Path),
				Fix:     "check the path, or give - to count standard input"}}
		case *port.Unreadable:
			return cli.CodeEnvironment, []cli.Problem{{Rule: "COUNT_INPUT_UNREADABLE",
				Message: fmt.Sprintf("the file %s cannot be read as text", e.Path),
				Fix:     "give a file this program can read, not a folder"}}
		}
	}
	return 0, nil
}
