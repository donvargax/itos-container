// Package command is the worker init's handler: it checks the command line,
// hands the workload argv to the domain, and writes the one line a refusal
// owes. It parses no flag and prints no help or version.
package command

import (
	"fmt"
	"io"

	"github.com/donvargax/itos-container/internal/slices/workerinit/domain"
	"github.com/donvargax/itos-container/internal/slices/workerinit/port"
)

// Init is the handler, with the two ports main bound.
type Init struct {
	Landlock port.Landlock
	Process  port.Process
}

// usage is the refusal of a command line that is not `-- <argv...>`.
const usage = "usage"

// exitCodes follow Docker's own: 125 for the init's errors, 126 for a command
// it cannot invoke and 127 for a command not found.
var exitCodes = map[domain.Code]int{
	domain.Unavailable: 125,
	domain.Failed:      125,
	domain.NotFound:    127,
	domain.ExecFailed:  126,
}

// Run starts the workload named by args, which must be `--` and then at least
// one more argument; everything after that first `--` is the workload argv and
// is never interpreted. It returns only a refusal's exit code, after writing
// its one line to stderr: a started workload replaces the process.
func (i Init) Run(args []string, stderr io.Writer) int {
	if len(args) < 2 || args[0] != "--" {
		return refuse(stderr, usage, 125)
	}
	code := domain.Start(i.Landlock, i.Process, args[1:])
	return refuse(stderr, string(code), exitCodes[code])
}

// refuse writes the refusal's single line and returns its exit code. Nothing
// from argv, the environment or an error is in it.
func refuse(stderr io.Writer, code string, exit int) int {
	_, _ = fmt.Fprintf(stderr, "itos-container-init: %s\n", code)
	return exit
}
