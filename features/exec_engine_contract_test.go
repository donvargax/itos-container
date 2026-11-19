package features

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
)

// Exercise the recorder with the pinned client, independently of the future
// exec command. A malformed upgrade or inspect body would otherwise look like
// a product failure when the red rows first move past launch.
func TestExecRecorderWireContract(t *testing.T) {
	worker := strings.Repeat("b", 64)
	w := &world{doctor: &doctorWorld{}, exec: &execWorld{worker: worker, execStdout: "out", execStderr: "err", execExit: 23}}
	api := &sessionAPI{w: w, execs: map[string]map[string]any{}}
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		q := sessionRequest{Method: r.Method, Path: doctorRequestPath(r.URL.Path)}
		if r.Method == "POST" {
			if err := json.NewDecoder(r.Body).Decode(&q.Body); err != nil {
				t.Error(err)
				return
			}
		}
		api.mu.Lock()
		defer api.mu.Unlock()
		api.requests = append(api.requests, q)
		index := len(api.requests) - 1
		if q.Path == "/containers/"+worker+"/exec" {
			api.serveExecCreate(out, r, q, index)
			return
		}
		api.serveExec(out, r, q, index)
	}))
	defer server.Close()
	cli, err := client.New(client.WithHost("tcp://"+server.Listener.Addr().String()), client.WithAPIVersion("1.48"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cli.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	created, err := cli.ExecCreate(ctx, worker, client.ExecCreateOptions{Cmd: []string{nativeExecProbe, "two words", ""}, AttachStdin: true, AttachStdout: true, AttachStderr: true})
	if err != nil {
		t.Fatal(err)
	}
	inspected, err := cli.ExecInspect(ctx, created.ID, client.ExecInspectOptions{})
	if err != nil || inspected.ContainerID != worker || inspected.ID != created.ID || inspected.Running {
		t.Fatalf("pre-start inspection = %+v, %v", inspected, err)
	}
	attach, err := cli.ExecAttach(ctx, created.ID, client.ExecAttachOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer attach.Close()
	var out, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&out, &stderr, attach.Reader); err != nil {
		t.Fatal(err)
	}
	inspected, err = cli.ExecInspect(ctx, created.ID, client.ExecInspectOptions{})
	if err != nil || inspected.Running || inspected.ExitCode != 23 || out.String() != "out" || stderr.String() != "err" {
		t.Fatalf("final inspection = %+v, stdout %q, stderr %q, error %v", inspected, out.String(), stderr.String(), err)
	}
}
