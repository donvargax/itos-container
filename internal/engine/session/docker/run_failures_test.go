package docker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"

	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestRunCreateOverridesImageDefaultsWithoutChangingLiteralArgv(t *testing.T) {
	requests := make(chan container.Config, 1)
	engine, binding, _ := sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, b sp.Binding) {
		switch strings.TrimPrefix(r.URL.Path, "/v1.56") {
		case "/images/" + b.ImageID + "/json":
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": b.ImageID, "Config": container.Config{
				User: "0:0", WorkingDir: "/image", Entrypoint: []string{"image-wrapper"}, Cmd: []string{"image-command"},
				Env: []string{"PATH=/image/bin", "HOME=/root", "TMPDIR=/image/tmp", "APP=image", "IMAGE_DEFAULT=retained"},
			}})
		case "/containers/create":
			var config container.Config
			if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			requests <- config
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": strings.Repeat("c", 64)})
		default:
			t.Error("unexpected request", r.Method, r.URL.Path)
			w.WriteHeader(400)
		}
	})
	binding.Workspace = "/workspace/subdir"
	binding.Environment = `{"APP":"$(touch marker);$HOST_ONLY"}`
	ctx := context.Background()
	connection, failure := engine.OpenRun(ctx, binding)
	if failure != "" {
		t.Fatal(failure)
	}
	defer connection.Close()
	argv := []string{"probe", "", "two words", "\t\n雪", "$HOME", "--json", "--"}
	id, failure := connection.CreateRun(ctx, binding.Intent(strings.Repeat("1", 64)), argv)
	if failure != "" || id != strings.Repeat("c", 64) {
		t.Fatal(id, failure)
	}
	config := <-requests
	// Moby distinguishes an explicit empty entrypoint from nil (image default):
	// https://github.com/moby/moby/blob/v28.5.2/daemon/commit.go#L68-L75
	if config.Entrypoint == nil || len(config.Entrypoint) != 0 || !slices.Equal(config.Cmd, argv) {
		t.Fatal("image command can precede or replace literal argv", config.Entrypoint, config.Cmd)
	}
	if config.Image != binding.ImageID || config.User != binding.UID+":"+binding.GID || config.WorkingDir != binding.Workspace {
		t.Fatal("create inherited image identity or workspace", config)
	}
	wantEnv := []string{"APP=$(touch marker);$HOST_ONLY", "HOME=/tmp", "IMAGE_DEFAULT=retained", "PATH=/image/bin", "TMPDIR=/tmp"}
	if !slices.Equal(config.Env, wantEnv) || config.Tty || !config.OpenStdin || !config.StdinOnce || !config.AttachStdin || !config.AttachStdout || !config.AttachStderr {
		t.Fatal("create changed literal environment or stream configuration", config)
	}
	if config.Healthcheck == nil || !slices.Equal(config.Healthcheck.Test, []string{"NONE"}) {
		t.Fatal("image healthcheck may run alongside the workload")
	}
}

func TestRunCreateRefusesUnverifiedImage(t *testing.T) {
	for _, tc := range []struct {
		condition string
		want      sp.Outcome
	}{
		{"declared volume", sp.Refused},
		{"different image config ID", sp.Refused},
		{"missing Config", sp.Uncertain},
		{"unavailable", sp.Uncertain},
		{"malformed response", sp.Uncertain},
	} {
		t.Run(tc.condition, func(t *testing.T) {
			// This fixture rejects any create request after an unsafe image read.
			e, b, _ := sdkFixture(t, tc.condition)
			c, failure := e.OpenRun(context.Background(), b)
			if failure != "" {
				t.Fatal(failure)
			}
			defer c.Close()
			id, failure := c.CreateRun(context.Background(), b.Intent(strings.Repeat("1", 64)), []string{"probe"})
			if id != "" || failure != tc.want {
				t.Fatal(id, failure)
			}
		})
	}
}

func TestRunOperationsRefuseReplacedEngineSocket(t *testing.T) {
	e, b, pin := sdkHandlerFixture(t, func(_ http.ResponseWriter, r *http.Request, _ sp.Binding) {
		t.Error("contacted a replaced engine socket", r.Method, r.URL.Path)
	})
	ctx := context.Background()
	c, failure := e.OpenRun(ctx, b)
	if failure != "" {
		t.Fatal(failure)
	}
	defer c.Close()
	pin.Current = false
	id := strings.Repeat("c", 64)
	if created, failure := c.CreateRun(ctx, b.Intent(strings.Repeat("1", 64)), []string{"probe"}); created != "" || failure != sp.Refused {
		t.Fatal(created, failure)
	}
	if failure := c.VerifyRun(ctx, id, []string{"probe"}); failure != sp.Refused {
		t.Fatal(failure)
	}
	if attached, failure := c.Attach(ctx, id, sp.Streams{}); attached != nil || failure != sp.ExecutionFailed {
		t.Fatal(attached, failure)
	}
	if failure := c.Start(ctx, id); failure != sp.Refused {
		t.Fatal(failure)
	}
	if failure := c.Signal(ctx, id, "SIGTERM"); failure != sp.Refused {
		t.Fatal(failure)
	}
	if exit, failure := c.Wait(ctx, id); exit != 0 || failure != sp.Refused {
		t.Fatal(exit, failure)
	}
	c.RemoveRun(ctx, id)
}

func TestRunWaitRequiresAValidObservedExit(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		wantExit   int
		want       sp.Outcome
	}{
		{"success", `{"StatusCode":0}`, 200, 0, ""},
		{"nonzero", `{"StatusCode":75}`, 200, 75, ""},
		{"largest exit", `{"StatusCode":255}`, 200, 255, ""},
		{"negative exit", `{"StatusCode":-1}`, 200, 0, sp.ExecutionFailed},
		{"invalid exit", `{"StatusCode":256}`, 200, 0, sp.ExecutionFailed},
		{"daemon error", `{"StatusCode":0,"Error":{"Message":"synthetic daemon failure"}}`, 200, 0, sp.ExecutionFailed},
		{"transport error", `{"message":"synthetic response"}`, 503, 0, sp.ExecutionFailed},
		{"malformed", `{"StatusCode":`, 200, 0, sp.ExecutionFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, b, _ := sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, _ sp.Binding) {
				if !strings.HasSuffix(r.URL.Path, "/wait") || r.Method != http.MethodPost || r.URL.Query().Get("condition") != "not-running" {
					t.Error("unexpected wait request", r.Method, r.URL)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			c, failure := e.OpenRun(ctx, b)
			if failure != "" {
				t.Fatal(failure)
			}
			defer c.Close()
			exit, failure := c.Wait(ctx, strings.Repeat("c", 64))
			if exit != tc.wantExit || failure != tc.want {
				t.Fatal(exit, failure)
			}
		})
	}
}

func TestRunWaitOutlivesStartupContext(t *testing.T) {
	requested := make(chan struct{})
	release := make(chan struct{})
	e, b, _ := sdkHandlerFixture(t, func(w http.ResponseWriter, _ *http.Request, _ sp.Binding) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		close(requested)
		<-release
		_, _ = w.Write([]byte(`{"StatusCode":7}`))
	})
	startup, cancelStartup := context.WithCancel(context.Background())
	defer cancelStartup()
	c, failure := e.OpenRun(startup, b)
	if failure != "" {
		t.Fatal(failure)
	}
	defer c.Close()
	cancelStartup()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		select {
		case <-requested:
		case <-ctx.Done():
		}
		close(release)
	}()
	if exit, failure := c.Wait(ctx, strings.Repeat("c", 64)); exit != 7 || failure != "" {
		t.Fatal("workload wait inherited the canceled startup context", exit, failure)
	}
}

type failedOutput struct{}

func (failedOutput) Write([]byte) (int, error) { return 0, errors.New("synthetic output failure") }

func TestRunDrainReportsOutputFailure(t *testing.T) {
	e, b, _ := runSDKFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c, failure := e.OpenRun(ctx, b)
	if failure != "" {
		t.Fatal(failure)
	}
	defer c.Close()
	id, failure := c.CreateRun(ctx, b.Intent(strings.Repeat("1", 64)), []string{"probe"})
	if failure != "" {
		t.Fatal(failure)
	}
	a, failure := c.Attach(ctx, id, sp.Streams{Out: failedOutput{}})
	if failure != "" {
		t.Fatal(failure)
	}
	defer a.Close()
	if failure := c.Start(ctx, id); failure != "" {
		t.Fatal(failure)
	}
	if failure := a.Drain(ctx); failure != sp.ExecutionFailed {
		t.Fatal("lost stdout was reported as a complete drain", failure)
	}
}
