package disk

import (
	"io/fs"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// Walk the complete pinned checkout, including .git and ignored directories.
// WalkDir does not follow symlinks; os.Root confines opens if an entry changes
// during traversal. This is only a startup observation: sockets created or
// swapped in after inspection remain reachable through the writable bind.
func checkoutSockets(root fs.FS) port.Outcome {
	var failure port.Outcome
	err := fs.WalkDir(root, ".", func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&fs.ModeSocket != 0 {
			failure = port.IsolationUnverified
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return port.IsolationInspection
	}
	return failure
}
