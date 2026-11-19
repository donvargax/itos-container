package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The exec local backends. Both run the exec command through the command
// assembly: memory wires I/O-free stateful port fakes, disk wires the real
// disk and Moby adapters against the recording Engine API fixture. The
// assembly subprocess is SLICE_38_EXEC_INPUT-driven: before the exec slice
// exists it reports nothing, which the Then steps capture as missing
// behaviour evidence, never as a setup failure.
type execWorld struct {
	condition, id, worker, leasePath string
	argv                             []string
	stdin                            string
	results                          map[string]execBackend
	before, immutable                map[string]string
	resourcesBefore                  map[string]string
	lease                            *execLease
	restore                          []func()
	engineUnavailable                bool
	substituteAuthority              string
	substituteBoundary               string
	substituted                      bool
	execLie                          string
	execExit                         int
	execStdout, execStderr           string
}

type execBackend struct {
	missing      bool
	subprocess   doctorResult
	report       execProbeReport
	execRequests []sessionRequest
}

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
}

// execProbeReport is the assembly's own observation of the invocation.
type execProbeReport struct {
	Exit                             int
	Stdout, Stderr, Rule             string
	ExecCreate, ExecStart            bool
	StartedExecs                     int
	Signalled, Removed, Restarted    bool
	Session                          string
	LateReply                        bool
	MarkedErrors, MarkedStdoutMarker string
}

type execLease struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	done  chan struct{}
	err   error
}

func (w *world) execSessionDir() string {
	return filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", w.recoveryIndexKey(), w.exec.id)
}

func (w *world) execFixture(condition string) error {
	if err := w.sessionFixture(false, "0022"); err != nil {
		return err
	}
	if err := w.doctorSentinels(); err != nil {
		return err
	}
	w.exec = &execWorld{condition: condition, results: map[string]execBackend{}}
	if err := w.execSeedSession(); err != nil {
		return err
	}
	return w.execAdmissionCondition(condition)
}

// execSeedSession publishes the original owned authority the exec command
// admits against: one frozen snapshot, one durable intent with the full
// worker ID, one public running registration with both locks, and a fresh
// running primary in the recording Engine API.
func (w *world) execSeedSession() error {
	s := w.session
	if err := w.sessionSeedSnapshot(); err != nil {
		return err
	}
	worker := approvalHash(map[string]any{"slice-38-primary": s.token})
	container := map[string]any{"Id": worker, "Name": "/itos-container-" + s.token, "Image": w.doctor.imageID,
		"Config": map[string]any{"Labels": w.sessionLabels(), "User": w.approval.uid + ":" + w.approval.gid},
		"State":  map[string]any{"Status": "running", "Running": true, "Pid": float64(1), "StartedAt": "2026-01-01T00:00:00Z"}}
	s.api.local[worker] = container
	s.api.observed[worker] = approvalClone(container)
	s.seedID = worker
	if err := writeManifestFixture(w.sessionRecordPath(), approvalJSON(w.sessionExpectedRecord("owned", worker)), 0600); err != nil {
		return err
	}
	id := approvalHash(map[string]any{"slice-38-public": s.token})
	w.exec.id, w.exec.worker = id, worker
	w.exec.leasePath = filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", w.recoveryIndexKey(), id, "controller.lock")
	w.exec.argv = []string{"/workspace/exec-probe"}
	return w.execPublicRecord(map[string]any{"schema": 1, "id": id, "project": w.approval.project, "intent": w.session.reference,
		"state": "running", "workload_exit": nil, "last_problem": nil})
}

func (w *world) execPublicRecord(record map[string]any) error {
	dir := w.execSessionDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "record.json"), approvalJSON(record), 0600); err != nil {
		return err
	}
	for _, name := range []string{"controller.lock", "operation.lock"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			return err
		}
	}
	return nil
}

// execAdmissionCondition alters exactly one fact of otherwise valid original
// authority. Filesystem defects are seeded on disk, the recording engine
// serves engine-side facts, and adapter-level faults are named for the
// command assembly, which exposes the same underlying fault on both backends.
func (w *world) execAdmissionCondition(condition string) error {
	e, s := w.exec, w.session
	switch condition {
	case "":
	case "unregistered public ID":
		e.id = approvalHash(map[string]any{"slice-38-unregistered": s.token})
	case "worker ID used as public ID":
		e.id = e.worker
	case "another project with same basename":
		other := filepath.Join(w.dir, "slice-38-other-project", filepath.Base(w.doctor.checkout))
		if err := os.MkdirAll(other, 0700); err != nil {
			return err
		}
		key := w.execIndexKey(other)
		dir := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", key, e.id)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		record := map[string]any{"schema": 1, "id": e.id, "project": w.approval.project, "intent": w.session.reference,
			"state": "running", "workload_exit": nil, "last_problem": nil}
		if err := os.WriteFile(filepath.Join(dir, "record.json"), approvalJSON(record), 0600); err != nil {
			return err
		}
		for _, name := range []string{"controller.lock", "operation.lock"} {
			if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
				return err
			}
		}
	case "different effective host user":
		intent := w.sessionExpectedRecord("owned", e.worker)
		uid, _ := strconv.Atoi(w.approval.uid)
		intent["uid"] = strconv.Itoa(uid + 1)
		if err := writeManifestFixture(w.sessionRecordPath(), approvalJSON(intent), 0600); err != nil {
			return err
		}
	case "conflicting intent and snapshot":
		path := w.sessionSnapshotPath()
		var snapshot map[string]any
		if err := json.Unmarshal([]byte(w.execSnapshotText()), &snapshot); err != nil {
			return err
		}
		snapshot["fingerprint"] = "sha256:" + approvalHash(map[string]any{"slice-38-conflict": s.token})
		if err := os.WriteFile(path, approvalJSON(snapshot), 0600); err != nil {
			return err
		}
	case "foreign worker with copied labels":
		s.api.local[e.worker]["Image"] = "sha256:" + strings.Repeat("e", 64)
	case "original daemon replaced":
		intent := w.sessionExpectedRecord("owned", e.worker)
		intent["engine_id"] = "other-daemon"
		if err := writeManifestFixture(w.sessionRecordPath(), approvalJSON(intent), 0600); err != nil {
			return err
		}
	case "symlinked protected record":
		path := filepath.Join(w.execSessionDir(), "record.json")
		outside := filepath.Join(w.doctor.host, "slice-38-outside-record")
		if err := os.Rename(path, outside); err != nil {
			return err
		}
		if err := os.Symlink(outside, path); err != nil {
			return err
		}
	case "malformed protected record":
		if err := os.WriteFile(filepath.Join(w.execSessionDir(), "record.json"), []byte(`{"schema":`), 0600); err != nil {
			return err
		}
	case "protected state unreadable", "primary controller probe unknown":
		// Adapter-level faults: the assembly opens the real registration but
		// its read, or the advisory controller probe, fails without changing
		// ownership. The lease still holds the controller lock on disk.
	case "preparing", "ready", "stopping", "stopped", "failed":
		record := map[string]any{"schema": 1, "id": e.id, "project": w.approval.project, "intent": w.session.reference,
			"state": condition, "workload_exit": nil, "last_problem": nil}
		if err := os.WriteFile(filepath.Join(w.execSessionDir(), "record.json"), approvalJSON(record), 0600); err != nil {
			return err
		}
	case "running record but primary exited":
		sessionMutateInspection(s.api.local[e.worker], "exited state after starting")
	case "running record but worker absent":
		delete(s.api.local, e.worker)
	case "primary controller absent":
	case "original engine unavailable":
		e.engineUnavailable = true
	default:
		return fmt.Errorf("unknown exec admission condition %q", condition)
	}
	if condition != "primary controller absent" {
		if err := w.execHoldLease(); err != nil {
			return err
		}
	}
	var err error
	e.before, err = w.sessionProtected()
	if err != nil {
		return err
	}
	e.immutable = map[string]string{}
	for path, value := range e.before {
		if path == filepath.Join(w.execSessionDir(), "controller.lock") {
			continue
		}
		e.immutable[path] = value
	}
	e.resourcesBefore = map[string]string{}
	for id, c := range s.api.local {
		e.resourcesBefore[id] = string(approvalJSON(c))
	}
	return nil
}

func (w *world) execSnapshotText() string {
	a := w.approval
	return string(approvalJSON(map[string]any{"schema": 1, "project": a.project, "generation": "1", "fingerprint": a.fingerprint, "settings": a.settings}))
}

// execIndexKey is the protected session index key of an arbitrary checkout,
// encoded independently of the product's own lookup.
func (w *world) execIndexKey(checkout string) string {
	return approvalHash(map[string]any{"index": w.approval.uid, "checkout": checkout})
}

// execHoldLease keeps an exclusive advisory lock on the session's controller
// lock, so a probe sees the primary controller present. The holder is a
// separate process that lives until the harness releases its input.
func (w *world) execHoldLease() error {
	binary, err := w.execLeaseBinary()
	if err != nil {
		return err
	}
	ready := filepath.Join(w.dir, "slice-38-lease-ready")
	lease := &execLease{done: make(chan struct{})}
	lease.cmd = exec.Command(binary, w.exec.leasePath, ready)
	lease.stdin, err = lease.cmd.StdinPipe()
	if err != nil {
		return err
	}
	lease.cmd.Stdout, lease.cmd.Stderr = io.Discard, io.Discard
	if err := lease.cmd.Start(); err != nil {
		return err
	}
	w.exec.lease = lease
	go func() { lease.err = lease.cmd.Wait(); close(lease.done) }()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if data, readErr := os.ReadFile(ready); readErr == nil && string(data) == "ready" {
			return nil
		}
		select {
		case <-lease.done:
			return fmt.Errorf("controller lease holder exited early: %v", lease.err)
		case <-timer.C:
			return errors.New("controller lease holder never reported ready")
		case <-ticker.C:
		}
	}
}

func (w *world) execLeaseBinary() (string, error) {
	path := filepath.Join(w.dir, "slice-38-lease")
	if filepath.IsAbs(w.bin) {
		path = w.bin + "-slice-38-lease"
	}
	return featureBinary(path, func(target string) error {
		return recoveryBuildHelper(w.root, target, execLeaseSource)
	})
}

// execSubstitute mutates exactly one fact of the fixture's original
// authority at an observation barrier, after a successful admission
// inspection. The next revalidation must refuse it.
func (w *world) execSubstitute() error {
	e := w.exec
	switch e.substituteAuthority {
	case "protected intent":
		intent := w.sessionExpectedRecord("owned", e.worker)
		intent["fingerprint"] = "sha256:" + approvalHash(map[string]any{"slice-38-substituted": e.id})
		if err := writeManifestFixture(w.sessionRecordPath(), approvalJSON(intent), 0600); err != nil {
			return err
		}
	case "full worker ID and labels":
		c := w.session.api.local[e.worker]
		c["Id"] = strings.Repeat("e", 64)
		object(object(c["Config"])["Labels"])["io.itos-container.intent"] = "foreign"
	case "pinned engine socket":
		intent := w.sessionExpectedRecord("owned", e.worker)
		intent["engine_endpoint"] = "unix:///slice-38-replaced-socket"
		if err := writeManifestFixture(w.sessionRecordPath(), approvalJSON(intent), 0600); err != nil {
			return err
		}
	case "original daemon identity":
		w.doctor.engine.info["ID"] = "other-daemon"
	}
	e.substituted = true
	return nil
}

func (w *world) execSubstitutionArmed() bool {
	e := w.exec
	return e != nil && e.substituteAuthority != "" && !e.substituted
}

func (w *world) closeExec() error {
	if w.exec == nil {
		return nil
	}
	var result error
	if lease := w.exec.lease; lease != nil {
		_ = lease.stdin.Close()
		select {
		case <-lease.done:
		case <-time.After(5 * time.Second):
			result = errors.New("controller lease holder did not exit")
			_ = lease.cmd.Process.Kill()
		}
	}
	for _, restore := range w.exec.restore {
		restore()
	}
	return result
}

// execInvoke runs the command assembly's exec probe once and files the
// observation under key. A missing probe, or one that never published a
// report, is recorded as data: the behaviour assertions in Then must fail,
// not the invocation step.
func (w *world) execInvoke(key, backend string, request execProbeRequest) error {
	bin, err := w.sessionProbeBinary()
	if err != nil {
		return err
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
	request.Output = filepath.Join(w.dir, "slice-38-"+key+"-report.json")
	input := filepath.Join(w.dir, "slice-38-"+key+"-input.json")
	if err := os.WriteFile(input, approvalJSON(request), 0600); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	args := []string{"-test.run=^TestPublicExecCommand$", "-test.count=1"}
	if cover := w.coverageDir(); cover != "" {
		args = append(args, "-test.gocoverdir="+cover)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = w.doctor.checkout
	cmd.Env = w.coverageEnv(sessionChildEnvironment(os.Environ()))
	for name, value := range w.doctor.env {
		cmd.Env = setEnv(cmd.Env, name, value)
	}
	cmd.Env = setEnv(cmd.Env, "ITOS_ROOT", w.doctor.checkout)
	cmd.Env = setEnv(cmd.Env, "SLICE_38_EXEC_INPUT", input)
	var transcript strings.Builder
	cmd.Stdout, cmd.Stderr = &transcript, &transcript
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := w.startLocalCommand(cmd); err != nil {
		return err
	}
	_ = stdin.Close()
	runErr := cmd.Wait()
	b := execBackend{subprocess: doctorResult{exit: -1}}
	if state := cmd.ProcessState; state != nil {
		b.subprocess.exit = state.ExitCode()
	}
	b.subprocess.stdout, b.subprocess.stderr = transcript.String(), ""
	s.api.mu.Lock()
	b.execRequests = append([]sessionRequest{}, s.api.requests[pre:]...)
	s.api.mu.Unlock()
	report, readErr := os.ReadFile(request.Output)
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
			return fmt.Errorf("exec assembly subprocess failed: %w\n%s", runErr, transcript.String())
		}
	}
	w.exec.results[key] = b
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

// execBackendResult is what a Then step judges: the assembly's report when it
// published one, otherwise the subprocess's own output.
func (b execBackend) result() doctorResult {
	if b.missing {
		return b.subprocess
	}
	return doctorResult{exit: b.report.Exit, stdout: b.report.Stdout, stderr: b.report.Stderr}
}

// Only a product worker never executes this helper; it holds the advisory
// controller lock of a fixture session until its standard input closes.
const execLeaseSource = `package main
import ("os"; "syscall")
func main() {
    if len(os.Args) != 3 { os.Exit(96) }
    f, err := os.OpenFile(os.Args[1], os.O_RDWR, 0)
    if err != nil { os.Exit(97) }
    if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil { os.Exit(98) }
    if os.WriteFile(os.Args[2], []byte("ready"), 0600) != nil { os.Exit(99) }
    data := make([]byte, 64)
    for {
        if _, err := os.Stdin.Read(data); err != nil { return }
    }
}
`
