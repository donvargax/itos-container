package domain

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestFiniteBoundaries(t *testing.T) {
	for _, delay := range []time.Duration{10*time.Second - time.Nanosecond, 10 * time.Second, 10*time.Second + time.Nanosecond} {
		t.Run(delay.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, end := Start(context.Background(), 120*time.Second, "startup")
				defer end()
				start := time.Now()
				value, err := Call(ctx, "inspection", func(call context.Context) int {
					deadline, _ := call.Deadline()
					if deadline.Sub(start) != 10*time.Second {
						t.Error("call budget", deadline.Sub(start))
					}
					time.Sleep(delay)
					return 17
				})
				if delay < 10*time.Second {
					if err != nil || value != 17 || ctx.Err() != nil {
						t.Error(value, err)
					}
				} else {
					var failure Failure
					if !errors.As(err, &failure) || failure.Operation != "inspection" || failure.Phase != "" || time.Since(start) != 10*time.Second {
						t.Error(value, err, time.Since(start))
					}
					if context.Cause(ctx) != err {
						t.Error("lost original cause")
					}
				}
				time.Sleep(time.Second)
				synctest.Wait()
			})
		})
	}
}
func TestPhaseWinsAtCoincidentBoundary(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, end := Start(context.Background(), 10*time.Second, "startup")
		defer end()
		_, err := Call(ctx, "start", func(call context.Context) int { <-call.Done(); return 7 })
		var f Failure
		if !errors.As(err, &f) || f.Phase != "startup" {
			t.Fatal(err)
		}
	})
}
func TestRemainingPhaseAndLateHandle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, end := Start(context.Background(), 120*time.Second, "startup")
		defer end()
		time.Sleep(115 * time.Second)
		release, discarded := make(chan struct{}), make(chan int, 1)
		r, err := Acquire(ctx, "attach", func(call context.Context) int {
			d, _ := call.Deadline()
			if time.Until(d) != 5*time.Second {
				t.Error("phase reset")
			}
			<-release
			return 23
		}, func(r int) { discarded <- r })
		if r != 0 || err == nil {
			t.Fatal(r, err)
		}
		close(release)
		if <-discarded != 23 {
			t.Fatal("late handle leaked")
		}
		called := false
		_, err = Call(ctx, "start", func(context.Context) bool { called = true; return true })
		if err == nil || called {
			t.Fatal("late completion resumed launch")
		}
	})
}
func TestCallerCancellationAndHandleLifetime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		parent, cancel := context.WithCancelCause(context.Background())
		cause := errors.New("first cancellation")
		phase, end := Start(parent, 120*time.Second, "startup")
		defer end()
		ctx, err := Call(phase, "open", func(call context.Context) context.Context { return Lifetime(call) })
		if err != nil || ctx.Err() != nil {
			t.Fatal("establishment ended handle lifetime", err)
		}
		a, b, err := Call2(ctx, "read", func(context.Context) (int, string) { return 7, "ok" })
		if a != 7 || b != "ok" || err != nil {
			t.Fatal(a, b, err)
		}
		go func() { time.Sleep(time.Second); cancel(cause) }()
		_, err = Call(phase, "inspect", func(call context.Context) bool { <-call.Done(); return false })
		if !errors.Is(err, cause) {
			t.Fatal(err)
		}
		if Lifetime(context.Background()) != context.Background() {
			t.Fatal("unscoped lifetime changed")
		}
		attached := WithStartup(parent, phase)
		selected, finish := Startup(attached)
		finish()
		if selected != phase {
			t.Fatal("startup restarted")
		}
		fresh, stop := Startup(context.Background())
		defer stop()
		d, _ := fresh.Deadline()
		if time.Until(d) != 120*time.Second {
			t.Fatal("default startup", time.Until(d))
		}
	})
}
