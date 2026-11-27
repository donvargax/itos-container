package features

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
	"slices"
	"strconv"
	"strings"
	"time"
)

const nativeExecProbe = "/workspace/exec-probe"

type nativeExec struct {
	nonce            string
	attachments      map[string]io.Closer // only accessed under the offline recorder's lock
	clients          []*nativeExecClient
	primary          nativeExecReceipt
	receipt          nativeExecReceipt
	control, refused *recoveryProcess
	before, after    []sessionRecord
	event, cleaned   time.Time
	stop             *recoveryProcess
	err              error
}

type nativeExecReceipt struct {
	Nonce, Cwd, App, Home, Tmp, EnvHash, MountNS, NetNS string
	UID, GID                                            int
	WorkspaceInode, WorkspaceDevice                     uint64
	Denied, ReadOnly                                    bool
	Status                                              map[string]string
	Argv                                                []string
}

type nativeExecClient struct {
	p                 *recoveryProcess
	input, sink       *os.File
	name, mode, trace string
	argv              []string
	ready, eof        time.Time
}

// Admit only the trusted, non-privileged probe in this recorder's own live
// worker. The product independently has to validate its original authority.
func (a *offlineAPI) execProbeCreate(q offlineRequest, observed map[string]any) bool {
	if a.w.exec == nil || a.w.exec.native == nil || q.Query != "" || !a.safeObserved(observed) ||
		object(observed["State"])["Running"] != true {
		return false
	}
	body := q.Body
	argv, ok := body["Cmd"].([]any)
	user := body["User"]
	return ok && len(argv) > 0 && argv[0] == nativeExecProbe && body["Privileged"] != true &&
		body["Tty"] != true && (user == nil || user == "" || user == a.w.approval.uid+":"+a.w.approval.gid)
}

func (a *offlineAPI) execSessionRequest(q offlineRequest) bool {
	if a.w.exec == nil || a.w.exec.native == nil || q.Query != "" {
		return false
	}
	id, operation, _ := strings.Cut(strings.TrimPrefix(q.Path, "/exec/"), "/")
	if !offlineHex(id) {
		return false
	}
	for _, created := range a.requests {
		if created.Method != "POST" || !strings.HasSuffix(created.Path, "/exec") || created.Status != 201 || created.Observation["Id"] != id {
			continue
		}
		if operation == "json" && q.Method == "GET" {
			return true
		}
		worker := strings.TrimSuffix(strings.TrimPrefix(created.Path, "/containers/"), "/exec")
		if operation != "start" || q.Method != "POST" || q.Body["Detach"] == true || q.Body["Tty"] == true {
			return false
		}
		var ownership map[string]any
		return a.w.doctor.real.read("/exec/"+id+"/json", &ownership, 200) == nil && ownership["ID"] == id && ownership["ContainerID"] == worker
	}
	return false
}

func (w *world) nativeExecFixture(pair bool) error {
	if err := doctorRealGuard(); err != nil {
		return err
	}
	w.exec = &execWorld{native: &nativeExec{nonce: egressNonce(), attachments: map[string]io.Closer{}}}
	// The shell fixture's frozen cwd and literal APP apply to all probes here.
	if err := w.offlineFixtureWithProbe("shell", func() error {
		return w.installNativeProbe("exec-probe", func(path string) error {
			return egressBuild(w.root, "features/testdata/slice-38/probe", path)
		})
	}); err != nil {
		return err
	}
	w.recovery = &recoveryNative{mode: "exec"}
	w.offline.api.mu.Lock()
	w.offline.api.recovery = true
	w.offline.api.mu.Unlock()
	if err := w.nativeWrite("nonce", w.exec.native.nonce); err != nil {
		return err
	}
	if !pair {
		return nil
	}
	child := &world{root: w.root, bin: w.bin, coverdir: w.coverdir, dir: filepath.Join(w.dir, "slice-38-other")}
	w.recovery.second = child
	if err := os.Mkdir(child.dir, 0700); err != nil {
		return err
	}
	return child.nativeExecFixture(false)
}

func (w *world) nativePath(name string) string {
	return filepath.Join(w.doctor.checkout, "slice-38-"+name)
}
func (w *world) nativeWrite(name, value string) error {
	return os.WriteFile(w.nativePath(name), []byte(value), 0600)
}
func (w *world) nativeTick(name string) int {
	data, err := os.ReadFile(w.nativePath(name))
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(string(data))
	return n
}

// A process exit or missing barrier is an observation for Then. Never trigger
// the cancellation, disconnect or release on the strength of a timeout.
func nativeAwait(p *recoveryProcess, bound time.Duration, condition func() bool) error {
	timer := time.NewTimer(bound)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer timer.Stop()
	defer ticker.Stop()
	for {
		select {
		case <-p.done:
			return fmt.Errorf("controller exited %d before the barrier: %s", p.exit, p.errout.bytes())
		case <-timer.C:
			return errors.New("controller did not reach the native exec barrier")
		case <-ticker.C:
			if condition() {
				return nil
			}
		}
	}
}

func (w *world) nativePrimary() error {
	p := w.recoveryStartWithin(4*time.Minute, "run", "--", nativeExecProbe, "primary")
	w.recovery.run = p
	n := w.exec.native
	if err := nativeAwait(p, engineReadinessAllowance, func() bool {
		return bytes.Contains(p.out.bytes(), []byte("primary "+n.nonce+"\n")) && w.nativeTick("primary-tick") > 0
	}); err != nil {
		return fmt.Errorf("starting primary: %w", err)
	}
	w.recoveryDiscover()
	if w.recovery.err != nil {
		return w.recovery.err
	}
	if !offlineHex(w.recovery.id) || !w.recoveryNativePresent(true) {
		return errors.New("ready primary lacks a live owned registration")
	}
	// The probe can run before the controller publishes its running state.
	if err := nativeAwait(p, engineReadinessAllowance, func() bool {
		record, err := w.nativeRecords()
		if err != nil {
			return false
		}
		decoded, err := sessionDecodeRecord(record[0].Bytes)
		return err == nil && decoded["state"] == "running"
	}); err != nil {
		return err
	}
	data, err := os.ReadFile(w.nativePath("primary"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, &n.primary); err != nil {
		return err
	}
	if n.primary.Nonce != n.nonce {
		return errors.New("primary receipt came from another run")
	}
	return nil
}

func (w *world) nativeRecords() ([]sessionRecord, error) {
	state := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")
	paths := []string{
		filepath.Join(state, "sessions", w.recoveryIndexKey(), w.recovery.id, "record.json"),
		w.sessionRecordPath(), filepath.Join(state, "snapshots", w.approval.project, "1.json"),
	}
	var records []sessionRecord
	for _, path := range paths {
		record, err := sessionReadRecord(path)
		if err != nil {
			return nil, err
		}
		if record.Bytes == "" {
			return nil, errors.New("missing original session authority")
		}
		records = append(records, record)
	}
	return records, nil
}

// Keep the traced controller's stdin and both output sinks independent. The
// trace records attempted host signals; the upstream engine journal records
// signals requested through its API, including their full target IDs.
func (w *world) nativeClient(name, mode string, argv []string) (*nativeExecClient, error) {
	trace, err := w.recoveryTraceBinary()
	if err != nil {
		return nil, err
	}
	n := w.exec.native
	base := filepath.Join(w.dir, "slice-38-controller-"+name)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	p := &recoveryProcess{done: make(chan struct{}), exit: -1, cancel: cancel, pid: base + ".pid", journal: base + ".journal"}
	state := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container")
	args := []string{p.journal, p.pid, state, w.bin, "exec", w.recovery.id, "--"}
	p.cmd = exec.CommandContext(ctx, trace, append(args, argv...)...)
	p.cmd.Dir = w.doctor.checkout
	p.cmd.Env = w.env()
	for k, v := range w.doctor.env {
		p.cmd.Env = setEnv(p.cmd.Env, k, v)
	}
	c := &nativeExecClient{p: p, name: name, mode: mode, trace: base + ".signals", argv: slices.Clone(argv)}
	p.cmd.Env = setEnv(p.cmd.Env, "SLICE_11_SIGNAL_TRACE", c.trace)
	p.cmd.Env = setEnv(p.cmd.Env, "ITOS_ROOT", w.doctor.checkout)
	p.cmd.Stdout, p.cmd.Stderr = &p.out, &p.errout
	in, writer, err := os.Pipe()
	if err != nil {
		cancel()
		return nil, err
	}
	defer func() { _ = in.Close() }()
	p.cmd.Stdin, c.input = in, writer
	w.exec.restore = append(w.exec.restore, func() { _ = writer.Close() })
	if mode == "stdout" || mode == "stderr" || mode == "blocked" {
		reader, out, err := os.Pipe()
		if err != nil {
			cancel()
			return nil, err
		}
		defer func() { _ = out.Close() }()
		c.sink = reader
		w.exec.restore = append(w.exec.restore, func() { _ = reader.Close() })
		if mode == "stderr" {
			p.cmd.Stderr = out
		} else {
			p.cmd.Stdout = out
		}
	}
	w.recovery.processes = append(w.recovery.processes, p)
	n.clients = append(n.clients, c)
	lifecycleStartProcess(ctx, p)
	return c, nil
}

func (w *world) nativeWatch(name, mode string) (*nativeExecClient, error) {
	c, err := w.nativeClient(name, mode, []string{nativeExecProbe, "watch", name, mode})
	if err != nil {
		return nil, err
	}
	nonce := w.exec.native.nonce
	out, stderr := "ready "+nonce+" "+name+"\n", "live "+nonce+" "+name+"\n"
	if c.sink != nil {
		frame := out
		sink := &c.p.out
		if mode == "stderr" {
			frame = stderr
			sink = &c.p.errout
		}
		if err := c.sink.SetReadDeadline(time.Now().Add(engineReadinessAllowance)); err != nil {
			return nil, err
		}
		data := make([]byte, len(frame))
		_, err = io.ReadFull(c.sink, data)
		if err != nil || string(data) != frame {
			return nil, fmt.Errorf("missing %s readiness frame in %s (read %v)", name, mode, err)
		}
		_, _ = sink.Write(data)
	}
	err = nativeAwait(c.p, engineReadinessAllowance, func() bool {
		return bytes.Contains(c.p.out.bytes(), []byte(out)) && bytes.Contains(c.p.errout.bytes(), []byte(stderr)) && w.nativeTick(name+"-child") > 0
	})
	if err == nil {
		c.ready = time.Now()
	}
	return c, err
}

func (w *world) nativeOnce(name string) *recoveryProcess {
	p := w.recoveryStart("exec", w.recovery.id, "--", nativeExecProbe, "once", name)
	p.wait()
	return p
}

func (w *world) nativeCheckOnce(p *recoveryProcess, name string) error {
	want := "once " + w.exec.native.nonce + " " + name + "\n"
	if p == nil || p.err != nil || p.exit != 23 || string(p.out.bytes()) != want || len(p.errout.bytes()) != 0 {
		return errors.New("owned control did not return its exact nonce receipt and exit 23")
	}
	return nil
}

func (w *world) nativeRelease() error {
	if err := w.nativeWrite("end-primary", "release"); err != nil {
		return err
	}
	return nativeJoin(w.recovery.run, time.Now().Add(30*time.Second))
}

func nativeJoin(p *recoveryProcess, deadline time.Time) error {
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-p.done:
		if p.finished.After(deadline) {
			return errors.New("controller returned after its deadline")
		}
		return p.err
	case <-timer.C:
		return errors.New("controller still active at its deadline")
	}
}
