package main

import (
	"context"
	"encoding/json"
	approvalhost "github.com/donvargax/itos-container/internal/engine/approval/host"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/donvargax/itos-container/internal/cli"
	canonical "github.com/donvargax/itos-container/internal/engine/approval/canonical"
	approvaldisk "github.com/donvargax/itos-container/internal/engine/approval/disk"
	approval "github.com/donvargax/itos-container/internal/engine/approval/domain"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	manifestdisk "github.com/donvargax/itos-container/internal/engine/manifest/disk"
	"github.com/donvargax/itos-container/internal/engine/manifest/jsonc"
	"github.com/donvargax/itos-container/internal/engine/manifest/reference"
	policydisk "github.com/donvargax/itos-container/internal/engine/policy/disk"
	policyyaml "github.com/donvargax/itos-container/internal/engine/policy/yaml"
	sessiondisk "github.com/donvargax/itos-container/internal/engine/session/disk"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/donvargax/itos-container/internal/engine/session/presentation"
	"github.com/donvargax/itos-container/internal/slices/doctor/inspection"
	runcommand "github.com/donvargax/itos-container/internal/slices/run/command"
	"github.com/donvargax/itos-container/internal/slices/run/signals"
)

// The standalone test executable is the sole entry point for this fixture.
// Its host port supplies a disposable account home, independently of HOME/XDG.
// Normal binaries have neither this environment switch nor this assembly.
func TestMain(m *testing.M) {
	if os.Getenv("SLICE_21_ACCOUNT_PROBE") == "1" {
		home, failure := (approvalhost.Process{}).AccountHome()
		if json.NewEncoder(os.Stdout).Encode(struct {
			Home    string
			Failure ap.Failure
		}{home, failure}) != nil {
			os.Exit(96)
		}
		os.Exit(0)
	}
	home := os.Getenv("SLICE_21_ACCOUNT_HOME")
	cleanup := func() {}
	if home == "" {
		var err error
		home, err = os.MkdirTemp("", "slice-21-account-")
		if err != nil {
			os.Exit(96)
		}
		cleanup = func() { _ = os.RemoveAll(home) }
	}
	accountLookup = controlAccountHost{Host: approvalhost.Process{}, home: filepath.Clean(home)}
	if home := os.Getenv("SLICE_21_ACCOUNT_HOME"); home != "" && (len(os.Args) < 2 || !strings.HasPrefix(os.Args[1], "-test.")) {
		if os.Getenv("SLICE_21_BOUNDARY") != "" {
			os.Exit(controlStateCommand(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, home))
		}
		os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}

type controlAccountHost struct {
	ap.Host
	home string
}

func (h controlAccountHost) AccountHome() (string, ap.Failure) {
	if path := os.Getenv("SLICE_21_LOOKUP_LOG"); path != "" {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return "", "state.unavailable"
		}
		_, _ = file.WriteString("account-home lookup\n")
		_ = file.Close()
	}
	if h.home == "unavailable" {
		return "", "state.unavailable"
	}
	return h.home, ""
}

func controlStateCommand(args []string, in io.Reader, out, errout io.Writer, home string) int {
	var command commandLine
	parser, err := kong.New(&command, append(cli.Flags(), kong.Writers(out, errout), kong.Vars{"version": "slice-21-fixture"})...)
	if err != nil {
		return 96
	}
	parsed, err := parser.Parse(runOptions(args))
	if err != nil {
		return 96
	}
	service, view := approvalServices(in, out, errout)
	host := controlAccountHost{Host: service.Host, home: home}
	service.Host = host
	service = coordinatedApproval(service, host)
	if os.Getenv("SLICE_21_BOUNDARY") == "after bootstrap inspection" {
		store := service.Store.(approvaldisk.Store)
		store.Control = controlBootstrapProbe{store.Control.(approvaldisk.Control)}
		service.Store = store
	}
	ui := &cli.UI{In: in, Stdout: out, Stderr: errout, Classify: runcommand.Classify}
	name, _, _ := strings.Cut(parsed.Command(), " ")
	switch name {
	case "doctor":
		command.Doctor.Policies = inspection.Policies{Source: policyyaml.Source{Files: policydisk.Files{}}}
		command.Doctor.Manifests = inspection.Manifests{Files: manifestdisk.Files{}, Parser: jsonc.Parser{}, References: reference.References{}}
		command.Doctor.Readiness, command.Doctor.Approval, command.Doctor.Presentation = service.Readiness, service, view
		return command.Doctor.Run(ui)
	case "approve":
		if os.Getenv("SLICE_21_BOUNDARY") == "before publication" {
			service.Store = controlApprovalStore{Store: service.Store}
		}
		command.Approve.Service, command.Approve.Presentation = service, view
		return command.Approve.Run(ui)
	}
	runner, recovery := sessionServices(service)
	store := runner.Store.(sessiondisk.Store)
	store.Host = host
	registry := runner.Registry.(sessiondisk.Registry)
	registry.Store = store
	runner.Store, runner.Registry, recovery.Registry = store, registry, registry
	switch name {
	case "run":
		command.Run.Approval = approval.Preflight{Service: service, Local: service.Readiness.(ap.StagedReadiness), Canonical: canonical.JSON{}}
		command.Run.Snapshots = approvaldisk.Snapshots{Store: service.Store.(approvaldisk.Store), Records: canonical.JSON{}}
		command.Run.Runner = signals.Runner{Next: controlPreflightRunner{run: runner.Run}}
		return command.Run.Run(ui)
	case "status":
		command.Status.Service, command.Status.Presentation = recovery, presentation.UI{Out: out, Err: errout}
		return command.Status.Run()
	case "stop":
		command.Stop.Service, command.Stop.Presentation = recovery, presentation.UI{Out: out, Err: errout}
		return command.Stop.Run()
	default:
		return 96
	}
}

type controlApprovalStore struct{ ap.Store }

// Pause only a writer's initial reservation read, after the real read returns.
// The default-tree fixture has one location; a peer can create the lock before
// this writer leaves its unlocked inspection. All validation remains real.
type controlBootstrapProbe struct{ approvaldisk.Control }

func (p controlBootstrapProbe) Begin(ctx context.Context, c ap.Candidate, write bool) (ap.ControlTransaction, ap.Failure) {
	control := p.Control
	if write {
		control.Reservations = &controlBootstrapReservations{Reservations: control.Reservations}
	}
	return control.Begin(ctx, c, write)
}

type controlBootstrapReservations struct {
	ap.Reservations
	paused bool
}

func (r *controlBootstrapReservations) Roots(ctx context.Context, c ap.Candidate, path string) ([]string, ap.Failure) {
	roots, failure := r.Reservations.Roots(ctx, c, path)
	if !r.paused && failure == "" {
		r.paused = true
		b := publicBarrier{request: publicProbeRequest{Boundary: "after bootstrap inspection", Ready: os.Getenv("SLICE_21_READY"), Release: os.Getenv("SLICE_21_RELEASE")}}
		b.wait(ctx, "after bootstrap inspection")
	}
	return roots, failure
}

type controlPreflightRunner struct {
	run func(context.Context, ap.Durable, []string, sp.Streams) sp.RunResult
}

func (r controlPreflightRunner) Run(ctx context.Context, frozen ap.Durable, args []string, streams sp.Streams) sp.RunResult {
	if os.Getenv("SLICE_21_BOUNDARY") == "before admission" {
		b := publicBarrier{request: publicProbeRequest{Boundary: "before admission", Ready: os.Getenv("SLICE_21_READY"), Release: os.Getenv("SLICE_21_RELEASE")}}
		b.wait(ctx, "before admission")
	}
	return r.run(ctx, frozen, args, streams)
}

func (s controlApprovalStore) Commit(c ap.Candidate, generation string) (ap.Record, ap.Failure) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	b := publicBarrier{request: publicProbeRequest{Boundary: "before publication", Ready: os.Getenv("SLICE_21_READY"), Release: os.Getenv("SLICE_21_RELEASE")}}
	b.wait(ctx, "before publication")
	return s.Store.Commit(c, generation)
}
