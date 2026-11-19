package features

// The exec command-assembly probe: what the harness asks the assembly to
// run, what the assembly reports back, and the subprocess plumbing that
// starts, joins and, for the sink-failure schedule, breaks the controller's
// own output streams. A probe that does not exist yet publishes nothing,
// which the behaviour assertions capture as missing evidence.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// execProbeRequest is what the harness asks the command assembly to run.
type execProbeRequest struct {
	Backend, Condition  string
	SessionID, Worker   string
	Argv                []string
	Stdin               string
	Output              string
	Authority, Boundary string
	Exit                int
	Stdout, Stderr      string
	Event               string
	EventBoundary       string
	StopBoundary        string
	Fault               string
	Schedule            string
	ExitObservation     string
	CleanupFault        string
	Ready, Release      string
	FollowStatus        bool
}

// execProbeReport is the assembly's own observation of the invocation.
type execProbeReport struct {
	Exit                          int
	Stdout, Stderr, Rule          string
	ExecCreate, ExecStart         bool
	StartedExecs                  int
	Signalled, Removed, Restarted bool
	Session                       string
	LateReply                     bool
	Argv                          []string
	Env                           []string
	User, WorkingDir              string
	StopExit                      int
	ElapsedMS                     int64
	StatusExit                    int
	StatusOut, StatusErr          string
	MarkedDelivered               int
}

type execProbeHandle struct {
	key, backend string
	request      execProbeRequest
	cmd          *exec.Cmd
	cancel       context.CancelFunc
	wait         chan error
	waitErr      error
	exited       bool
	transcript   strings.Builder
	pre          int

	stdoutReader *os.File
	stdoutWriter *os.File
	pumped       bytesBuffer
	pumpWG       sync.WaitGroup
}

// bytesBuffer is a mutex-guarded buffer the pump goroutine appends to and
// the event awaiter polls.
type bytesBuffer struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (b *bytesBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.Write(data)
}

func (b *bytesBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.String()
}

// execInvoke runs the command assembly's exec probe once and files the
// observation under key. A missing probe, or one that never published a
// report, is recorded as data: the behaviour assertions in Then must fail,
// not the invocation step.
func (w *world) execInvoke(key, backend string, request execProbeRequest) error {
	handle, err := w.execProbeStart(key, backend, request)
	if err != nil {
		return err
	}
	return w.execProbeWait(handle)
}

func (w *world) execProbeStart(key, backend string, request execProbeRequest) (*execProbeHandle, error) {
	return w.execProbeStartPumped(key, backend, request, false)
}

// execProbeStartPumped starts the assembly probe; when sink is set the
// probe's standard output flows through a pipe this harness can break under
// the controller's feet, after the deliberate prefix is observed.
func (w *world) execProbeStartPumped(key, backend string, request execProbeRequest, sink bool) (*execProbeHandle, error) {
	bin, err := w.sessionProbeBinary()
	if err != nil {
		return nil, err
	}
	s := w.session
	s.api.mu.Lock()
	pre := len(s.api.requests)
	s.api.mu.Unlock()
	request.Backend = backend
	request.SessionID = w.exec.id
	request.Worker = w.exec.worker
	request.Condition = w.exec.condition
	request.Argv = w.exec.argv
	request.Stdin = w.exec.stdin
	if w.exec.substituteAuthority != "" {
		request.Authority, request.Boundary = w.exec.substituteAuthority, w.exec.substituteBoundary
	}
	request.Exit = w.exec.execExit
	request.Stdout, request.Stderr = w.exec.execStdout, w.exec.execStderr
	request.Event, request.EventBoundary = w.exec.event, w.exec.boundary
	request.StopBoundary = w.exec.stopBoundary
	request.Output = filepath.Join(w.dir, "slice-38-"+key+"-report.json")
	input := filepath.Join(w.dir, "slice-38-"+key+"-input.json")
	if err := os.WriteFile(input, approvalJSON(request), 0600); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	args := []string{"-test.run=^TestPublicExecCommand$", "-test.count=1"}
	if cover := w.coverageDir(); cover != "" {
		args = append(args, "-test.gocoverdir="+cover)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = w.doctor.checkout
	if w.exec.callerRoot != "" {
		cmd.Dir = w.exec.callerRoot
	}
	cmd.Env = w.coverageEnv(sessionChildEnvironment(os.Environ()))
	for name, value := range w.doctor.env {
		cmd.Env = setEnv(cmd.Env, name, value)
	}
	for name, value := range w.exec.envExtra {
		cmd.Env = setEnv(cmd.Env, name, value)
	}
	cmd.Env = setEnv(cmd.Env, "ITOS_ROOT", cmd.Dir)
	cmd.Env = setEnv(cmd.Env, "SLICE_38_EXEC_INPUT", input)
	handle := &execProbeHandle{key: key, backend: backend, request: request, cmd: cmd, cancel: cancel, pre: pre, wait: make(chan error, 1)}
	if sink {
		reader, writer, err := os.Pipe()
		if err != nil {
			cancel()
			return nil, err
		}
		handle.stdoutReader, handle.stdoutWriter = reader, writer
		cmd.Stdout = writer
		cmd.Stderr = &handle.transcript
		handle.pumpWG.Add(1)
		go func() {
			defer handle.pumpWG.Done()
			_, _ = io.Copy(&handle.pumped, reader)
		}()
	} else {
		cmd.Stdout, cmd.Stderr = &handle.transcript, &handle.transcript
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := w.startLocalCommand(cmd); err != nil {
		cancel()
		return nil, err
	}
	_ = stdin.Close()
	return handle, nil
}

// execProbeWait joins the probe and files its observation under the handle's
// key. A probe that published no report is recorded as missing evidence.
func (w *world) execProbeWait(handle *execProbeHandle) error {
	var runErr error
	if handle.exited {
		runErr = handle.waitErr
	} else {
		runErr = handle.cmd.Wait()
	}
	defer handle.cancel()
	if handle.stdoutReader != nil {
		// The parent's own write end must go first: while it stays open the
		// pump never sees end of file after the probe exits.
		_ = handle.stdoutWriter.Close()
		handle.pumpWG.Wait()
		_ = handle.stdoutReader.Close()
		handle.transcript.WriteString(handle.pumped.String())
	}
	b := execBackend{subprocess: doctorResult{exit: -1}}
	if state := handle.cmd.ProcessState; state != nil {
		b.subprocess.exit = state.ExitCode()
	}
	b.subprocess.stdout, b.subprocess.stderr = handle.transcript.String(), ""
	s := w.session
	s.api.mu.Lock()
	b.execRequests = append([]sessionRequest{}, s.api.requests[handle.pre:]...)
	s.api.mu.Unlock()
	report, readErr := os.ReadFile(handle.request.Output)
	if readErr == nil {
		if err := json.Unmarshal(report, &b.report); err != nil {
			return err
		}
	} else if !os.IsNotExist(readErr) {
		return readErr
	} else {
		b.missing = true
	}
	if runErr != nil {
		var exit *exec.ExitError
		if !errors.As(runErr, &exit) {
			return fmt.Errorf("exec assembly subprocess failed: %w\n%s", runErr, handle.transcript.String())
		}
	}
	w.exec.results[handle.key] = b
	return nil
}

// execEachBackends runs the probe through both local backends.
func (w *world) execEachBackends(extra func(execProbeRequest) execProbeRequest) error {
	for _, backend := range []string{"memory", "disk"} {
		request := execProbeRequest{}
		if extra != nil {
			request = extra(request)
		}
		if err := w.execInvoke(backend, backend, request); err != nil {
			return err
		}
	}
	return nil
}
