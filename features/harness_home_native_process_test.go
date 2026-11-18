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
	"strings"
	"time"
)

type homeNativeResult struct {
	exit            int
	out, errout     []byte
	err             error
	journal, review string
}

type homeNativeProcess struct {
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	stdin      io.WriteCloser
	done       chan struct{}
	out, diag  offlineBuffer
	result     homeNativeResult
	run        homeNativeRun
	id         string
	registered map[string]any
}

// Keep the probe's input open until the native observation barrier. No success
// prerequisite is hidden in When: a refusal is carried to the first Then.
func (w *world) homeNativeStart(mode, marker, other string) *homeNativeProcess {
	p := &homeNativeProcess{done: make(chan struct{}), result: homeNativeResult{exit: -1}}
	w.home.native.processes = append(w.home.native.processes, p)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	p.cancel = cancel
	p.cmd = exec.CommandContext(ctx, w.bin, "run", "--", "/workspace/slice-34-probe", mode, w.home.native.sentinel, marker, other)
	p.cmd.Dir, p.cmd.Env = w.doctor.checkout, w.env()
	p.cmd.Stdout, p.cmd.Stderr = &p.out, &p.diag
	p.cmd.WaitDelay = time.Second
	var err error
	p.stdin, err = p.cmd.StdinPipe()
	if err == nil {
		err = p.cmd.Start()
	}
	if err != nil {
		p.result.err = err
		close(p.done)
		return p
	}
	go func() {
		err := p.cmd.Wait()
		p.result.exit = p.cmd.ProcessState.ExitCode()
		var exited *exec.ExitError
		if err != nil && !errors.As(err, &exited) {
			p.result.err = err
		}
		if ctx.Err() != nil {
			p.result.err = errors.Join(p.result.err, errors.New("native home run exceeded its fixture deadline"))
		}
		close(p.done)
	}()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-p.done:
			p.run.worker, p.run.home, p.run.err = w.homeNativeObserve()
			return p
		case <-ctx.Done():
			<-p.done
			p.run.worker, p.run.home, p.run.err = w.homeNativeObserve()
			return p
		case <-tick.C:
			if json.Unmarshal(p.out.bytes(), &p.run.report) == nil {
				p.run.worker, p.run.home, p.run.err = w.homeNativeObserve()
				p.id, p.registered, err = w.homeNativeRegistration(p.run.worker)
				p.run.err = errors.Join(p.run.err, err)
				return p
			}
		}
	}
}

func (p *homeNativeProcess) finish() homeNativeRun {
	if p.stdin != nil {
		_ = p.stdin.Close()
	}
	<-p.done
	p.result.out, p.result.errout = p.out.bytes(), p.diag.bytes()
	p.run.exit, p.run.out, p.run.errout = p.result.exit, p.result.out, p.result.errout
	p.run.err = errors.Join(p.run.err, p.result.err)
	return p.run
}

func (w *world) homeNativeCompleted(mode, marker, other string) homeNativeRun {
	p := w.homeNativeStart(mode, marker, other)
	run := p.finish()
	_, _, err := w.homeNativeObserve()
	run.err = errors.Join(run.err, err)
	w.home.native.runs = append(w.home.native.runs, run)
	return run
}

// Discover the protected registration while the worker is held, independently
// of CLI output. A killed controller cannot be expected to print a retry.
func (w *world) homeNativeRegistration(worker map[string]any) (string, map[string]any, error) {
	labels := object(object(worker["Config"])["Labels"])
	token, _ := labels["io.itos-container.intent"].(string)
	if !offlineHex(token) {
		return "", nil, errors.New("home worker lacks its full intent identity")
	}
	root := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "sessions", w.recoveryIndexKey())
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", nil, err
	}
	id := ""
	var record map[string]any
	for _, entry := range entries {
		if !entry.IsDir() || !offlineHex(entry.Name()) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, entry.Name(), "record.json"))
		if err != nil {
			return "", nil, err
		}
		var candidate map[string]any
		if err := json.Unmarshal(data, &candidate); err != nil {
			return "", nil, err
		}
		// Earlier successful AND failed attempts remain in the catalog. Match
		// this worker's intent instead of guessing liveness from record state.
		intent, _ := candidate["intent"].(string)
		if strings.TrimSuffix(filepath.Base(intent), ".json") != token {
			continue
		}
		if id != "" || candidate["id"] != entry.Name() || candidate["project"] != w.approval.project {
			return "", nil, errors.New("home probe lacks one matching protected registration")
		}
		id, record = entry.Name(), candidate
	}
	if !offlineHex(id) {
		return "", nil, errors.New("home worker differs from its independently recorded session")
	}
	return id, record, nil
}

func (w *world) homeNativeInvoke(args ...string) homeNativeResult {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.bin, args...)
	cmd.Dir, cmd.Env = w.doctor.checkout, w.env()
	var out, diag offlineBuffer
	cmd.Stdout, cmd.Stderr = &out, &diag
	err := cmd.Run()
	r := homeNativeResult{exit: -1, out: out.bytes(), errout: diag.bytes()}
	if cmd.ProcessState != nil {
		r.exit = cmd.ProcessState.ExitCode()
	}
	var exited *exec.ExitError
	if err != nil && !errors.As(err, &exited) {
		r.err = err
	}
	if ctx.Err() != nil {
		r.err = errors.Join(r.err, fmt.Errorf("home command %v exceeded its fixture deadline", args))
	}
	return r
}
