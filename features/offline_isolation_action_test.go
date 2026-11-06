package features

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/client"
)

func (w *world) isolationAction() error {
	s := w.offline.isolation
	// Controls are observations of independent fixture operations, never of
	// product code. An unavailable control remains a test failure, not denial.
	if s.mode == "network" || s.mode == "dns" {
		s.controlsBefore, s.controlError = s.routeControls(false)
	}
	count := 1
	if s.mode == "filesystem" || s.mode == "exit" {
		count = 2
	}
	for n := 0; n < count; n++ {
		c := s.config
		if n != 0 {
			c.Previous = c.Nonce
			c.Nonce = isolationNonce()
		}
		if s.mode == "exit" {
			s.removeFault = n == 1
		}
		s.runs = append(s.runs, s.execute(c))
	}
	var err error
	s.after, err = s.protected()
	s.controlError = errors.Join(s.controlError, err)
	s.resourcesAfter, err = w.doctor.real.snapshotResources()
	s.controlError = errors.Join(s.controlError, err)
	switch s.mode {
	case "identity":
		s.setuidAfter, err = s.setuidControl()
	case "filesystem":
		s.nestedAfter = s.nestedControl()
	case "host":
		s.hostAfter, err = s.hostControls()
	case "primary", "exit":
		s.controlBeatsAfter, err = s.liveControl()
	case "network", "dns":
		s.controlsAfter, err = s.routeControls(true)
	}
	s.controlError = errors.Join(s.controlError, err)
	return nil // every early product exit is judged by the named Then
}

func (s *isolationWorld) execute(c isolationConfig) *isolationRun {
	w, a := s.w, s.w.offline.api
	r := &isolationRun{config: c, exit: -1}
	path := filepath.Join(w.doctor.checkout, "slice-14-run-"+c.Nonce+".json")
	if err := os.WriteFile(path, approvalJSON(c), 0600); err != nil {
		r.err = err
		return r
	}
	var err error
	r.input, err = sessionReadRecord(path)
	if err != nil {
		r.err = err
		return r
	}
	ctx, cancel := context.WithTimeout(context.Background(), 170*time.Second)
	defer cancel()
	argv := []string{"/workspace/slice-14-probe", c.Mode, "/workspace/" + filepath.Base(path)}
	if c.Mode == "exit" {
		argv = append(argv, "SLICE_14_SYNTHETIC_ARGV")
	}
	cmd := exec.CommandContext(ctx, w.bin, append([]string{"run", "--"}, argv...)...)
	cmd.Dir, cmd.Env = w.doctor.checkout, w.env()
	for k, v := range w.doctor.env {
		cmd.Env = setEnv(cmd.Env, k, v)
	}
	cmd.Env = setEnv(cmd.Env, "ITOS_ROOT", w.doctor.checkout)
	var out, errout offlineBuffer
	cmd.Stdout, cmd.Stderr = &out, &errout
	in, err := cmd.StdinPipe()
	if err != nil {
		r.err = err
		return r
	}
	defer func() { _ = in.Close() }()
	a.mu.Lock()
	begin, created := len(a.requests), len(a.created)
	a.mu.Unlock()
	if err = cmd.Start(); err != nil {
		r.err = err
		return r
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	barrier := c.Mode == "network" || c.Mode == "dns" || c.Mode == "primary"
	if barrier {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		barrierDeadline := time.NewTimer(140 * time.Second)
		defer barrierDeadline.Stop()
	loop:
		for {
			select {
			case err = <-done:
				done <- err
				break loop
			case <-ctx.Done():
				break loop
			case <-barrierDeadline.C:
				r.err = errors.New("workload did not reach its live barrier")
				_ = in.Close()
				break loop
			case <-ticker.C:
				reports, parseErr := isolationReports(out.bytes())
				if parseErr != nil {
					continue
				}
				ready := false
				for _, v := range reports {
					ready = ready || (v.Nonce == c.Nonce && (v.Kind == "descendant" || v.Phase == "first"))
				}
				if !ready {
					continue
				}
				a.mu.Lock()
				if len(a.created) > created {
					r.id = a.created[created]
					r.running, err = a.inspect(r.id)
					if err == nil {
						err = a.identity(r.running, r.id)
					}
				} else {
					err = errors.New("report arrived without a native worker")
				}
				a.mu.Unlock()
				if err == nil && object(r.running["State"])["Running"] != true {
					err = errors.New("barrier report arrived after worker exit")
				}
				if err == nil && c.Mode == "primary" {
					err = s.observeDescendant(r, reports)
				}
				r.err = errors.Join(r.err, err)
				_, err = in.Write([]byte("continue\n"))
				r.err = errors.Join(r.err, err)
				_ = in.Close()
				break loop
			}
		}
	} else {
		_ = in.Close()
	}
	err = <-done
	r.exit = 0
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			r.exit = exit.ExitCode()
		} else {
			r.err = errors.Join(r.err, err)
		}
	}
	if ctx.Err() != nil {
		r.err = errors.Join(r.err, errors.New("product exceeded native fixture deadline"))
	}
	r.stdout, r.stderr = out.bytes(), errout.bytes()
	r.reports, err = isolationReports(r.stdout)
	r.err = errors.Join(r.err, err)
	// Native and protected-state observations precede EVERY harness cleanup.
	a.mu.Lock()
	if len(a.created) > created {
		r.id = a.created[created]
		r.absent = w.doctor.real.read("/containers/"+r.id+"/json", nil, 404) == nil
		if !r.absent {
			r.stopped, err = a.inspect(r.id)
			r.err = errors.Join(r.err, err)
		}
	}
	r.requests = append([]offlineRequest{}, a.requests[begin:]...)
	r.err = errors.Join(r.err, a.problem)
	a.mu.Unlock()
	r.record, err = sessionReadRecord(w.sessionRecordPath())
	r.err = errors.Join(r.err, err)
	r.expected = approvalClone(w.sessionExpectedRecord("absent", r.id))
	r.snapshot, err = sessionReadRecord(filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "snapshots", w.approval.project, "1.json"))
	r.err = errors.Join(r.err, err)
	return r
}

func (s *isolationWorld) observeDescendant(r *isolationRun, reports []isolationReport) error {
	var report isolationReport
	for _, v := range reports {
		if v.Kind == "descendant" {
			report = v
		}
	}
	if report.Nonce != r.config.Nonce || report.Values["beat"] != "2" || report.Values["start"] == "" {
		return errors.New("no live descendant heartbeat/start identity")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	top, err := s.cli.ContainerTop(ctx, r.id, client.ContainerTopOptions{Arguments: []string{"-eo", "pid,args"}})
	if err != nil {
		return err
	}
	for _, row := range top.Processes {
		if len(row) < 2 || !strings.Contains(strings.Join(row[1:], " "), "child "+r.config.Nonce) {
			continue
		}
		pid, err := strconv.Atoi(row[0])
		if err != nil {
			return err
		}
		if isolationProcessStart(pid) != report.Values["start"] {
			return errors.New("descendant native start identity differs from its heartbeat")
		}
		r.descendantPID, r.descendantStart = pid, report.Values["start"]
		return nil
	}
	return fmt.Errorf("native worker %s has no matching live descendant", r.id)
}
