package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/cli"
	canonical "github.com/donvargax/itos-container/internal/engine/approval/canonical"
	disk "github.com/donvargax/itos-container/internal/engine/approval/disk"
	approval "github.com/donvargax/itos-container/internal/engine/approval/domain"
	host "github.com/donvargax/itos-container/internal/engine/approval/host"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/manifest/jsonc"
	readinessdisk "github.com/donvargax/itos-container/internal/engine/readiness/disk"
	readinessdocker "github.com/donvargax/itos-container/internal/engine/readiness/docker"
	codec "github.com/donvargax/itos-container/internal/engine/session/codec"
	sessiondisk "github.com/donvargax/itos-container/internal/engine/session/disk"
	docker "github.com/donvargax/itos-container/internal/engine/session/docker"
	session "github.com/donvargax/itos-container/internal/engine/session/domain"
	"github.com/donvargax/itos-container/internal/engine/session/entropy"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/inspection"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
	"github.com/donvargax/itos-container/internal/slices/run/signals"
)

// This assembly keeps real preflight, durable storage, signal handling and SDK
// I/O. Its port decorator shortens only fixture deadlines and observes drain;
// it never supplies an engine response, worker exit or controller outcome.
func TestForegroundRecoveryCommand(t *testing.T) {
	output := os.Getenv("SLICE_10_RECOVERY_RESULT")
	if output == "" {
		t.Skip("foreground recovery feature subprocess only")
	}
	open := func(key string) *os.File {
		f, err := os.OpenFile(os.Getenv(key), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = f.Close() })
		return f
	}
	out, errout := open("SLICE_10_RECOVERY_STDOUT"), open("SLICE_10_RECOVERY_STDERR")
	control := os.NewFile(3, "slice-10-recovery-observations")
	defer func() { _ = control.Close() }()
	service, _ := approvalServices(os.Stdin, out, errout)
	source := service.Readiness.(inspection.Readiness)
	engine := &recoveryEngine{RunEngine: docker.Engine{Readiness: readinessdocker.Engine{}, Selection: source.Input.Host},
		control: json.NewEncoder(control), late: os.Getenv("SLICE_10_RECOVERY_LATE") == "1", ignored: os.Getenv("SLICE_10_RECOVERY_IGNORED") == "1"}
	runner := session.Runner{
		Store: sessiondisk.Store{Storage: disk.Storage{Store: service.Store.(disk.Store)}, Codec: codec.JSON{Approval: canonical.JSON{}, Parser: jsonc.Parser{}},
			Host: host.Process{}, Checkout: &readinessdisk.Checkout{}, Input: source.Input},
		Engine: engine, Entropy: entropy.Random{}, Workspace: &sessiondisk.Workspace{},
	}
	command := runcommand.CLI{
		Approval:  approval.Preflight{Service: service, Local: service.Readiness.(ap.StagedReadiness), Canonical: canonical.JSON{}},
		Snapshots: disk.Snapshots{Store: service.Store.(disk.Store), Records: canonical.JSON{}},
		Runner:    signals.Runner{Next: runner}, Args: []string{"--", "probe"},
	}
	code := command.Run(&cli.UI{In: os.Stdin, Stdout: out, Stderr: errout, Classify: runcommand.Classify})
	writeSessionContract(t, output, code)
}

type recoveryEvent struct {
	Name     string
	Deadline time.Time
	Budget   time.Duration
}

type recoveryEngine struct {
	sp.RunEngine
	control               *json.Encoder
	opened, late, ignored bool
	graceUsed             bool
}

func (e *recoveryEngine) OpenRun(ctx context.Context, b sp.Binding) (sp.RunConnection, sp.Outcome) {
	launch := !e.opened
	e.opened = true
	cancel := func() {}
	if launch && e.late {
		original, _ := ctx.Deadline()
		ctx, cancel = context.WithTimeout(ctx, time.Second)
		deadline, _ := ctx.Deadline()
		_ = e.control.Encode(recoveryEvent{Name: "startup", Deadline: deadline, Budget: time.Until(original)})
	}
	c, failure := e.RunEngine.OpenRun(ctx, b)
	if failure != "" {
		cancel()
		return nil, failure
	}
	return recoveryConnection{RunConnection: c, engine: e, launch: launch, cancel: cancel}, ""
}

type recoveryConnection struct {
	sp.RunConnection
	engine *recoveryEngine
	launch bool
	cancel context.CancelFunc
}

func (c recoveryConnection) Close() { c.RunConnection.Close(); c.cancel() }

func (c recoveryConnection) Wait(ctx context.Context, id string) (int, sp.Outcome) {
	if !c.launch && c.engine.ignored && !c.engine.graceUsed {
		c.engine.graceUsed = true
		deadline, _ := ctx.Deadline()
		_ = c.engine.control.Encode(recoveryEvent{Name: "grace", Budget: time.Until(deadline)})
		short, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		ctx = short
	}
	return c.RunConnection.Wait(ctx, id)
}

func (c recoveryConnection) Attach(ctx context.Context, id string, streams sp.Streams) (sp.Attachment, sp.Outcome) {
	a, failure := c.RunConnection.Attach(ctx, id, streams)
	if failure != "" {
		return nil, failure
	}
	return recoveryAttachment{a, c.engine.control}, ""
}

type recoveryAttachment struct {
	sp.Attachment
	control *json.Encoder
}

func (a recoveryAttachment) Drain(ctx context.Context) sp.Outcome {
	_ = a.control.Encode(recoveryEvent{Name: "drain"})
	return a.Attachment.Drain(ctx)
}
