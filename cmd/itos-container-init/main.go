// Command itos-container-init is the trusted worker init. It restricts itself
// with Landlock so the workload cannot reach a pathname socket created outside
// its domain, then replaces itself with the literal workload argv:
//
//	itos-container-init -- <argv...>
//
// It takes no flag, prints no help or version, reads nothing but PATH for the
// lookup, and prints nothing when it succeeds.
package main

import (
	"os"
	"runtime"

	"github.com/donvargax/itos-container/internal/slices/workerinit/command"
	"github.com/donvargax/itos-container/internal/slices/workerinit/landlock"
	"github.com/donvargax/itos-container/internal/slices/workerinit/process"
)

func main() {
	// A Landlock domain and no_new_privs belong to one thread, and only the
	// thread that executes hands them to the workload. Everything below runs
	// on this one, and starts no goroutine.
	runtime.LockOSThread()
	handler := command.Init{Landlock: landlock.New(), Process: process.New()}
	os.Exit(handler.Run(os.Args[1:], os.Stderr))
}
