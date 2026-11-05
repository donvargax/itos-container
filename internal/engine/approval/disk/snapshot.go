package disk

import (
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"os"
)

// SnapshotIO supplies file operations at the publication boundary. Nil operations
// select the real filesystem calls; callers cannot bypass record/path validation.
type SnapshotIO struct {
	Create        func(*os.Root, string) (*os.File, error)
	Write         func(*os.File, []byte) (int, error)
	FileSync      func(*os.File) error
	Rename        func(*os.Root, string, string) error
	ParentSync    func(*os.File) error
	DirectorySync func(*os.File) error
}

// Snapshots shares approval's protected state root and canonical record codec.
type Snapshots struct {
	Store Store
	IO    SnapshotIO
}

// Publish is unavailable until the snapshot implementation follows red evidence.
func (s Snapshots) Publish(port.Frozen) (port.Durable, port.Failure) {
	return port.Durable{}, "snapshot.write"
}
