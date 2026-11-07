package features

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type lifecycleWaitBody struct {
	io.ReadCloser
	data     bytes.Buffer
	once     sync.Once
	finished func([]byte, error)
}

func (b *lifecycleWaitBody) Read(data []byte) (int, error) {
	n, err := b.ReadCloser.Read(data)
	if b.data.Len()+n <= 65536 {
		_, _ = b.data.Write(data[:n])
	}
	if err != nil {
		b.once.Do(func() { b.finished(b.data.Bytes(), err) })
	}
	return n, err
}

// Faults affect only the controller-facing recorder. The independent observer
// retains its original native transport, and every mutation still passes the
// recorder's protected-record and full-resource-identity checks first.
type lifecycleFaults struct {
	mu            sync.Mutex
	mode, barrier string
	delay         time.Duration
	lost, held    bool
	removed       bool
	ready         chan struct{}
	release       chan struct{}
	releaseOnce   sync.Once
	waits         map[*http.Request]context.CancelFunc
}

func newLifecycleFaults(mode, barrier string, delay time.Duration) *lifecycleFaults {
	return &lifecycleFaults{mode: mode, barrier: barrier, delay: delay,
		ready: make(chan struct{}), release: make(chan struct{}), waits: map[*http.Request]context.CancelFunc{}}
}

func (f *lifecycleFaults) request(r *http.Request) (*http.Request, context.CancelFunc) {
	ctx, cancel := context.WithCancel(r.Context())
	r = r.WithContext(ctx)
	f.mu.Lock()
	f.waits[r] = cancel
	if f.lost {
		cancel()
	}
	f.mu.Unlock()
	return r, func() {
		cancel()
		f.mu.Lock()
		delete(f.waits, r)
		f.mu.Unlock()
	}
}

func (f *lifecycleFaults) disconnect() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lost = true
	for _, cancel := range f.waits {
		cancel()
	}
}

func (f *lifecycleFaults) restore() {
	f.mu.Lock()
	f.lost = false
	f.mode = "restored"
	f.mu.Unlock()
	f.releaseBarrier()
}

// Finish the controlled overlap without restoring the injected failure. A
// cleanup-signals DELETE still receives 503 until a separate restore call.
func (f *lifecycleFaults) releaseBarrier() {
	f.releaseOnce.Do(func() { close(f.release) })
}

func (f *lifecycleFaults) hold(ctx context.Context) {
	f.mu.Lock()
	if f.held {
		f.mu.Unlock()
		return
	}
	f.held = true
	close(f.ready)
	f.mu.Unlock()
	select {
	case <-ctx.Done():
	case <-f.release:
	}
}

func (f *lifecycleFaults) before(out http.ResponseWriter, r *http.Request, e offlineRequest) bool {
	f.mu.Lock()
	lost, mode, barrier := f.lost, f.mode, f.barrier
	f.mu.Unlock()
	if lost {
		// Lose the connection, not the native daemon. Neither a fabricated exit
		// nor a fabricated not-found reply can satisfy a product observation.
		if hijack, ok := out.(http.Hijacker); ok {
			conn, _, err := hijack.Hijack()
			if err == nil {
				_ = conn.Close()
				return true
			}
		}
		http.Error(out, "SLICE_12_SYNTHETIC_DISCONNECT", http.StatusServiceUnavailable)
		return true
	}
	if e.Method != http.MethodDelete {
		return false
	}
	if mode == "cleanup-signals" {
		f.hold(r.Context())
		// All attempts remain unavailable until the first invocation returned.
		select {
		case <-r.Context().Done():
		case <-f.release:
		}
		http.Error(out, "SLICE_12_SYNTHETIC_REMOVE", http.StatusServiceUnavailable)
		return true
	}
	if mode == "crash" && barrier == "during removal before its reply" {
		f.hold(r.Context())
		return r.Context().Err() != nil
	}
	return false
}

func (f *lifecycleFaults) response(r *http.Request, e offlineRequest, status int) {
	f.mu.Lock()
	mode, barrier, delay := f.mode, f.barrier, f.delay
	if e.Method == http.MethodDelete && status == http.StatusNoContent {
		f.removed = true
	}
	removed := f.removed
	f.mu.Unlock()
	if e.Method == http.MethodPost && strings.HasSuffix(e.Path, "/kill") &&
		r.URL.Query().Get("signal") != "SIGKILL" && status == http.StatusNoContent && delay > 0 {
		// Delivery already succeeded at the daemon. Only its acknowledgement
		// is delayed; a probe receipt independently establishes actual delivery.
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-r.Context().Done():
		}
	}
	if mode != "crash" {
		return
	}
	match := (barrier == "after accepted create before reply" && e.Path == "/containers/create" && status == http.StatusCreated) ||
		(barrier == "after accepted start before reply" && strings.HasSuffix(e.Path, "/start") && status == http.StatusNoContent) ||
		(barrier == "after removal before durable absence update" && removed && e.Method == http.MethodGet &&
			strings.HasPrefix(e.Path, "/containers/") && strings.HasSuffix(e.Path, "/json") && status == http.StatusNotFound)
	if match {
		f.hold(r.Context())
	}
}
