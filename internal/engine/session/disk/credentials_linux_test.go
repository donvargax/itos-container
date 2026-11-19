package disk

import "golang.org/x/sys/unix"

// Compile-time comparisons also check cross-compiled tests, so an arm64 build
// catches amd64 flag values without needing to execute an arm64 test binary.
var (
	_ [credentialNoFollow]struct{}  = [unix.O_NOFOLLOW]struct{}{}
	_ [credentialDirectory]struct{} = [unix.O_DIRECTORY]struct{}{}
)
