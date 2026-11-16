package porttest

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/donvargax/itos-container/internal/engine/terminal/port"
)

// Fixture controls a terminal's external state, not calls made by the provider.
// Break invalidates observation and restoration; Hangup optionally leaves unread
// input first. A native factory runs this contract in a dedicated signal child.
type Fixture struct {
	Provider    port.Provider
	Raw         func() bool
	Snapshot    func() any
	ChangeState func()
	Resize      func(port.Dimensions)
	Hangup      func(bool)
	Break       func()
}

// Factory returns an independent terminal initially measuring 93 columns by 31
// rows. Invalid selects an acquisition refusal fixture; empty means valid.
type Factory func(*testing.T, port.FailureKind) Fixture

// Contract holds the same observable lease requirements for fake and native.
func Contract(t *testing.T, factory Factory) {
	t.Helper()
	t.Run("initial_size_and_raw", func(t *testing.T) {
		f := factory(t, "")
		l := acquire(t, f.Provider, context.Background())
		if l.Initial() != (port.Dimensions{Columns: 93, Rows: 31}) || !f.Raw() {
			t.Fatalf("initial dimensions/raw = %v/%v", l.Initial(), f.Raw())
		}
	})
	for _, name := range []string{"normal_restore", "caller_failure_restore"} {
		t.Run(name, func(t *testing.T) {
			f := factory(t, "")
			before := f.Snapshot()
			func() {
				l := acquire(t, f.Provider, context.Background())
				defer l.Close() // also the caller's error-return path
				if name == "caller_failure_restore" {
					return
				}
				result(t, l.Close(), port.Closed, false)
			}()
			if !reflect.DeepEqual(before, f.Snapshot()) {
				t.Fatal("original state was not restored")
			}
		})
	}
	t.Run("precancelled", func(t *testing.T) {
		f := factory(t, "")
		before := f.Snapshot()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		refusal(t, f.Provider, ctx, port.Cancelled)
		if !reflect.DeepEqual(before, f.Snapshot()) {
			t.Fatal("cancelled acquisition changed state")
		}
		l := acquire(t, f.Provider, context.Background())
		result(t, l.Close(), port.Closed, false)
	})
	for _, kind := range []port.FailureKind{port.InvalidTerminal, port.Unsupported, port.InvalidDimensions} {
		t.Run(string(kind), func(t *testing.T) {
			f := factory(t, kind)
			before := f.Snapshot()
			refusal(t, f.Provider, context.Background(), kind)
			if !reflect.DeepEqual(before, f.Snapshot()) {
				t.Fatal("refusal changed terminal state")
			}
		})
	}
	t.Run("busy", func(t *testing.T) {
		f := factory(t, "")
		l := acquire(t, f.Provider, context.Background())
		before := f.Snapshot()
		refusal(t, f.Provider, context.Background(), port.Busy)
		if !reflect.DeepEqual(before, f.Snapshot()) || !f.Raw() {
			t.Fatal("busy acquisition changed active settings")
		}
		result(t, l.Close(), port.Closed, false)
	})
	t.Run("repeated_close_and_closed_streams", func(t *testing.T) {
		f := factory(t, "")
		l := acquire(t, f.Provider, context.Background())
		first := boundedClose(t, l)
		result(t, first, port.Closed, false)
		f.ChangeState()
		after := f.Snapshot()
		if !reflect.DeepEqual(first, l.Close()) || !reflect.DeepEqual(first, l.Result()) {
			t.Fatal("close result changed")
		}
		if !reflect.DeepEqual(after, f.Snapshot()) {
			t.Fatal("repeated close restored settings more than once")
		}
		finished(t, l)
		for range l.Resizes() {
		} // Done must imply closed, including a queued final size
	})
	t.Run("reacquire_new_snapshot", func(t *testing.T) {
		f := factory(t, "")
		l := acquire(t, f.Provider, context.Background())
		l.Close()
		f.ChangeState()
		before := f.Snapshot()
		l = acquire(t, f.Provider, context.Background())
		result(t, l.Close(), port.Closed, false)
		if !reflect.DeepEqual(before, f.Snapshot()) {
			t.Fatal("reacquisition restored an old snapshot")
		}
	})
	t.Run("independent_terminals", func(t *testing.T) {
		a, b := factory(t, ""), factory(t, "")
		la := acquire(t, a.Provider, context.Background())
		lb := acquire(t, b.Provider, context.Background())
		a.Resize(port.Dimensions{Columns: 121, Rows: 41})
		latest(t, la, port.Dimensions{Columns: 121, Rows: 41})
		select {
		case size := <-lb.Resizes():
			t.Fatalf("other terminal resized: %v", size)
		default:
		}
		la.Close()
		if !b.Raw() {
			t.Fatal("closing one terminal changed another")
		}
		result(t, lb.Close(), port.Closed, false)
	})
	t.Run("changed_and_latest_resize", func(t *testing.T) {
		f := factory(t, "")
		l := acquire(t, f.Provider, context.Background())
		f.Resize(l.Initial())
		// Acquisition is the subscription readiness handshake. This bounded
		// absence check verifies that an unchanged observation emits no event.
		select {
		case size := <-l.Resizes():
			t.Fatalf("unchanged size published: %v", size)
		case <-time.After(25 * time.Millisecond):
		}
		f.Resize(port.Dimensions{Columns: 110, Rows: 40})
		latest(t, l, port.Dimensions{Columns: 110, Rows: 40})
		for n := 111; n <= 150; n++ {
			f.Resize(port.Dimensions{Columns: n, Rows: 42})
		}
		latest(t, l, port.Dimensions{Columns: 150, Rows: 42})
	})
	for _, end := range []string{"close", "cancel", "hangup"} {
		t.Run("unread_resize_"+end, func(t *testing.T) {
			f := factory(t, "")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			l := acquire(t, f.Provider, ctx)
			f.Resize(port.Dimensions{Columns: 99, Rows: 50})
			deadline := time.Now().Add(time.Second)
			for len(l.Resizes()) == 0 && time.Now().Before(deadline) {
				runtime.Gosched()
			}
			if len(l.Resizes()) == 0 {
				t.Fatal("unread-event fixture never filled its stream")
			}
			for n := 100; n < 180; n++ {
				f.Resize(port.Dimensions{Columns: n, Rows: 50})
			}
			switch end {
			case "close":
				result(t, boundedClose(t, l), port.Closed, false)
			case "cancel":
				cancel()
				finished(t, l)
				result(t, l.Result(), port.ContextCancelled, false)
			case "hangup":
				f.Hangup(false)
				hangupResult(t, l)
			}
		})
	}
	t.Run("idle_cancellation", func(t *testing.T) {
		f := factory(t, "")
		before := f.Snapshot()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		l := acquire(t, f.Provider, ctx)
		cancel()
		finished(t, l)
		result(t, l.Result(), port.ContextCancelled, false)
		if !errors.Is(l.Result().Err, context.Canceled) {
			t.Fatal("lost context error")
		}
		if !reflect.DeepEqual(before, f.Snapshot()) {
			t.Fatal("cancel did not restore state before Done")
		}
	})
	for _, unread := range []bool{false, true} {
		name := "idle_hangup"
		if unread {
			name = "unread_input_hangup"
		}
		t.Run(name, func(t *testing.T) {
			f := factory(t, "")
			l := acquire(t, f.Provider, context.Background())
			f.Hangup(unread)
			hangupResult(t, l)
		})
	}
	t.Run("invalid_resize", func(t *testing.T) {
		f := factory(t, "")
		before := f.Snapshot()
		l := acquire(t, f.Provider, context.Background())
		f.Resize(port.Dimensions{Columns: 0, Rows: 31})
		finished(t, l)
		result(t, l.Result(), port.ObservationFailed, false)
		if l.Result().Err == nil {
			t.Fatal("observation lost its error")
		}
		if !reflect.DeepEqual(before, f.Snapshot()) {
			t.Fatal("observation failure did not restore settings")
		}
		for size := range l.Resizes() {
			if size.Columns == 0 {
				t.Fatal("published invalid size")
			}
		}
	})
	t.Run("observation_and_restore_failure", func(t *testing.T) {
		f := factory(t, "")
		l := acquire(t, f.Provider, context.Background())
		f.Break()
		finished(t, l)
		first := l.Result()
		result(t, first, port.ObservationFailed, true)
		if first.Err == nil || !reflect.DeepEqual(first, l.Close()) {
			t.Fatal("lost sticky observation/restoration errors")
		}
	})
	t.Run("concurrent_close_cancel", func(t *testing.T) {
		f := factory(t, "")
		before := f.Snapshot()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		l := acquire(t, f.Provider, ctx)
		var wg sync.WaitGroup
		results := make(chan port.Result, 8)
		for range 8 {
			wg.Go(func() { results <- l.Close() })
		}
		cancel()
		finished(t, l)
		wg.Wait()
		close(results)
		first := l.Result()
		if first.Cause != port.Closed && first.Cause != port.ContextCancelled {
			t.Fatalf("unexpected cause: %v", first)
		}
		for r := range results {
			if !reflect.DeepEqual(first, r) {
				t.Fatal("racing closes disagree")
			}
		}
		if !reflect.DeepEqual(before, f.Snapshot()) {
			t.Fatal("racing termination did not restore settings")
		}
	})
}

func acquire(t *testing.T, p port.Provider, ctx context.Context) port.Lease {
	t.Helper()
	l, err := p.Acquire(ctx)
	if err != nil || l == nil {
		t.Fatalf("Acquire must return a usable lease: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

func refusal(t *testing.T, p port.Provider, ctx context.Context, kind port.FailureKind) {
	t.Helper()
	start := time.Now()
	l, err := p.Acquire(ctx)
	if l != nil {
		l.Close()
		t.Fatal("refusal returned a lease")
	}
	var failure *port.Failure
	if !errors.As(err, &failure) || failure.Kind != kind {
		t.Fatalf("refusal = %v; want %s", err, kind)
	}
	if time.Since(start) >= time.Second {
		t.Fatal("refusal took a second")
	}
}

func finished(t *testing.T, l port.Lease) {
	t.Helper()
	select {
	case <-l.Done():
	case <-time.After(time.Second):
		t.Fatal("cleanup did not finish within a second")
	}
}

func boundedClose(t *testing.T, l port.Lease) port.Result {
	t.Helper()
	result := make(chan port.Result, 1)
	go func() { result <- l.Close() }()
	select {
	case r := <-result:
		return r
	case <-time.After(time.Second):
		t.Fatal("close exceeded a second")
		return port.Result{}
	}
}

func result(t *testing.T, r port.Result, cause port.Cause, restoreFailure bool) {
	t.Helper()
	if r.Cause != cause || (r.RestoreErr != nil) != restoreFailure {
		t.Fatalf("result = %+v; want cause %s, restoration failure %v", r, cause, restoreFailure)
	}
}

func hangupResult(t *testing.T, l port.Lease) {
	t.Helper()
	finished(t, l)
	first := l.Result()
	if first.Cause != port.Hangup {
		t.Fatalf("result = %+v; want cause %s", first, port.Hangup)
	}
	// Hangup does not imply restoration failure: the OS may still accept the
	// saved settings. Either outcome must remain unchanged after completion.
	for range 2 {
		if !reflect.DeepEqual(first, boundedClose(t, l)) || !reflect.DeepEqual(first, l.Result()) {
			t.Fatal("close changed the hangup/restoration result")
		}
	}
}

func latest(t *testing.T, l port.Lease, want port.Dimensions) {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case size, ok := <-l.Resizes():
			if !ok {
				t.Fatal("resize stream closed before latest size")
			}
			if size == want {
				return
			}
		case <-timer.C:
			t.Fatalf("latest size %v was not observable", want)
		}
	}
}
