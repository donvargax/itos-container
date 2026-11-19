package main

import (
	"context"
	"io/fs"
	"os"
	"runtime"
	"testing"

	"github.com/alecthomas/kong"

	"github.com/donvargax/itos-container/internal/cli"
	canonical "github.com/donvargax/itos-container/internal/engine/approval/canonical"
	disk "github.com/donvargax/itos-container/internal/engine/approval/disk"
	approval "github.com/donvargax/itos-container/internal/engine/approval/domain"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	sessiondisk "github.com/donvargax/itos-container/internal/engine/session/disk"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	terminalnative "github.com/donvargax/itos-container/internal/engine/terminal/native"
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

func localProcessOpen(pid int) (sp.Process, sp.Outcome) {
	if pid != 123 {
		return nil, sp.MountRefused
	}
	return &sf.Process{}, ""
}

// Local HTTP fixtures independently supply PID 123 and the fixture checkout as
// its mount. Only this test executable substitutes pidfd acquisition and stat;
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
	in, out := os.Stdin, os.Stdout
	if alias := os.Getenv("SLICE_37_TERMINAL_ALIAS"); alias != "" {
		file, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = file.Close() }()
		if alias == "stdin" {
			in = file
		} else {
			out = file
		}
	}
	ui := &cli.UI{In: in, Stdout: out, Stderr: os.Stderr, Classify: runcommand.Classify}
	_, err = parser.Parse(runOptions(args))
	if stopped >= 0 {
		os.Exit(stopped)
	}
	if err != nil {
		os.Exit(ui.Usage(commandName(args), err, wantsJSON(args)))
	}
	service, _ := approvalServices("run", os.Stdin, os.Stdout, os.Stderr)
	runner, _ := sessionServices(service)
	runner.Workspace = &sessiondisk.Workspace{ProcessOpen: localProcessOpen, MountStat: localMountStat(os.Getenv("SLICE_17_LOCAL_MOUNT_ROOT"))}
	ctx, release := signals.Capture(context.Background(), c.Run.TTY)
	c.Run.Approval = approval.Preflight{Service: service, Local: service.Readiness.(ap.StagedReadiness), Canonical: canonical.JSON{}}
	c.Run.Snapshots = disk.Snapshots{Store: service.Store.(disk.Store), Records: canonical.JSON{}}
	c.Run.Runner, c.Run.Context = runner, ctx
	c.Run.InheritedJSON = os.Getenv("ITOS_JSON") == "1"
	c.Run.Terminal = terminalnative.NewPair(in, out, runtime.GOOS)
	code := c.Run.Run(ui)
	release()
	os.Exit(code)
}
