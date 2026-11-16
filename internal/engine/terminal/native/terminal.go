// Package native implements the host-terminal port for an explicitly bound file.
package native

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/donvargax/itos-container/internal/engine/terminal/port"
)

// Provider never discovers or closes a caller's terminal.
type Provider struct {
	file   *os.File
	osName string
	mu     sync.Mutex
	active *lease
}

// New binds a file and an explicit operating-system value.
func New(file *os.File, osName string) *Provider { return &Provider{file: file, osName: osName} }

// Acquire subscribes before measuring, so a resize between the initial measurement
// and observation is retained. All fallible setup precedes successful MakeRaw;
// there is no partially acquired lease or fallible post-raw setup to roll back.
func (p *Provider) Acquire(ctx context.Context) (port.Lease, error) {
	if p.osName != "linux" || !supported {
		return nil, &port.Failure{Kind: port.Unsupported}
	}
	if err := ctx.Err(); err != nil {
		return nil, &port.Failure{Kind: port.Cancelled, Err: err}
	}
	if !p.mu.TryLock() {
		return nil, &port.Failure{Kind: port.Busy}
	}
	defer p.mu.Unlock()
	if p.active != nil {
		return nil, &port.Failure{Kind: port.Busy}
	}
	if p.file == nil || *p.file == (os.File{}) {
		return nil, &port.Failure{Kind: port.InvalidTerminal}
	}
	// SyscallConn only rejects a nil File, already refused above. A closed
	// file still yields a connection; Control below reports its failure.
	conn, _ := p.file.SyscallConn()
	l := &lease{provider: p, conn: conn, resizes: make(chan port.Dimensions, 1),
		done: make(chan struct{}), stop: make(chan struct{}), signals: make(chan os.Signal, 1)}
	signal.Notify(l.signals, windowSignal)
	kind := port.InvalidTerminal
	var err error
	err = l.control(func(fd int) error {
		if !term.IsTerminal(fd) {
			return errors.New("not a terminal")
		}
		kind = port.InvalidDimensions
		l.initial, err = dimensions(fd)
		if err != nil {
			return err
		}
		kind = port.Cancelled
		if err := ctx.Err(); err != nil {
			return err
		}
		kind = port.AcquisitionFailed
		l.original, err = term.MakeRaw(fd)
		return err
	})
	if err != nil {
		signal.Stop(l.signals)
		return nil, &port.Failure{Kind: kind, Err: err}
	}
	p.active = l
	go l.run(ctx)
	return l, nil
}

type lease struct {
	provider   *Provider
	conn       syscall.RawConn
	original   *term.State
	initial    port.Dimensions
	resizes    chan port.Dimensions
	done, stop chan struct{}
	signals    chan os.Signal
	closeOnce  sync.Once
	result     port.Result
}

func (l *lease) Initial() port.Dimensions        { return l.initial }
func (l *lease) Resizes() <-chan port.Dimensions { return l.resizes }
func (l *lease) Done() <-chan struct{}           { return l.done }
func (l *lease) Result() port.Result             { <-l.done; return l.result }
func (l *lease) Close() port.Result {
	l.closeOnce.Do(func() { close(l.stop) })
	return l.Result()
}

// control pins the caller's descriptor for each syscall without File.Fd's
// implicit switch to blocking mode or retaining a reusable numeric descriptor.
func (l *lease) control(call func(int) error) error {
	var callErr error
	err := l.conn.Control(func(fd uintptr) { callErr = call(int(fd)) })
	if err != nil {
		return err
	}
	return callErr
}

func dimensions(fd int) (port.Dimensions, error) {
	columns, rows, err := term.GetSize(fd)
	if err != nil {
		return port.Dimensions{}, err
	}
	d := port.Dimensions{Columns: columns, Rows: rows}
	if !d.Valid() {
		return port.Dimensions{}, errors.New("invalid terminal dimensions")
	}
	return d, nil
}

func (l *lease) run(ctx context.Context) {
	// observe owns no subsidiary goroutines. It has returned, and stopped its
	// ticker, before restoration and the public completion notification.
	l.result = l.observe(ctx)
	signal.Stop(l.signals)
	l.result.RestoreErr = l.control(func(fd int) error { return term.Restore(fd, l.original) })
	l.provider.mu.Lock()
	l.provider.active = nil
	l.provider.mu.Unlock()
	close(l.resizes)
	close(l.done)
}

func (l *lease) observe(ctx context.Context) port.Result {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	last := l.initial
	resize := false
	for {
		select {
		case <-ctx.Done():
			return port.Result{Cause: port.ContextCancelled, Err: ctx.Err()}
		case <-l.stop:
			return port.Result{Cause: port.Closed}
		case <-l.signals:
			resize = true
		case <-ticker.C:
		}
		var size port.Dimensions
		var hungup bool
		err := l.control(func(fd int) error {
			var err error
			hungup, err = disconnected(fd)
			if err != nil || hungup {
				return err
			}
			if resize {
				size, err = dimensions(fd)
			}
			return err
		})
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			return port.Result{Cause: port.ObservationFailed, Err: err}
		}
		if hungup {
			return port.Result{Cause: port.Hangup}
		}
		if resize && size != last {
			last = size
			// A single publisher replaces an unread notification, retaining the
			// latest size without a queue or a blocked cancellation path.
			select {
			case <-l.resizes:
			default:
			}
			l.resizes <- size
		}
		resize = false
	}
}

func disconnected(fd int) (bool, error) {
	if fd < 0 || fd > 2147483647 {
		return false, errors.New("invalid terminal descriptor")
	}
	// Events is deliberately zero: unread payload never wakes or spins this
	// observer. poll still reports HUP, ERR and NVAL without requesting input.
	descriptors := []pollDescriptor{{Fd: int32(fd)}}
	_, err := poll(descriptors)
	if err != nil {
		return false, err
	}
	events := descriptors[0].Revents
	if events&pollHangup != 0 {
		return true, nil
	}
	if events&(pollError|pollInvalid) != 0 {
		return false, errors.New("terminal observation unavailable")
	}
	return false, nil
}
