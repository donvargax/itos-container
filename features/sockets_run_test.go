package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type socketOutput struct {
	mu      sync.Mutex
	pending []byte
	frames  []socketReport
	events  chan socketReport
	err     error
}

func (o *socketOutput) Write(b []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.pending = append(o.pending, b...)
	for {
		line, tail, found := bytes.Cut(o.pending, []byte{'\n'})
		if !found {
			break
		}
		var r socketReport
		if err := json.Unmarshal(line, &r); err != nil {
			o.err = errors.Join(o.err, errors.New("invalid socket probe JSON frame"))
		} else {
			o.frames = append(o.frames, r)
			select {
			case o.events <- r:
			default:
				o.err = errors.Join(o.err, errors.New("socket probe exceeded its report budget"))
			}
		}
		o.pending = tail
	}
	return len(b), nil
}

func (o *socketOutput) snapshot() ([]socketReport, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	err := o.err
	if len(o.pending) != 0 {
		err = errors.Join(err, errors.New("truncated socket probe frame"))
	}
	return append([]socketReport{}, o.frames...), err
}

type socketProcess struct {
	input  io.WriteCloser
	output socketOutput
	stderr offlineBuffer
	done   chan struct{}
	err    error // read only after done
	exit   int
}

func startSocketProcess(ctx context.Context, bin string, args []string, dir string, env []string) (*socketProcess, error) {
	p := &socketProcess{done: make(chan struct{}), output: socketOutput{events: make(chan socketReport, 2048)}, exit: -1}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir, cmd.Env, cmd.WaitDelay = dir, env, time.Second
	cmd.Stdout, cmd.Stderr = &p.output, &p.stderr
	var err error
	p.input, err = cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		_ = p.input.Close()
		return nil, err
	}
	go func() {
		defer close(p.done)
		p.err = cmd.Wait()
		p.exit = cmd.ProcessState.ExitCode()
	}()
	return p, nil
}

func (p *socketProcess) await(ctx context.Context, kind, phase, nonce string) (socketReport, error) {
	for {
		var r socketReport
		select {
		case r = <-p.output.events:
		case <-p.done:
			// Wait drains stdout before closing done; retain final queued frames
			// even if the process exited immediately after writing them.
			select {
			case r = <-p.output.events:
			default:
				return socketReport{}, fmt.Errorf("probe exited %d before %s/%s: %s", p.exit, kind, phase, p.stderr.bytes())
			}
		case <-ctx.Done():
			return socketReport{}, fmt.Errorf("fixture deadline before %s/%s: %w", kind, phase, ctx.Err())
		}
		if r.Kind == "error" {
			return r, fmt.Errorf("trusted socket probe failed: %s", r.Error)
		}
		if r.Kind == kind && r.Phase == phase && r.Nonce == nonce {
			return r, nil
		}
	}
}

func (p *socketProcess) release(nonce, phase string) error {
	return json.NewEncoder(p.input).Encode(map[string]string{"Nonce": nonce, "Phase": phase})
}

func (s *socketWorld) observeRunning() error {
	a := s.w.offline.api
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.created) != 1 {
		return errors.New("barrier lacks one independently observed worker")
	}
	id := a.created[0]
	observed, err := a.inspect(id)
	if err != nil || a.identity(observed, id) != nil || object(observed["State"])["Running"] != true {
		return errors.New("full-ID inspection did not confirm a running worker")
	}
	if s.id != "" && s.id != id {
		return errors.New("publication or phase switched worker identity")
	}
	s.id = id
	snapshot, err := sessionReadRecord(s.snapshotPath())
	if err != nil || snapshot.Bytes == "" {
		return errors.New("running worker lacks its protected frozen snapshot")
	}
	if s.snapshot.Bytes != "" && snapshot != s.snapshot {
		return errors.New("frozen snapshot changed while the worker ran")
	}
	s.snapshot = snapshot
	return nil
}

func (s *socketWorld) snapshotPath() string {
	return filepath.Join(s.w.doctor.env["XDG_STATE_HOME"], "itos-container", "snapshots", s.w.approval.project, "1.json")
}

func (s *socketWorld) atBarrier(ctx context.Context, p *socketProcess, phase string) error {
	if _, err := p.await(ctx, "barrier", phase, s.config.Nonce); err != nil {
		return err
	}
	if err := s.observeRunning(); err != nil {
		return err
	}
	s.barriers = append(s.barriers, phase)
	return nil
}

func (w *world) socketRun() error {
	s, o := w.sockets, w.offline
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if s.config.Mode == "existing" {
		s.fixtureErr = errors.Join(s.fixtureErr, s.hostControl(ctx, "first-before"))
	}
	b, err := json.Marshal(s.config)
	if err != nil {
		s.fixtureErr = errors.Join(s.fixtureErr, err)
		return nil
	}
	o.argv = []string{"/workspace/slice-19-probe", "worker", string(b)}
	env := setEnv(w.env(), "ITOS_ROOT", w.doctor.checkout)
	p, err := startSocketProcess(ctx, w.bin, append([]string{"run", "--"}, o.argv...), w.doctor.checkout, env)
	if err != nil {
		s.runErr = err
		return nil
	}
	s.runErr = s.drive(ctx, p)
	_ = p.input.Close()
	if s.runErr != nil {
		// Missing barrier / early refusal is Then data, never a blocked When.
		cancel()
	}
	<-p.done
	o.exit, o.stderr = p.exit, p.stderr.bytes()
	s.reports, err = p.output.snapshot()
	s.runErr = errors.Join(s.runErr, err)
	if ctx.Err() != nil {
		s.runErr = errors.Join(s.runErr, ctx.Err())
	}
	// All observations precede identity-checked harness cleanup in the After.
	o.err = o.api.observeAfter()
	o.after, err = s.protected()
	s.fixtureErr = errors.Join(s.fixtureErr, err)
	o.resourcesAfter, err = w.doctor.real.snapshotResources()
	s.fixtureErr = errors.Join(s.fixtureErr, err)
	if s.listener != nil {
		// Final control receipt checkpoint and inode/liveness observation.
		s.fixtureErr = errors.Join(s.fixtureErr, s.listenersUnchanged())
	}
	return nil
}

func (s *socketWorld) drive(ctx context.Context, p *socketProcess) error {
	if s.config.Mode == "existing" {
		if err := s.atBarrier(ctx, p, "first"); err != nil {
			return err
		}
		s.fixtureErr = errors.Join(s.fixtureErr, s.hostControl(ctx, "first-after"), s.hostControl(ctx, "running-before"))
		if err := p.release(s.config.Nonce, "first"); err != nil {
			return err
		}
	} else {
		if err := s.atBarrier(ctx, p, "live"); err != nil {
			return err
		}
		// No scanner delay: bind/rename and controls directly precede release.
		if err := s.publish(ctx); err != nil {
			s.fixtureErr = errors.Join(s.fixtureErr, err)
			return err
		}
		s.fixtureErr = errors.Join(s.fixtureErr, s.hostControl(ctx, "parent-before"))
		if err := p.release(s.config.Nonce, "live"); err != nil {
			return err
		}
		if err := s.atBarrier(ctx, p, "parent"); err != nil {
			return err
		}
		s.fixtureErr = errors.Join(s.fixtureErr, s.hostControl(ctx, "parent-after"), s.hostControl(ctx, "child-before"))
		if err := p.release(s.config.Nonce, "parent"); err != nil {
			return err
		}
	}
	if err := s.atBarrier(ctx, p, "exit"); err != nil {
		return err
	}
	last := "child-after"
	if s.config.Mode == "existing" {
		last = "running-after"
	}
	s.fixtureErr = errors.Join(s.fixtureErr, s.hostControl(ctx, last))
	var err error
	s.edit, err = sessionReadRecord(filepath.Join(s.w.doctor.checkout, "slice-19-edit"))
	if err != nil {
		return err
	}
	s.editIdentity, err = socketStat(filepath.Join(s.w.doctor.checkout, "slice-19-edit"))
	if err != nil {
		return err
	}
	// A second native inspection surrounds the host read before release.
	if err = s.observeRunning(); err != nil {
		return err
	}
	return p.release(s.config.Nonce, "exit")
}
