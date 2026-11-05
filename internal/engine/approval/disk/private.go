package disk

import (
	"os"
	"strconv"
)

// The procfs descriptor link addresses the pinned inode even if its directory
// entry moves. It works with O_PATH and an unsearchable mode without requiring
// fchmodat2/AT_EMPTY_PATH, which older Linux kernels do not implement. The caller
// keeps the descriptor open throughout and verifies the entry before and after.
func chmodPrivate(fd uintptr) error {
	return os.Chmod("/proc/self/fd/"+strconv.FormatUint(uint64(fd), 10), 0700) //nolint:gosec // G302: the verified directory needs owner search permission.
}
