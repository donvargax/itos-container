package main

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/donvargax/itos-container/internal/cli"
	canonical "github.com/donvargax/itos-container/internal/engine/approval/canonical"
	approvaldisk "github.com/donvargax/itos-container/internal/engine/approval/disk"
	approval "github.com/donvargax/itos-container/internal/engine/approval/domain"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	sessiondisk "github.com/donvargax/itos-container/internal/engine/session/disk"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
)

// Only the standalone test assembly can enter this function. Every service and
// the log writer are real; one selected underlying log operation returns an I/O
// failure. The parent observes the receipt, bytes, API requests and fresh CLI
// processes. No product option or environment seam selects these faults.
func egressLifecycleFaultCommand(args []string, in io.Reader, out, errout io.Writer, fault string) int {
	var c commandLine
	parser, err := kong.New(&c, append(cli.Flags(), kong.Writers(out, errout), kong.Vars{"version": "slice-28-fixture"})...)
	if err != nil {
		return 96
	}
	parsed, err := parser.Parse(runOptions(args))
	if err != nil {
		return 96
	}
	name, _, _ := strings.Cut(parsed.Command(), " ")
	if name != "run" {
		return 96
	}
	service, _ := approvalServices(name, in, out, errout)
	runner, _ := sessionServices(service)
	log := runner.Refusals.(sessiondisk.RefusalLog)
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
	runner.Refusals = log
	c.Run.Approval = approval.Preflight{Service: service, Local: service.Readiness.(ap.StagedReadiness), Canonical: canonical.JSON{}}
	c.Run.Snapshots = approvaldisk.Snapshots{Store: service.Store.(approvaldisk.Store), Records: canonical.JSON{}}
	c.Run.Runner = runner
	c.Run.InheritedJSON = os.Getenv("ITOS_JSON") == "1"
	return c.Run.Run(&cli.UI{In: in, Stdout: out, Stderr: errout, Classify: runcommand.Classify})
}
