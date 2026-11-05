package features

import (
	"context"
	"errors"
	"fmt"
	"github.com/creack/pty"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type approvalProcess struct {
	master         *os.File
	cmd            *exec.Cmd
	cancel         context.CancelFunc
	done           chan struct{}
	mu             sync.Mutex
	output, stderr strings.Builder
	review         string
}

func (p *approvalProcess) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.output.Write(b)
}
func (p *approvalProcess) text() string { p.mu.Lock(); defer p.mu.Unlock(); return p.output.String() }
func (w *world) startApproval(mode, streams, consent string) (*approvalProcess, error) {
	master, slave, err := pty.Open()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	env, args := w.approvalMode(mode)
	if consent == "exact fingerprint" {
		args = append(args, "--fingerprint", w.approval.fingerprint)
	}
	cmd := exec.CommandContext(ctx, w.bin, args...)
	cmd.Dir = w.doctor.cwd
	cmd.Env = env
	p := &approvalProcess{master: master, cmd: cmd, cancel: cancel, done: make(chan struct{})}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, &p.stderr
	if strings.HasPrefix(streams, "redirected stdin") {
		cmd.Stdin = strings.NewReader("y\n")
	}
	if strings.Contains(streams, "redirected stdout") || streams == "redirected stdin and stdout" {
		cmd.Stdout = p
	}
	if err := cmd.Start(); err != nil {
		cancel()
		_ = master.Close()
		_ = slave.Close()
		return nil, err
	}
	_ = slave.Close()
	drained := make(chan struct{})
	go func() { _, _ = io.Copy(p, master); close(drained) }()
	go func() { _ = cmd.Wait(); <-drained; close(p.done) }()
	// A missing command exits early; it is evidence for Then, not a setup error.
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-p.done:
			p.review = p.text()
			return p, nil
		case <-ticker.C:
			if strings.Contains(p.text(), "y/N") {
				p.review = p.text()
				return p, nil
			}
		case <-ctx.Done():
			p.review = p.text()
			return p, nil
		}
	}
}
func (p *approvalProcess) answer(answer string) {
	if !strings.Contains(p.review, "y/N") {
		return
	}
	if answer == "unavailable" || answer == "no fingerprint" {
		return
	}
	if answer == "EOF" {
		_, _ = p.master.Write([]byte{4})
		return
	}
	if answer == "y after review" {
		answer = "y"
	}
	_, _ = p.master.Write([]byte(answer + "\n"))
}
func (w *world) finishApproval(p *approvalProcess) {
	<-p.done
	w.stdout, w.stderr, w.exit = p.text(), p.stderr.String(), p.cmd.ProcessState.ExitCode()
	w.approval.beforeReview = p.review
	p.cancel()
	_ = p.master.Close()
}
func (w *world) approvalTerminal(mode, answer, edit, streams string) error {
	if err := w.approvalOracle(); err != nil {
		return err
	}
	if w.approval.before == nil {
		if err := w.approvalCapture(); err != nil {
			return err
		}
	}
	w.approval.mode = mode
	p, err := w.startApproval(mode, streams, answer)
	if err != nil {
		return err
	}
	if edit != "" && strings.Contains(p.review, "y/N") {
		if err := w.approvalEdit(edit); err != nil {
			p.cancel()
			w.finishApproval(p)
			return err
		}
	}
	p.answer(answer)
	w.finishApproval(p)
	return nil
}
func (w *world) approvalConcurrent() error {
	if err := w.approvalCapture(); err != nil {
		return err
	}
	first, err := w.startApproval("plain", "", "y")
	if err != nil {
		return err
	}
	second, err := w.startApproval("plain", "", "y")
	if err != nil {
		first.cancel()
		w.finishApproval(first)
		return err
	}
	first.answer("y")
	w.finishApproval(first)
	w.approval.results = append(w.approval.results, doctorResult{stdout: w.stdout, stderr: w.stderr, exit: w.exit})
	second.answer("y")
	w.finishApproval(second)
	return nil
}
func (w *world) approvalPrerequisite() error {
	condition := w.approval.condition
	if !strings.Contains(condition, "changes during inspection") {
		return w.approvalExact("--json")
	}
	if err := w.approvalCapture(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- w.runDoctor(w.env(), "approve", "--json", "--fingerprint", w.approval.fingerprint) }()
	select {
	case err := <-done:
		return err
	case <-w.doctor.engine.reached:
		edit := "daemon ID"
		if condition == "checkout root changes during inspection" {
			edit = "checkout root"
		}
		err := w.approvalEdit(edit)
		w.doctor.engine.unpause()
		runErr := <-done
		return errors.Join(err, runErr)
	case <-time.After(9 * time.Second):
		w.doctor.engine.unpause()
		<-done
		return fmt.Errorf("prerequisite invocation exceeded deadline")
	}
}
