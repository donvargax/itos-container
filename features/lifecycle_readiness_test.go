package features

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const lifecycleReadyFrame = "slice-12:ready\n"

// Signals and transport loss share this gate, including the healthy control.
func (w *world) lifecycleReadyAction(action func()) bool {
	if !w.lifecycleReady() {
		return false
	}
	action()
	return true
}

// File publication precedes capture propagation. All three observations share
// the controller's original deadline, including independent observer I/O.
func (w *world) lifecycleReady() bool {
	l := w.lifecycle
	p := l.run
	if l.err != nil {
		return false
	}
	if p == nil || p.started.IsZero() {
		l.err = errors.Join(l.err, errors.New("readiness missing controller start origin"))
		return false
	}
	ctx, cancel := context.WithDeadline(context.Background(), p.started.Add(engineReadinessAllowance))
	defer cancel()
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		select {
		case <-p.done:
			cancel()
		case <-ctx.Done():
		}
	}()
	defer func() { cancel(); <-watchDone }()

	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	missing := "ready file"
	var boundary map[string]any
	for {
		if cause := lifecycleReadinessEnd(ctx, p); cause != "" {
			return l.readinessFailure(missing, cause, nil)
		}
		var err error
		available := true
		if boundary == nil {
			boundary, missing, err = w.lifecycleReadyObservation(ctx)
		} else {
			available, err = w.lifecycleRecorderProblem()
			if !available {
				missing = "recorder observation"
			}
		}
		// Observer completion at/after expiry, or after process exit, is not readiness.
		if cause := lifecycleReadinessEnd(ctx, p); cause != "" {
			return l.readinessFailure(missing, cause, err)
		}
		if err != nil {
			return l.readinessFailure(missing, "observation failed", err)
		}
		if boundary != nil && available {
			missing = "foreground frame"
			ready, err := l.readinessFrame(ctx)
			if err != nil {
				return l.readinessFailure(missing, "observation failed", err)
			}
			if cause := lifecycleReadinessEnd(ctx, p); cause != "" {
				return l.readinessFailure(missing, cause, nil)
			}
			if ready {
				w.recoveryDiscover()
				if cause := lifecycleReadinessEnd(ctx, p); cause != "" {
					return l.readinessFailure(missing, cause, nil)
				}
				l.initialBoundary, l.ready = boundary, time.Now()
				return true
			}
		}
		select {
		case <-ctx.Done():
		case <-p.done:
		case <-tick.C:
		}
	}
}

func lifecycleReadinessEnd(ctx context.Context, p *recoveryProcess) string {
	select {
	case <-p.done:
		return "controller exited"
	default:
	}
	deadline, _ := ctx.Deadline()
	if !time.Now().Before(deadline) {
		return "deadline expired with live controller"
	}
	return ""
}

func (l *lifecycleNative) readinessFailure(missing, cause string, err error) bool {
	l.err = errors.Join(l.err, fmt.Errorf("lifecycle readiness missing %s; %s; mode=%q condition=%q elapsed=%s limit=%s",
		missing, cause, l.mode, l.condition, time.Since(l.run.started), engineReadinessAllowance), err)
	return false
}

func (w *world) lifecycleRecorderProblem() (bool, error) {
	if w.offline == nil || w.offline.api == nil {
		return false, errors.New("engine recorder unavailable")
	}
	a := w.offline.api
	// A recorder handler may hold this lock during its own bounded native call.
	// Poll contention within readiness's deadline instead of blocking beyond it.
	if !a.mu.TryLock() {
		return false, nil
	}
	defer a.mu.Unlock()
	return true, a.problem
}

func (w *world) lifecycleReadyObservation(ctx context.Context) (map[string]any, string, error) {
	if available, err := w.lifecycleRecorderProblem(); err != nil || !available {
		return nil, "recorder", err
	}
	data, err := os.ReadFile(filepath.Join(w.doctor.checkout, "slice-12-ready"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, "ready file", nil
	}
	if err != nil || string(data) != "ready" {
		return nil, "ready file", err
	}
	a := w.offline.api
	if !a.mu.TryLock() {
		return nil, "recorder", nil
	}
	id := ""
	if len(a.created) == 1 {
		id = a.created[0]
	}
	a.mu.Unlock()
	if id == "" {
		return nil, "running identity", nil
	}
	var observed map[string]any
	// Never hold the recorder lock across an independent engine request.
	err = w.doctor.real.readContext(ctx, "/containers/"+id+"/json", &observed, 200)
	if err != nil {
		return nil, "running identity", err
	}
	if !a.mu.TryLock() {
		return nil, "running identity", nil
	}
	err = errors.Join(a.problem, a.identity(observed, id))
	a.mu.Unlock()
	if err != nil || object(observed["State"])["Running"] != true {
		return nil, "running identity", err
	}
	return observed, "foreground frame", nil
}

func (l *lifecycleNative) readinessFrame(ctx context.Context) (bool, error) {
	if l.condition == "unread workload stdout" {
		if l.pipe == nil {
			return false, errors.New("stdout readiness pipe unavailable")
		}
		// Read only bytes already available, up to the exact pre-release frame.
		// The workload writer is not released yet; never drain its later flood.
		data := l.run.out.bytes()
		if len(data) < len(lifecycleReadyFrame) {
			_, available, err, readErr := lifecyclePipeSize(l.pipe)
			if err != nil || readErr != nil {
				return false, errors.Join(err, readErr)
			}
			if available > 0 {
				deadline, _ := ctx.Deadline()
				if err := l.pipe.SetReadDeadline(deadline); err != nil {
					return false, err
				}
				defer func() { _ = l.pipe.SetReadDeadline(time.Time{}) }()
				part := make([]byte, min(available, len(lifecycleReadyFrame)-len(data)))
				n, err := l.pipe.Read(part)
				_, _ = l.run.out.Write(part[:n])
				if err != nil {
					return false, err
				}
			}
		}
	}
	data := l.run.out.bytes()
	// Match a complete line, not a substring of a different or partial frame.
	return bytes.HasPrefix(data, []byte(lifecycleReadyFrame)) || bytes.Contains(data, []byte("\n"+lifecycleReadyFrame)), nil
}
