package porttest

import (
	"context"
	"errors"
	"sync"

	"github.com/donvargax/itos-container/internal/engine/terminal/port"
)

// Pair is an I/O-free terminal pair. Reusing Input for Output models separate
// descriptors naming one terminal; distinct output never supplies dimensions.
type Pair struct {
	Input, Output                    *Fake
	OutputStateError, OutputRawError error
	AfterInput                       func()
}

func (p *Pair) Acquire(ctx context.Context) (port.Lease, error) {
	in, err := p.Input.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	if p.AfterInput != nil {
		p.AfterInput()
	}
	if p.Output == nil || p.Output == p.Input {
		return in, nil
	}
	if err := errors.Join(p.OutputStateError, p.OutputRawError); err != nil {
		r := in.Close()
		return nil, &port.Failure{Kind: port.AcquisitionFailed, Err: err, RestoreErr: r.RestoreErr}
	}
	out, err := p.Output.Acquire(ctx)
	if err != nil {
		r := in.Close()
		return nil, &port.Failure{Kind: port.AcquisitionFailed, Err: err, RestoreErr: r.RestoreErr}
	}
	l := &pairLease{in: in, out: out, done: make(chan struct{})}
	go func() {
		var first port.Result
		select {
		case <-in.Done():
			first = in.Result()
		case <-out.Done():
			first = out.Result()
		}
		a, b := in.Close(), out.Close()
		first.RestoreErr = errors.Join(a.RestoreErr, b.RestoreErr)
		l.result = first
		close(l.done)
	}()
	return l, nil
}

type pairLease struct {
	in, out port.Lease
	done    chan struct{}
	once    sync.Once
	result  port.Result
}

func (l *pairLease) Initial() port.Dimensions        { return l.in.Initial() }
func (l *pairLease) Resizes() <-chan port.Dimensions { return l.in.Resizes() }
func (l *pairLease) Done() <-chan struct{}           { return l.done }
func (l *pairLease) Result() port.Result             { <-l.done; return l.result }
func (l *pairLease) Close() port.Result {
	l.once.Do(func() { l.in.Close(); l.out.Close() })
	return l.Result()
}

func (l *pairLease) Check() port.Cause {
	select {
	case <-l.done:
		return l.result.Cause
	default:
	}
	a, b := l.in.Check(), l.out.Check()
	for _, cause := range []port.Cause{a, b} {
		if cause == port.Hangup || cause == port.ObservationFailed {
			return cause
		}
	}
	if a != "" {
		return a
	}
	return b
}
