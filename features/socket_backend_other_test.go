//go:build !(linux && amd64)

package features

import "testing"

// The kernel answer is chosen at the system call of a Linux amd64 process. The
// package still builds elsewhere, for the manual macOS and Windows jobs.

const socketBackendSupported = false

func socketBackendChild() {}

func runSocketBackendFixture(*testing.T) {}
