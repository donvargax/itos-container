package main

import (
	"context"
	"io/fs"
	"os"
	"testing"

	"github.com/alecthomas/kong"

	"github.com/donvargax/itos-container/internal/cli"
	canonical "github.com/donvargax/itos-container/internal/engine/approval/canonical"
	disk "github.com/donvargax/itos-container/internal/engine/approval/disk"
	approval "github.com/donvargax/itos-container/internal/engine/approval/domain"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	sessiondisk "github.com/donvargax/itos-container/internal/engine/session/disk"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
	"github.com/donvargax/itos-container/internal/slices/run/signals"
)

func localMountStat(root string) func(string) (fs.FileInfo, error) {
	return func(path string) (fs.FileInfo, error) {
		if path != "/proc/123/root/workspace" {
			return nil, fs.ErrNotExist
		}
		return os.Stat(root)
	}
}

// Local HTTP fixtures independently supply PID 123 and the fixture checkout as
// its mount. Only this test executable substitutes the otherwise real stat;
// native scenarios and the product executable use the actual process mount.
func TestLocalRunCommand(t *testing.T) {
	if os.Getenv("SLICE_17_LOCAL_RUN") != "1" {
		t.Skip("local HTTP scenario only")
	}
	var args []string
	for i, arg := range os.Args {
		if arg == "--" {
			args = os.Args[i+1:]
			break
		}
	}
	if len(args) < 2 || args[0] != "run" {
		t.Fatal("missing local run argv")
	}
	var c commandLine
	stopped := -1
	parser, err := kong.New(&c, append(cli.Flags(), kong.Name("itos-container"), kong.Writers(os.Stdout, os.Stderr), kong.Vars{"version": "fixture"}, kong.Exit(func(code int) { stopped = code }))...)
	if err != nil {
		t.Fatal(err)
	}
	ui := &cli.UI{In: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Classify: runcommand.Classify}
	_, err = parser.Parse(runOptions(args))
	if stopped >= 0 {
		os.Exit(stopped)
	}
	if err != nil {
		os.Exit(ui.Usage(commandName(args), err, wantsJSON(args)))
	}
	service, _ := approvalServices(os.Stdin, os.Stdout, os.Stderr)
	runner, _ := sessionServices(service)
	runner.Workspace = &sessiondisk.Workspace{MountStat: localMountStat(os.Getenv("SLICE_17_LOCAL_MOUNT_ROOT"))}
	ctx, release := signals.Capture(context.Background())
	c.Run.Approval = approval.Preflight{Service: service, Local: service.Readiness.(ap.StagedReadiness), Canonical: canonical.JSON{}}
	c.Run.Snapshots = disk.Snapshots{Store: service.Store.(disk.Store), Records: canonical.JSON{}}
	c.Run.Runner, c.Run.Context = runner, ctx
	c.Run.InheritedJSON = os.Getenv("ITOS_JSON") == "1"
	code := c.Run.Run(ui)
	release()
	os.Exit(code)
}
