package features

import "testing"

// TestSocketBackendFixture holds the socket-backend fixture (T-66): the answer
// the kernel gives the Landlock version query and the runtimes the fixture
// daemon reports. It needs a Linux amd64 host that lets a process install a
// seccomp filter with a listener, so it never skips there, and a host whose
// processes already sit under a live listener fails it.
func TestSocketBackendFixture(t *testing.T) {
	if !socketBackendSupported {
		t.Skip("the Landlock answer is chosen at the system call of a Linux amd64 process")
	}
	runSocketBackendFixture(t)
}
