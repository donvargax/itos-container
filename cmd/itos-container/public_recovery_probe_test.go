package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/cli"
	canonical "github.com/donvargax/itos-container/internal/engine/approval/canonical"
	approvaldisk "github.com/donvargax/itos-container/internal/engine/approval/disk"
	approval "github.com/donvargax/itos-container/internal/engine/approval/domain"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	sessiondisk "github.com/donvargax/itos-container/internal/engine/session/disk"
	"github.com/donvargax/itos-container/internal/engine/session/entropy"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/donvargax/itos-container/internal/engine/session/presentation"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
	statuscommand "github.com/donvargax/itos-container/internal/slices/status/command"
	stopcommand "github.com/donvargax/itos-container/internal/slices/stop/command"
)

// These seams exist only in the test executable. They change observations at
// adapter boundaries, never command results, ownership decisions or engine data.
type publicProbeRequest struct {
	Args                                                        []string
	Fault, Boundary, ID, Ready, Release, Output, Stdout, Stderr string
}

func TestPublicRecoveryCommand(t *testing.T) {
	path := os.Getenv("SLICE_11_REQUEST")
	if path == "" {
		t.Skip("slice-11 fixture subprocess only")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var request publicProbeRequest
	if err = json.Unmarshal(data, &request); err != nil || len(request.Args) == 0 {
		t.Fatal("invalid recovery probe request", err)
	}
	open := func(path string) *os.File {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = file.Close() })
		return file
	}
	out, errout := open(request.Stdout), open(request.Stderr)
	service, _ := approvalServices(nil, out, errout)
	runner, recovery := sessionServices(service)
	registry := runner.Registry.(sessiondisk.Registry)
	publicStorage := registry.Storage.(approvaldisk.Registry)
	store := runner.Store.(sessiondisk.Store)
	intentStorage := store.Storage.(approvaldisk.Storage)
	ioFault := publicProbeIO(request.Fault)
	publicStorage.IO, intentStorage.IO = ioFault, ioFault
	if request.Fault == "atomic publication" {
		publicStorage.Exclusive = func(*os.Root, string, string) error { return errors.New("fixture publication failure") }
	}
	if request.Fault == "record wrong filesystem owner" {
		publicStorage.Stat = func(file *os.File) (os.FileInfo, error) {
			info, err := file.Stat()
			if err != nil || filepath.Base(file.Name()) != "record.json" {
				return info, err
			}
			original := reflect.Indirect(reflect.ValueOf(info.Sys()))
			metadata := reflect.New(original.Type())
			metadata.Elem().Set(original)
			metadata.Elem().FieldByName("Uid").SetUint(uint64(os.Geteuid() + 1))
			return sessionOwnerInfo{FileInfo: info, metadata: metadata.Interface()}, nil
		}
	}
	if request.Fault == "public progress replacement" {
		publicStorage.Stat = publicProgressReplacement(t, publicStorage, request)
	}
	store.Storage = intentStorage
	registry.Store, registry.Storage = store, publicStorage
	runner.Store, runner.Registry = store, registry
	recovery.Registry = registry
	if request.Fault == "entropy unavailable" {
		runner.Entropy = entropy.Random{Read: func([]byte) (int, error) { return 0, errors.New("fixture entropy failure") }}
	}
	if request.Fault == "occupied public ID" {
		runner.Entropy = entropy.Random{Read: func(data []byte) (int, error) { id, err := hex.DecodeString(request.ID); return copy(data, id), err }}
	}
	if request.Fault == "unclassified adapter failure" {
		recovery.Engine = publicUnknownEngine{}
	}
	if request.Boundary != "" || request.Fault == "session reopen deadline" {
		barrier := &publicBarrier{request: request}
		runner.Registry = &publicBarrierRegistry{Registry: registry, barrier: barrier}
		runner.Engine = publicBarrierEngine{RunEngine: runner.Engine, barrier: barrier}
	}
	var code int
	ui := &cli.UI{Stdout: out, Stderr: errout, Classify: runcommand.Classify}
	switch request.Args[0] {
	case "status":
		code = (statuscommand.CLI{Service: recovery, Presentation: presentation.UI{Out: out, Err: errout}, JSON: true}).Run()
	case "stop":
		code = (stopcommand.CLI{Service: recovery, Presentation: presentation.UI{Out: out, Err: errout}, JSON: true, Args: request.Args[1:]}).Run()
	case "run":
		code = (runcommand.CLI{Approval: approval.Preflight{Service: service, Local: service.Readiness.(ap.StagedReadiness), Canonical: canonical.JSON{}}, Snapshots: approvaldisk.Snapshots{Store: service.Store.(approvaldisk.Store), Records: canonical.JSON{}}, Runner: runner, Args: append([]string{"--"}, request.Args[1:]...)}).Run(ui)
	default:
		t.Fatal("unknown recovery probe action")
	}
	writeSessionContract(t, request.Output, code)
}

type publicUnknownEngine struct{}

func (publicUnknownEngine) OpenRun(context.Context, sp.Binding) (sp.RunConnection, sp.Outcome) {
	return nil, "unclassified fixture failure"
}

func publicProbeIO(fault string) approvaldisk.SnapshotIO {
	bad := errors.New("synthetic persistence failure")
	state := func(file *os.File) string {
		path := file.Name()
		info, err := file.Stat()
		if err != nil {
			return ""
		}
		if info.IsDir() {
			path = filepath.Join(path, "record.json")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		var record struct{ State, Stage string }
		_ = json.Unmarshal(data, &record)
		if record.State != "" {
			return record.State
		}
		return record.Stage
	}
	return approvaldisk.SnapshotIO{
		Write: func(file *os.File, data []byte) (int, error) {
			if fault == "short record write" && strings.Contains(file.Name(), "/sessions/") {
				return 0, io.ErrShortWrite
			}
			return file.Write(data)
		},
		FileSync: func(file *os.File) error {
			public := strings.Contains(file.Name(), "/sessions/")
			if (fault == "record file fsync" && public && state(file) == "preparing") || (fault == "lock object durability" && public && strings.HasSuffix(file.Name(), ".lock")) || (fault == "session completion file fsync" && public && state(file) == "stopped") || (fault == "intent absent publication fsync" && state(file) == "absent") {
				return bad
			}
			return file.Sync()
		},
		Rename: func(root *os.Root, from, to string) error {
			if fault == "atomic publication" && to == "record.json" {
				return bad
			}
			return root.Rename(from, to)
		},
		ParentSync: func(file *os.File) error {
			if strings.Contains(file.Name(), "/sessions/") && ((fault == "session directory fsync" && state(file) == "preparing") || (fault == "session completion parent fsync" && state(file) == "stopped")) {
				return bad
			}
			return file.Sync()
		},
	}
}

type publicBarrier struct {
	request publicProbeRequest
	reached bool
}

func (b *publicBarrier) wait(ctx context.Context, boundary string) {
	if b.reached || b.request.Boundary != boundary {
		return
	}
	b.reached = true
	_ = os.WriteFile(b.request.Ready, []byte("ready"), 0600)
	timer := time.NewTicker(5 * time.Millisecond)
	defer timer.Stop()
	for {
		if data, err := os.ReadFile(b.request.Release); err == nil && string(data) == "continue" {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}

type publicBarrierRegistry struct {
	sp.Registry
	barrier *publicBarrier
	opened  int
}

func (r *publicBarrierRegistry) ForRun(frozen ap.Durable) sp.Registry {
	if original, ok := r.Registry.(sp.RunRegistry); ok {
		r.Registry = original.ForRun(frozen)
	}
	return r
}

func (r *publicBarrierRegistry) Open(ctx context.Context, id string, readOnly bool) (sp.Session, sp.Outcome) {
	r.opened++
	if r.opened == 1 {
		r.barrier.wait(ctx, "before create")
	}
	if r.opened == 2 {
		r.barrier.wait(ctx, "created before start")
		if r.barrier.request.Fault == "session reopen deadline" {
			// Exercise the real registry's expired-context path after create;
			// later cleanup opens retain their fresh teardown context.
			expired, end := context.WithDeadline(ctx, time.Now().Add(-time.Second))
			defer end()
			return r.Registry.Open(expired, id, readOnly)
		}
	}
	return r.Registry.Open(ctx, id, readOnly)
}

type publicBarrierEngine struct {
	sp.RunEngine
	barrier *publicBarrier
}

func (e publicBarrierEngine) OpenRun(ctx context.Context, b sp.Binding) (sp.RunConnection, sp.Outcome) {
	c, f := e.RunEngine.OpenRun(ctx, b)
	if f != "" {
		return nil, f
	}
	return publicBarrierConnection{RunConnection: c, barrier: e.barrier}, ""
}

type publicBarrierConnection struct {
	sp.RunConnection
	barrier *publicBarrier
}

func (c publicBarrierConnection) Wait(ctx context.Context, id string) (int, sp.Outcome) {
	c.barrier.wait(ctx, "foreground wait")
	exit, f := c.RunConnection.Wait(ctx, id)
	c.barrier.wait(ctx, "observed exit before cleanup")
	return exit, f
}
