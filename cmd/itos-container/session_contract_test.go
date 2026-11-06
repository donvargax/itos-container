package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"

	canonical "github.com/donvargax/itos-container/internal/engine/approval/canonical"
	disk "github.com/donvargax/itos-container/internal/engine/approval/disk"
	approval "github.com/donvargax/itos-container/internal/engine/approval/domain"
	approvalhost "github.com/donvargax/itos-container/internal/engine/approval/host"
	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	manifestjson "github.com/donvargax/itos-container/internal/engine/manifest/jsonc"
	readinessdisk "github.com/donvargax/itos-container/internal/engine/readiness/disk"
	readinessdocker "github.com/donvargax/itos-container/internal/engine/readiness/docker"
	readinessport "github.com/donvargax/itos-container/internal/engine/readiness/port"
	sessioncodec "github.com/donvargax/itos-container/internal/engine/session/codec"
	sessiondisk "github.com/donvargax/itos-container/internal/engine/session/disk"
	sessiondocker "github.com/donvargax/itos-container/internal/engine/session/docker"
	session "github.com/donvargax/itos-container/internal/engine/session/domain"
	sessionentropy "github.com/donvargax/itos-container/internal/engine/session/entropy"
	sessionport "github.com/donvargax/itos-container/internal/engine/session/port"
	rundomain "github.com/donvargax/itos-container/internal/slices/run/domain"
)

// This protocol exists only in the test binary. Recovery receives a safe record
// reference and host context, never the independent observer's container ID.
type sessionContractRequest struct {
	Action, Backend, Reference, Token, Fault, Umask      string
	Output, Checkpoint, Journal, MemoryFile              string
	Boundary                                             string
	LaunchBarrier, ReplacedPath, CleanupCause, ChildExit string
	Registered                                           string
}

type sessionContractResult struct {
	Outcome, Reference, ID, Cleanup, PreflightFailure string
	Handoff                                           bool
	Code                                              int
	Diagnostic                                        string
	RunEvents                                         []string
	Workload                                          *int
	Events                                            []sessionContractEvent
	Memory                                            *sessionMemoryOutput
	Trace                                             []offlineEvent
	Stdout, Stderr                                    string
	CleanupCode                                       int
	SessionID                                         string
	PublicRecords                                     map[string][]byte
}

type sessionContractEvent struct {
	Operation, Stage, Bytes string
	Failed                  bool
}

type sessionContractPreflight struct {
	approval.Preflight
	view ap.View
}

func (s *sessionContractPreflight) Inspect() ap.View {
	s.view = s.Preflight.Inspect()
	return s.view
}

func assembleSessionContract(request sessionContractRequest, ops *sessionContractIO) session.Service {
	root := os.Getenv("ITOS_ROOT")
	codec := sessioncodec.JSON{Approval: canonical.JSON{}, Parser: manifestjson.Parser{}}
	storage := disk.Storage{Store: disk.Store{StateHome: os.Getenv("XDG_STATE_HOME"), Home: os.Getenv("HOME")},
		IO: disk.SnapshotIO{Create: ops.create, Write: ops.write, FileSync: ops.fileSync, Rename: ops.rename, ParentSync: ops.parentSync, DirectorySync: ops.directorySync}, Exclusive: ops.exclusive}
	return session.Service{
		Store:  sessiondisk.Store{Storage: storage, Codec: codec, Host: approvalhost.Process{}, Checkout: &readinessdisk.Checkout{}, Input: readinessport.Input{Root: root, OS: runtime.GOOS, Architecture: runtime.GOARCH, Host: os.Getenv("DOCKER_HOST"), Context: os.Getenv("DOCKER_CONTEXT"), TLSVerify: os.Getenv("DOCKER_TLS_VERIFY"), CertPath: os.Getenv("DOCKER_CERT_PATH"), APIVersion: os.Getenv("DOCKER_API_VERSION")}},
		Engine: sessiondocker.Engine{Readiness: readinessdocker.Engine{}, Selection: os.Getenv("DOCKER_HOST")},
		Entropy: sessionentropy.Random{Read: func(data []byte) (int, error) {
			if request.Fault == "entropy source unavailable" {
				return 0, errors.New("fixture entropy unavailable")
			}
			token, err := hex.DecodeString(request.Token)
			return copy(data, token), err
		}},
	}
}
func sessionContractOutput(result sessionport.Result) sessionContractResult {
	return sessionContractResult{Outcome: string(result.Outcome), Reference: result.Reference, ID: result.ID, Cleanup: string(result.Cleanup), Handoff: result.Outcome == sessionport.Owned}
}

func TestSessionContractProbe(t *testing.T) {
	path := os.Getenv("SLICE_9_PROBE_INPUT")
	if path == "" {
		t.Skip("session feature subprocess only")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var request sessionContractRequest
	if err = json.Unmarshal(data, &request); err != nil {
		t.Fatal(err)
	}
	mask, err := strconv.ParseUint(request.Umask, 8, 9)
	if err != nil {
		t.Fatal(err)
	}
	restore := sessionContractUmask(int(mask))
	defer restore()
	ops := &sessionContractIO{fault: request.Fault, journal: request.Journal}
	var memory *sessionMemoryOutput
	controllerFor := func() session.Service {
		controller := assembleSessionContract(request, ops)
		if request.Backend == "memory" {
			controller, memory = assembleMemorySession(t, request, controller)
		}
		return controller
	}
	var result sessionContractResult
	switch request.Action {
	case "run", "run-cleanup":
		result = offlineContract(t, request, controllerFor, ops)
	case "create":
		service, _ := approvalServices(os.Stdin, io.Discard, io.Discard)
		preflight := &sessionContractPreflight{Preflight: approval.Preflight{Service: service, Local: service.Readiness.(ap.StagedReadiness), Canonical: canonical.JSON{}}}
		snapshots := disk.Snapshots{Store: service.Store.(disk.Store), Records: canonical.JSON{}}
		handoff, preflightErr := rundomain.Preflight(preflight, snapshots)
		if preflightErr != nil {
			result.PreflightFailure = fmt.Sprintf("%T: %v (%s, %s)", preflightErr, preflightErr, preflight.view.Status, preflight.view.Failure)
			result.Outcome = "preflight-refused"
		} else {
			// The parent checkpoints independently after snapshot publication and
			// before any session work. Exit is also a bounded barrier outcome.
			writeSessionContract(t, request.Checkpoint, handoff)
			var release string
			if err := json.NewDecoder(os.Stdin).Decode(&release); err != nil || release != "continue" {
				t.Fatal("session checkpoint was not released")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			controller := controllerFor()
			result = sessionContractOutput(controller.Create(ctx, handoff))
			cancel()
		}
	case "cleanup":
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		controller := controllerFor()
		result = sessionContractOutput(controller.Cleanup(ctx, request.Reference))
		cancel()
	default:
		t.Fatal("unknown session probe action")
	}
	if memory != nil {
		result.Memory = memory
		result.Events = memory.events()
	}
	if memory == nil {
		result.Events = append([]sessionContractEvent{}, ops.events...)
	}
	writeSessionContract(t, request.Output, result)
}

func writeSessionContract(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	// The parent precreates the files, so even umask 0777 leaves observations
	// readable. Product files do not use this observation channel.
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
