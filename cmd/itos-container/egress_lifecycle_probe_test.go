package main

import (
	"errors"
	"os"

	approvaldisk "github.com/donvargax/itos-container/internal/engine/approval/disk"
	sessiondisk "github.com/donvargax/itos-container/internal/engine/session/disk"
)

// Only the local HTTP test assembly selects these underlying storage failures.
// The real writer and protected filesystem still perform every other operation.
func egressLifecycleFaultLog(log sessiondisk.RefusalLog, fault string) sessiondisk.RefusalLog {
	storage := log.Storage.(approvaldisk.Registry)
	broken := errors.New("slice-28 injected log storage failure")
	fail := func(operation string) error {
		if fault != operation {
			return nil
		}
		if err := os.WriteFile(os.Getenv("SLICE_28_STORAGE_RECEIPT"), []byte(operation), 0600); err != nil {
			return err
		}
		return broken
	}
	storage.IO = approvaldisk.SnapshotIO{
		Create: func(root *os.Root, name string) (*os.File, error) {
			if err := fail("begin"); err != nil {
				return nil, err
			}
			return root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		},
		Write: func(file *os.File, p []byte) (int, error) {
			if err := fail("write"); err != nil {
				return 0, err
			}
			return file.Write(p)
		},
		FileSync: func(file *os.File) error {
			if err := fail("file fsync"); err != nil {
				return err
			}
			return file.Sync()
		},
		Rename: func(root *os.Root, from, to string) error {
			if err := fail("rename"); err != nil {
				return err
			}
			return root.Rename(from, to)
		},
		ParentSync: func(file *os.File) error {
			if err := fail("parent fsync"); err != nil {
				return err
			}
			return file.Sync()
		},
	}
	log.Storage = storage
	return log
}
