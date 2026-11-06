package porttest

import (
	"context"
	"errors"
	"strings"
	"time"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// BrokenStream is a memory-only source/sink that fails without producing bytes.
type BrokenStream struct{}

func (BrokenStream) Read([]byte) (int, error)  { return 0, errors.New("input unavailable") }
func (BrokenStream) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

func ForwardingFailureContract(t Reporter, open func() (port.RunEngine, port.Binding)) {
	t.Helper()
	for _, streams := range []port.Streams{{In: BrokenStream{}}, {Out: BrokenStream{}}, {Err: BrokenStream{}}} {
		ctx, end := context.WithTimeout(context.Background(), time.Second)
		e, b := open()
		c, failure := e.OpenRun(ctx, b)
		if failure != "" {
			t.Errorf("open: %s", failure)
			end()
			continue
		}
		id, failure := c.CreateRun(ctx, b.Intent(strings.Repeat("1", 64)), []string{"probe"})
		if failure != "" {
			t.Errorf("create: %s", failure)
			c.Close()
			end()
			continue
		}
		a, failure := c.Attach(ctx, id, streams)
		if failure != "" {
			t.Errorf("attach: %s", failure)
			c.Close()
			end()
			continue
		}
		if failure = c.Start(ctx, id); failure != "" {
			t.Errorf("start: %s", failure)
		}
		select {
		case failure := <-a.Failures():
			if failure != port.ExecutionFailed {
				t.Errorf("forwarding failure: %s", failure)
			}
		case <-ctx.Done():
			t.Errorf("forwarding failure was not observable before wait")
		}
		// Consuming the notification must not erase the failure at final drain.
		if a.Drain(ctx) != port.ExecutionFailed {
			t.Errorf("drain erased forwarding failure")
		}
		a.Close()
		c.Close()
		end()
	}
}

// StdinClosureContract models an exited worker that leaves redirected stdin
// unread, with clean stdout/stderr EOF. Only its input destination fails.
func StdinClosureContract(t Reporter, open func() (port.RunEngine, port.Binding)) {
	t.Helper()
	ctx, end := context.WithTimeout(context.Background(), time.Second)
	defer end()
	e, b := open()
	c, failure := e.OpenRun(ctx, b)
	if failure != "" {
		t.Errorf("open: %s", failure)
		return
	}
	defer c.Close()
	id, failure := c.CreateRun(ctx, b.Intent(strings.Repeat("1", 64)), []string{"probe"})
	if failure != "" {
		t.Errorf("create: %s", failure)
		return
	}
	a, failure := c.Attach(ctx, id, port.Streams{In: strings.NewReader(strings.Repeat("input", 1<<20))})
	if failure != "" {
		t.Errorf("attach: %s", failure)
		return
	}
	defer a.Close()
	if failure = c.Start(ctx, id); failure != "" {
		t.Errorf("start: %s", failure)
	}
	select {
	case failure := <-a.Failures():
		if failure != port.StdinClosed {
			t.Errorf("engine input closure became a host forwarding error: %s", failure)
		}
	case <-ctx.Done():
		t.Errorf("engine input closure was not reported")
	}
	if exit, failure := c.Wait(ctx, id); failure != "" || exit != 7 {
		t.Errorf("input closure lost worker exit: %d %s", exit, failure)
	}
	if failure := a.Drain(ctx); failure != "" {
		t.Errorf("input closure invalidated completed output: %s", failure)
	}
}

func CleanupStoreContract(t Reporter, open func() (port.Store, approval.Durable, func())) {
	t.Helper()
	store, frozen, replace := open()
	ctx := context.Background()
	token := strings.Repeat("1", 64)
	ref := port.Reference(frozen.Frozen.Project, token)
	i, failure := store.New(ctx, frozen, token)
	if failure != "" {
		t.Errorf("new: %s", failure)
		return
	}
	r := i.Record()
	r.Stage, r.ContainerID = "owned", strings.Repeat("2", 64)
	if failure = i.Save(r); failure != "" {
		t.Errorf("save: %s", failure)
	}
	i.Close()
	replace()
	if i, failure = store.Open(ctx, ref); failure == "" {
		i.Close()
		t.Errorf("launch validation accepted replacement")
	}
	i, failure = store.OpenCleanup(ctx, ref)
	if failure != "" {
		t.Errorf("cleanup depended on writable checkout: %s", failure)
		return
	}
	defer i.Close()
	if i.Validate() != "" || i.Record() != r {
		t.Errorf("cleanup lost protected identity")
	}
	r.Stage = "absent"
	if failure = i.Save(r); failure != "" {
		t.Errorf("completion depended on checkout: %s", failure)
	}
}
