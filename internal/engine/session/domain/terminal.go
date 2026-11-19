package domain

import (
	"context"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	terminal "github.com/donvargax/itos-container/internal/engine/terminal/port"
)

func (c finiteConnection) SetTerminal(size terminal.Dimensions) port.Outcome {
	t, ok := c.RunConnection.(port.TerminalConnection)
	if !ok {
		return port.TerminalUnavailable
	}
	return t.SetTerminal(size)
}

func (c finiteConnection) Resize(ctx context.Context, id string, size terminal.Dimensions) port.Outcome {
	t, ok := c.RunConnection.(port.TerminalConnection)
	if !ok {
		return port.TerminalResize
	}
	failure, err := connectionCall(ctx, c.gate, "terminal resize", func(call context.Context) port.Outcome { return t.Resize(call, id, size) })
	if err != nil {
		return port.CallTimeout
	}
	return failure
}

// lostClientTerminal reports a client terminal that has already ended before an
// irreversible launch edge. After the client is gone no owned workload may be
// created, attached or started for it, whatever ended the lease; the lease's
// own fresh poll makes the check immediate even when the periodic observer has
// not published the loss yet.
func lostClientTerminal(lease terminal.Lease) port.Outcome {
	if lease == nil {
		return ""
	}
	select {
	case <-lease.Done():
		if lease.Result().Cause == terminal.Hangup {
			return port.TerminalDisconnected
		}
		return port.TerminalUnavailable
	default:
	}
	switch lease.Check() {
	case terminal.Hangup:
		return port.TerminalDisconnected
	case terminal.ObservationFailed:
		return port.TerminalUnavailable
	}
	return ""
}

// waitForeground owns terminal events only while the command is live. It joins
// the existing wait before returning into the shared recovery path; no resize
// observer or independent cleanup routine can survive that transition.
func (s Runner) waitForeground(ctx context.Context, c port.RunConnection, a port.Attachment, lease terminal.Lease, b port.Binding, r port.Record, reference, id string) (exit int, exited bool, outcome port.Outcome, operation string) {
	defer func() {
		// Input EIO or an output write error can arrive before the periodic
		// hangup notification. Reobserve rather than mislabel terminal loss.
		if lease != nil && !exited && ctx.Err() == nil && outcome == port.ExecutionFailed {
			switch lease.Check() {
			case terminal.Hangup:
				outcome = port.TerminalDisconnected
			case terminal.ObservationFailed:
				outcome = port.TerminalUnavailable
			}
		}
	}()
	wait, cancel := context.WithCancel(ctx)
	defer cancel()
	type observed struct {
		exit    int
		exited  bool
		failure port.Outcome
	}
	finished := make(chan observed, 1)
	go func() {
		var v observed
		if b.Egress != (port.EgressGrant{}) {
			v.exit, v.exited, v.failure = s.waitEgress(wait, c, a, b, r)
		} else {
			v.exit, v.exited, v.failure = s.waitRun(wait, c, a, reference, id)
		}
		finished <- v
	}()
	var sizes <-chan terminal.Dimensions
	var done <-chan struct{}
	if lease != nil {
		sizes, done = lease.Resizes(), lease.Done()
	}
	for {
		// An already observed exit wins over a late notification.
		select {
		case v := <-finished:
			return v.exit, v.exited, v.failure, ""
		default:
		}
		var failure port.Outcome
		operation := ""
		resize := false
		select {
		case v := <-finished:
			return v.exit, v.exited, v.failure, ""
		case <-done:
			result := lease.Result()
			if result.Cause == terminal.Hangup {
				failure = port.TerminalDisconnected
			} else {
				failure = port.TerminalUnavailable
			}
		case size, open := <-sizes:
			if !open {
				sizes = nil
				continue
			}
			resize = true
			failure = s.resizeOwned(wait, reference, id, size)
			if failure == port.CallTimeout {
				operation = "terminal resize"
			}
			if failure == "" {
				continue
			}
		}
		// Record the parent state at the first failure. A later host signal
		// while the wait unwinds cannot replace this terminal cause.
		interrupted := ctx.Err() != nil
		cancel()
		v := <-finished
		// A resize racing the authoritative wait must not turn normal exit into
		// a controller failure. Genuine terminal loss still wins over exit zero.
		if resize && v.exited {
			return v.exit, true, v.failure, ""
		}
		if interrupted {
			return v.exit, v.exited, v.failure, ""
		}
		return v.exit, v.exited, failure, operation
	}
}

func (s Runner) resizeOwned(ctx context.Context, reference, id string, size terminal.Dimensions) (outcome port.Outcome) {
	if !size.Valid() {
		return port.TerminalUnavailable
	}
	call, end := context.WithTimeout(ctx, budget.CallDuration)
	defer end()
	defer func() {
		if budget.Cause(call) != nil {
			outcome = port.CallTimeout
		}
	}()
	// Bound individual operations while retaining ownership of their handles in
	// this goroutine. A timed-out resize cannot release a registry lock later,
	// concurrently with the shared teardown acquiring that same authority.
	registry := s.Registry
	if _, bounded := registry.(finiteRegistry); !bounded {
		registry = finiteRegistry{registry}
	}
	entry, f := registry.Open(call, reference, false)
	if f != "" {
		return port.TerminalResize
	}
	defer entry.Close()
	if entry.Validate() != "" {
		return port.TerminalResize
	}
	r := entry.Intent().Record()
	if r.Stage != "owned" || r.ContainerID != id || entry.Record().State != "running" {
		return port.TerminalResize
	}
	engine := s.Engine
	if _, bounded := engine.(finiteEngine); !bounded {
		engine = finiteEngine{engine}
	}
	c, f := engine.OpenRun(call, entry.Intent().Binding())
	if f != "" || c == nil {
		return port.TerminalResize
	}
	defer c.Close()
	running, failure := resizeTargetRunning(call, c, r)
	if failure != "" || !running {
		return failure
	}
	t := c.(port.TerminalConnection) // finiteEngine always supplies this capability
	if f := t.Resize(call, id, size); f != "" {
		if f == port.CallTimeout {
			return f
		}
		// The worker may have exited after inspection but before the daemon
		// handled the resize. Leave its authoritative wait subscribed.
		if running, failure := resizeTargetRunning(call, c, r); !running && failure == "" {
			return ""
		}
		return port.TerminalResize
	}
	return ""
}

func resizeTargetRunning(ctx context.Context, c port.RunConnection, r port.Record) (bool, port.Outcome) {
	o, status := c.Inspect(ctx, r.ContainerID)
	if status != port.Observed || o.ID != r.ContainerID || !owns(r, o) || !o.StatePresent {
		return false, port.TerminalResize
	}
	if !o.Running && o.Status != "exited" {
		return false, port.TerminalResize
	}
	return o.Running, ""
}
