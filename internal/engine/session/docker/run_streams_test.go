package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

// Each stream independently releases the caller only after its readiness bytes
// arrive. A buffered-until-exit implementation cannot complete the handshake.
type readyStream struct {
	mu     sync.Mutex
	data   bytes.Buffer
	prefix []byte
	ready  chan struct{}
	once   sync.Once
}

func (s *readyStream) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, err := s.data.Write(p)
	if bytes.HasPrefix(s.data.Bytes(), s.prefix) {
		s.once.Do(func() { close(s.ready) })
	}
	return n, err
}

func (s *readyStream) bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return bytes.Clone(s.data.Bytes())
}

func TestSDKLiveHandshakeSurvivesStartupAndDrainsAfterExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	readyOut, readyErr := []byte("ready-out\x00\xff"), []byte("ready-err\x00\xfe")
	tailOut, tailErr := []byte("tail-out\x00\xfd"), []byte("tail-err\x00\xfc")
	input := []byte("input\x00\xfb\n")
	out := &readyStream{prefix: readyOut, ready: make(chan struct{})}
	errout := &readyStream{prefix: readyErr, ready: make(chan struct{})}
	started, outputAllowed, tailAllowed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	inputDone := make(chan struct{})
	fixtureDone := make(chan error, 1)
	id := strings.Repeat("c", 64)
	e, b, _ := sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, _ sp.Binding) {
		switch strings.TrimPrefix(r.URL.Path, "/v1.56") {
		case "/containers/" + id + "/attach":
			for _, key := range []string{"stream", "stdin", "stdout", "stderr"} {
				if r.URL.Query().Get(key) != "1" {
					t.Errorf("attach omitted %s", key)
				}
			}
			conn := hijackRunStream(t, w)
			if conn == nil {
				return
			}
			defer func() { _ = conn.Close() }()
			deadline, _ := ctx.Deadline()
			_ = conn.SetDeadline(deadline)
			for _, barrier := range []<-chan struct{}{started, outputAllowed} {
				select {
				case <-barrier:
				case <-ctx.Done():
					fixtureDone <- ctx.Err()
					return
				}
			}
			writeFrame(conn, 1, readyOut)
			writeFrame(conn, 2, readyErr)
			received, err := io.ReadAll(conn)
			if err != nil || !bytes.Equal(received, input) {
				fixtureDone <- fmt.Errorf("worker stdin or EOF changed: %q, %v", received, err)
				return
			}
			close(inputDone)
			select {
			case <-tailAllowed:
			case <-ctx.Done():
				fixtureDone <- ctx.Err()
				return
			}
			// These arrive only after Wait has returned the workload's exit.
			writeFrame(conn, 1, tailOut)
			writeFrame(conn, 2, tailErr)
			fixtureDone <- nil
		case "/containers/" + id + "/start":
			close(started)
			w.WriteHeader(http.StatusNoContent)
		case "/containers/" + id + "/wait":
			select {
			case <-inputDone:
				_, _ = io.WriteString(w, `{"StatusCode":7}`)
			case <-ctx.Done():
				w.WriteHeader(http.StatusGatewayTimeout)
			}
		default:
			t.Error("unexpected stream fixture request", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	startup, cancelStartup := context.WithCancel(ctx)
	defer cancelStartup()
	c, failure := e.OpenRun(startup, b)
	if failure != "" {
		t.Fatal(failure)
	}
	defer c.Close()
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close(); _ = writer.Close() }()
	a, failure := c.Attach(startup, id, sp.Streams{In: reader, Out: out, Err: errout})
	if failure != "" {
		t.Fatal(failure)
	}
	defer a.Close()
	if failure = c.Start(startup, id); failure != "" {
		t.Fatal(failure)
	}
	cancelStartup()
	close(outputAllowed)
	for _, ready := range []<-chan struct{}{out.ready, errout.ready} {
		select {
		case <-ready:
		case <-ctx.Done():
			t.Fatal("both readiness streams did not arrive before the stdin reply")
		}
	}
	inputSent := make(chan error, 1)
	go func() {
		_, err := writer.Write(input)
		_ = writer.Close()
		inputSent <- err
	}()
	select {
	case err := <-inputSent:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("live stdin was not consumed")
	}
	if exit, failure := c.Wait(ctx, id); failure != "" || exit != 7 {
		t.Fatal("exit was lost after startup cancellation or stdin EOF", exit, failure)
	}
	// Output is still open after the exit response; a canceled drain cannot
	// claim it is complete, nor consume the eventual successful drain result.
	expired, expire := context.WithCancel(ctx)
	expire()
	if failure := a.Drain(expired); failure != sp.ExecutionFailed {
		t.Fatal("unfinished output was reported complete", failure)
	}
	close(tailAllowed)
	if failure := a.Drain(ctx); failure != "" {
		t.Fatal("trailing output did not drain", failure)
	}
	if err := <-fixtureDone; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.bytes(), append(bytes.Clone(readyOut), tailOut...)) ||
		!bytes.Equal(errout.bytes(), append(bytes.Clone(readyErr), tailErr...)) {
		t.Fatalf("separate binary streams changed: stdout %q, stderr %q", out.bytes(), errout.bytes())
	}
}

func hijackRunStream(t *testing.T, w http.ResponseWriter) net.Conn {
	t.Helper()
	conn, buffer, err := w.(http.Hijacker).Hijack()
	if err != nil {
		t.Error(err)
		return nil
	}
	_, err = buffer.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
	if err == nil {
		err = buffer.Flush()
	}
	if err != nil {
		_ = conn.Close()
		t.Error(err)
		return nil
	}
	return conn
}

func TestSDKAttachAndFrameFailuresAreNotSuccessfulDrains(t *testing.T) {
	for _, fault := range []string{"attach response", "stream frame"} {
		t.Run(fault, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			e, b, _ := sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, _ sp.Binding) {
				if !strings.HasSuffix(r.URL.Path, "/attach") {
					t.Error("unexpected request", r.Method, r.URL.Path)
				}
				if fault == "attach response" {
					http.Error(w, "synthetic transport failure", http.StatusServiceUnavailable)
					return
				}
				conn := hijackRunStream(t, w)
				if conn == nil {
					return
				}
				defer func() { _ = conn.Close() }()
				writeFrame(conn, 42, []byte("invalid stream type"))
			})
			c, failure := e.OpenRun(ctx, b)
			if failure != "" {
				t.Fatal(failure)
			}
			defer c.Close()
			a, failure := c.Attach(ctx, strings.Repeat("c", 64), sp.Streams{})
			if fault == "attach response" {
				if failure != sp.ExecutionFailed || a != nil {
					t.Fatal("failed attach returned a usable stream", a, failure)
				}
				return
			}
			if failure != "" {
				t.Fatal(failure)
			}
			defer a.Close()
			if failure := a.Drain(ctx); failure != sp.ExecutionFailed {
				t.Fatal("malformed stream was reported complete", failure)
			}
		})
	}
}

func TestSDKStdinHalfCloseAfterOutputEOFIsNotAForwardingFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	e, b, _ := sdkHandlerFixture(t, func(w http.ResponseWriter, _ *http.Request, _ sp.Binding) {
		if conn := hijackRunStream(t, w); conn != nil {
			_ = conn.Close()
		}
	})
	c, failure := e.OpenRun(ctx, b)
	if failure != "" {
		t.Fatal(failure)
	}
	defer c.Close()
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close(); _ = writer.Close() }()
	a, failure := c.Attach(ctx, strings.Repeat("c", 64), sp.Streams{In: reader})
	if failure != "" {
		t.Fatal(failure)
	}
	defer a.Close()
	if failure = a.Drain(ctx); failure != "" {
		t.Fatal("worker output EOF failed", failure)
	}
	// Input is still awaiting its source's EOF when the completed attachment
	// closes. Release EOF only now, so CloseWrite deterministically fails.
	a.Close()
	_ = writer.Close()
	select {
	case failure = <-a.Failures():
		if failure != sp.StdinClosed {
			t.Fatal("late half-close erased successful output", failure)
		}
	case <-ctx.Done():
		t.Fatal("half-close failure was not reported")
	}
	if failure = a.Drain(ctx); failure != "" {
		t.Fatal("late half-close left a sticky forwarding failure", failure)
	}
}

func TestStdinClosureDoesNotEndOutputDrain(t *testing.T) {
	for _, outputFailure := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			a := &attachment{done: make(chan struct{}), failures: make(chan sp.Outcome, 1)}
			a.notify(sp.StdinClosed)
			drained := make(chan sp.Outcome, 1)
			go func() { drained <- a.Drain(context.Background()) }()
			synctest.Wait()
			select {
			case failure := <-drained:
				t.Fatal("stdin closure completed unfinished output", failure)
			default:
			}
			want := sp.Outcome("")
			if outputFailure {
				a.fail()
				want = sp.ExecutionFailed
			} else {
				close(a.done)
			}
			if failure := <-drained; failure != want {
				t.Fatal("output result was lost after stdin closure", failure)
			}
		})
	}
}
