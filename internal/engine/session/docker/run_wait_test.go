package docker

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/client"

	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

// The pinned SDK performs the HTTP handshake synchronously, then reads the
// response body asynchronously. Exercise both sides through its real transport:
// the controller must bound a silent daemon without imposing a workload limit.
func TestRunWaitBoundsHandshakeWithoutBoundingWorkload(t *testing.T) {
	for _, acknowledged := range []bool{false, true} {
		name := "silent handshake"
		if acknowledged {
			name = "acknowledged workload"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			requested := make(chan struct{})
			finished := make(chan struct{})
			id := strings.Repeat("c", 64)
			e, b, _ := sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, _ sp.Binding) {
				defer close(finished)
				if r.Method != http.MethodPost || r.URL.Path != "/v1.56/containers/"+id+"/wait" || r.URL.Query().Get("condition") != "not-running" {
					t.Error("unexpected wait request", r.Method, r.URL)
				}
				close(requested)
				if !acknowledged {
					<-r.Context().Done()
					return
				}
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				select {
				case <-time.After(11 * time.Second):
					_, _ = w.Write([]byte(`{"StatusCode":23}`))
				case <-r.Context().Done():
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			c, failure := e.OpenRun(ctx, b)
			if failure != "" {
				t.Fatal(failure)
			}
			defer c.Close()
			start := time.Now()
			exit, failure := c.Wait(ctx, id)
			elapsed := time.Since(start)
			if ctx.Err() != nil {
				t.Fatal("wait exhausted the outer test budget instead of its handshake budget")
			}
			select {
			case <-requested:
			default:
				t.Fatal("wait never reached the engine fixture")
			}
			if acknowledged {
				if exit != 23 || failure != "" || elapsed < 11*time.Second {
					t.Fatalf("acknowledged workload wait = %d, %s after %v", exit, failure, elapsed)
				}
			} else if exit != 0 || failure != sp.ExecutionFailed || elapsed < 10*time.Second || elapsed >= 13*time.Second {
				t.Fatalf("silent handshake = %d, %s after %v; want failure at ten seconds", exit, failure, elapsed)
			}
			select {
			case <-finished:
			case <-ctx.Done():
				t.Fatal("wait left its HTTP request pending")
			}
		})
	}
}

func TestRunWaitAcknowledgedStreamHonorsCallerCancellation(t *testing.T) {
	acknowledged := make(chan struct{})
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	finished := make(chan struct{})
	e, b, _ := sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, _ sp.Binding) {
		defer close(finished)
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, failure := e.OpenRun(ctx, b)
	if failure != "" {
		t.Fatal(failure)
	}
	defer c.Close()
	// Delay the SDK's body read after real response headers have arrived. This
	// makes cancellation independent of the transport's eventual read error.
	run := c.(*runConnection)
	_ = run.api.Close()
	api, err := client.New(client.WithHost(b.Endpoint), client.WithAPIVersion("1.56"),
		client.WithHTTPClient(&http.Client{Transport: delayedWaitTransport{run.transport, acknowledged, release}}))
	if err != nil {
		t.Fatal(err)
	}
	run.api = api
	result := make(chan sp.Outcome, 1)
	go func() {
		_, outcome := c.Wait(ctx, strings.Repeat("c", 64))
		result <- outcome
	}()
	select {
	case <-acknowledged:
	case <-time.After(time.Second):
		t.Fatal("wait never acknowledged")
	}
	cancel()
	select {
	case outcome := <-result:
		if outcome != sp.ExecutionFailed {
			t.Fatal("canceled wait reported a worker exit", outcome)
		}
	case <-time.After(time.Second):
		t.Fatal("acknowledged wait ignored caller cancellation")
	}
	unblock()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("canceled wait left its HTTP request pending")
	}
}

type delayedWaitTransport struct {
	http.RoundTripper
	reading chan struct{}
	release <-chan struct{}
}

func (t delayedWaitTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := t.RoundTripper.RoundTrip(r)
	if err == nil {
		response.Body = &delayedWaitBody{ReadCloser: response.Body, reading: t.reading, release: t.release}
	}
	return response, err
}

type delayedWaitBody struct {
	io.ReadCloser
	reading chan struct{}
	release <-chan struct{}
	once    sync.Once
}

func (b *delayedWaitBody) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.reading) })
	<-b.release
	return b.ReadCloser.Read(p)
}
