package domain

import (
	"strings"

	"github.com/donvargax/itos-container/internal/engine/readiness/port"
)

// ProxyExecutable judges one observation of the controller's own program for
// delivery to the egress proxy. root is the pinned checkout's resolved root, as
// a value. Only a regular static Linux amd64 ELF whose resolved path lies
// outside that root is admitted: the proxy's inert image supplies no loader or
// library, and a file the checkout can write is not the program that was
// approved. An observation that could not be made is refused like any other,
// never read as safe, and so is a path or root that cannot place the file.
func ProxyExecutable(observed port.Executable, failure port.Failure, root string) port.Failure {
	switch {
	case failure != port.None:
	case !observed.Regular:
	case observed.OS != "linux" || observed.Architecture != "amd64":
	case observed.Interpreter || observed.Dynamic:
	case !resolved(observed.Path) || !resolved(root):
	case within(observed.Path, root):
	default:
		return port.None
	}
	return port.EgressExecutable
}

// resolved is whether a path is absolute and already in its shortest form: no
// empty, "." or ".." element, so a prefix comparison cannot be led elsewhere.
// A single trailing separator is allowed, as a root may be written with one.
func resolved(path string) bool {
	if !strings.HasPrefix(path, "/") {
		return false
	}
	trimmed := strings.TrimSuffix(path[1:], "/")
	if trimmed == "" {
		return true
	}
	for _, element := range strings.Split(trimmed, "/") {
		if element == "" || element == "." || element == ".." {
			return false
		}
	}
	return true
}

// within is whether path is root or lies below it. The filesystem root holds
// every path.
func within(path, root string) bool {
	root = strings.TrimSuffix(root, "/")
	return path == root || strings.HasPrefix(path, root+"/")
}
