package domain

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestFiniteAnchorRequiresObservationPorts(t *testing.T) {
	w := finiteWorkspace{Workspace: struct{ port.Workspace }{&f.Workspace{}}, ctx: t.Context()}
	for _, observe := range []func() (port.MountSource, port.Outcome){w.Source, func() (port.MountSource, port.Outcome) { return w.Mounted(123) }} {
		if source, failure := observe(); source != (port.MountSource{}) || failure != port.MountUnavailable {
			t.Fatal("missing anchor port supplied mount authority", source, failure)
		}
	}
}

func TestFiniteAnchorCallsStopAtBusyHandleDeadline(t *testing.T) {
	for _, operation := range []string{"source", "mounted", "process", "allocate"} {
		t.Run(operation, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				a := newAnchorFixture(t)
				gate := make(chan struct{}, 1)
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				w := finiteWorkspace{Workspace: a.workspace, ctx: ctx, gate: gate}
				before := len(a.engine.Events)
				start := time.Now()
				switch operation {
				case "source", "mounted":
					observe := w.Source
					if operation == "mounted" {
						observe = func() (port.MountSource, port.Outcome) { return w.Mounted(123) }
					}
					if source, failure := observe(); source != (port.MountSource{}) || failure != port.MountRefused {
						t.Fatal("expired observation supplied mount authority", source, failure)
					}
				case "process":
					if p, failure := w.HoldProcess(123); p != nil || failure != port.MountRefused {
						t.Fatal("expired acquisition supplied a process", p, failure)
					}
				case "allocate":
					c := finiteConnection{RunConnection: a.connection, gate: gate}
					if failure := c.AllocateAnchor(ctx, a.intent.Record()); failure != port.MountUncertain {
						t.Fatal("expired allocation was certain", failure)
					}
				}
				if time.Since(start) != time.Second {
					t.Fatal("call exceeded remaining phase budget", time.Since(start))
				}
				gate <- struct{}{}
				synctest.Wait()
				if len(a.engine.Events) != before || len(a.engine.Anchors) != 0 {
					t.Fatal("expired call resumed after gate release", a.engine.Events)
				}
			})
		})
	}
}

func TestFiniteProcessCancellationAtGateHandoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := make(chan struct{})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		opened := false
		w := &detectorWorkspace{process: &f.Process{}, open: func() { opened = true }}
		finite := finiteWorkspace{Workspace: w, ctx: ctx, gate: gate}
		done := make(chan struct{})
		go func() {
			defer close(done)
			if p, failure := finite.HoldProcess(123); p != nil || failure != port.MountRefused {
				t.Error("cancelled acquisition returned process authority", p, failure)
			}
		}()
		synctest.Wait()
		// Cancellation can win before or after the receiver opens its handle.
		// Returning the unbuffered gate holds publication until after cancellation;
		// neither interleaving may hand the caller authority or leak a late handle.
		gate <- struct{}{}
		cancel()
		<-done
		<-gate
		synctest.Wait()
		if w.process.Closed != opened {
			t.Fatal("cancelled handoff leaked an acquired process reference")
		}
	})
}
