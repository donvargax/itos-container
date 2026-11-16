package porttest

import (
	"context"
	"errors"
	"sync"

	"github.com/donvargax/itos-container/internal/engine/terminal/port"
)

// Fake is a stateful, I/O-free terminal for tests of terminal consumers. Configure
// exported fields before use; the methods synchronize subsequent external events.
type Fake struct {
	// OutputOnly owns settings and hangup observation without measuring size.
	OutputOnly   bool
	Size         port.Dimensions
	Invalid      port.FailureKind
	State        int
	RestoreError error
	mu           sync.Mutex
	active       *fakeLease
	raw          bool
	broken       bool
	hungup       bool
}

func (f *Fake) Acquire(ctx context.Context) (port.Lease, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Invalid == port.Unsupported {
		return nil, &port.Failure{Kind: f.Invalid}
	}
	if err := ctx.Err(); err != nil {
		return nil, &port.Failure{Kind: port.Cancelled, Err: err}
	}
	if f.active != nil {
		return nil, &port.Failure{Kind: port.Busy}
	}
	if f.Invalid != "" {
		return nil, &port.Failure{Kind: f.Invalid}
	}
	if f.broken || f.hungup {
		return nil, &port.Failure{Kind: port.InvalidTerminal}
	}
	if !f.OutputOnly && !f.Size.Valid() {
		return nil, &port.Failure{Kind: port.InvalidDimensions}
	}
	l := &fakeLease{fake: f, initial: f.Size, original: f.State, resizes: make(chan port.Dimensions, 1),
		done: make(chan struct{}), stop: make(chan struct{}), changed: make(chan struct{}, 1)}
	f.active = l
	f.raw = true
	go l.run(ctx)
	return l, nil
}

func (f *Fake) Raw() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.raw }
func (f *Fake) Snapshot() any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return struct {
		State int
		Raw   bool
	}{f.State, f.raw}
}
func (f *Fake) ChangeState() { f.mu.Lock(); defer f.mu.Unlock(); f.State++ }
func (f *Fake) Resize(size port.Dimensions) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Size = size
	f.notify()
}

// Hangup has the same effect whether input was left unread; the fake, like the
// native observer, has no input reader and does not interpret payload bytes.
func (f *Fake) Hangup(_ bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hungup = true
	f.notify()
}
func (f *Fake) Break() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.broken = true
	f.notify()
}

func (f *Fake) notify() {
	if f.active != nil {
		select {
		case f.active.changed <- struct{}{}:
		default:
		}
	}
}

type fakeLease struct {
	fake                *Fake
	initial             port.Dimensions
	original            int
	resizes             chan port.Dimensions
	done, stop, changed chan struct{}
	closeOnce           sync.Once
	result              port.Result
}

func (l *fakeLease) Initial() port.Dimensions        { return l.initial }
func (l *fakeLease) Resizes() <-chan port.Dimensions { return l.resizes }
func (l *fakeLease) Done() <-chan struct{}           { return l.done }
func (l *fakeLease) Result() port.Result             { <-l.done; return l.result }
func (l *fakeLease) Close() port.Result {
	l.closeOnce.Do(func() { close(l.stop) })
	return l.Result()
}

func (l *fakeLease) Check() port.Cause {
	select {
	case <-l.done:
		return l.result.Cause
	default:
	}
	f := l.fake
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.hungup {
		return port.Hangup
	}
	if f.broken || (!f.OutputOnly && !f.Size.Valid()) {
		return port.ObservationFailed
	}
	return ""
}

func (l *fakeLease) run(ctx context.Context) {
	l.result = l.observe(ctx)
	f := l.fake
	f.mu.Lock()
	l.result.RestoreErr = f.RestoreError
	if f.broken || f.hungup {
		l.result.RestoreErr = errors.New("terminal vanished")
	}
	if l.result.RestoreErr == nil {
		f.State = l.original
		f.raw = false
	}
	f.active = nil
	f.mu.Unlock()
	close(l.resizes)
	close(l.done)
}

func (l *fakeLease) observe(ctx context.Context) port.Result {
	last := l.initial
	for {
		select {
		case <-ctx.Done():
			return port.Result{Cause: port.ContextCancelled, Err: ctx.Err()}
		case <-l.stop:
			return port.Result{Cause: port.Closed}
		case <-l.changed:
		}
		f := l.fake
		f.mu.Lock()
		size, broken, hungup := f.Size, f.broken, f.hungup
		f.mu.Unlock()
		if broken {
			return port.Result{Cause: port.ObservationFailed, Err: errors.New("terminal unavailable")}
		}
		if hungup {
			return port.Result{Cause: port.Hangup}
		}
		if !f.OutputOnly && !size.Valid() {
			return port.Result{Cause: port.ObservationFailed, Err: errors.New("invalid terminal dimensions")}
		}
		if !f.OutputOnly && size != last {
			last = size
			select {
			case <-l.resizes:
			default:
			}
			l.resizes <- size
		}
	}
}
