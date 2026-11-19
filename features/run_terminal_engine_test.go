package features

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const terminalPayload = "slice-37 stdout\n\r\n\r\x01\x00\x00\x00\x00\x00\x00\x03raw\xff"
const terminalErrPayload = "slice-37 stderr"
const terminalTailPayload = "slice-37 tail\n\r\n\r\x01\x00tail"

// The local recorder never executes a workload. Framing follows the actual
// create request, so absent TTY selection cannot masquerade as a terminal.
type terminalEngine struct {
	fault               string
	exit                int
	hold                bool
	expect              int
	killDelay           time.Duration
	release             chan struct{}
	ready               chan struct{}
	started             sync.Once
	finished            sync.Once
	input               string
	resizes             []string
	tailStart, tailRead chan struct{}
	tailOnce            sync.Once
}

func newTerminalEngine() *terminalEngine {
	return &terminalEngine{release: make(chan struct{}), ready: make(chan struct{})}
}

func (f *terminalEngine) finish() { f.finished.Do(func() { close(f.release) }) }

// Called with the recorder lock held, just like its ordinary request handlers.
func (f *terminalEngine) serve(a *localOfflineAPI, out http.ResponseWriter, r *http.Request, id string, resource map[string]any) bool {
	path := doctorRequestPath(r.URL.Path)
	switch {
	case strings.HasSuffix(path, "/json") && r.Method == "GET":
		if f.fault == "created worker reports TTY disabled" || f.fault == "created worker reports different initial size" {
			copy := approvalClone(resource)
			if f.fault == "created worker reports TTY disabled" {
				object(copy["Config"])["Tty"] = false
			} else {
				object(copy["HostConfig"])["ConsoleSize"] = []int{23, 79}
			}
			_ = json.NewEncoder(out).Encode(copy)
			return true
		}
	case strings.HasSuffix(path, "/attach") && r.Method == "POST":
		if f.fault == "attach fails before start" {
			http.Error(out, "fixture attachment failure", 500)
			return true
		}
	case strings.HasSuffix(path, "/start") && r.Method == "POST":
		conn := a.attached[id]
		if conn == nil {
			return false
		}
		token, _ := object(object(resource["Config"])["Labels"])["io.itos-container.intent"].(string)
		if err := a.record(token, "owned", id); err != nil {
			a.problem = fmt.Errorf("terminal start without durable ownership: %w", err)
			http.Error(out, "unowned start", 400)
			return true
		}
		a.started[id] = true
		resource["State"] = map[string]any{"Status": "running", "Running": true, "Pid": 123, "StartedAt": "2026-01-01T00:00:00Z"}
		tty := object(resource["Config"])["Tty"] == true
		go func() {
			if tty {
				_, _ = io.WriteString(conn, terminalPayload+terminalErrPayload)
			} else {
				for i, data := range []string{terminalPayload, terminalErrPayload} {
					frame := make([]byte, 8)
					frame[0] = byte(i + 1)
					binary.BigEndian.PutUint32(frame[4:], uint32(len(data)))
					_, _ = conn.Write(append(frame, data...))
				}
			}
			f.started.Do(func() { close(f.ready) })
			if f.hold {
				expect := f.expect
				if expect == 0 {
					expect = 3
				}
				input := make([]byte, expect)
				_, err := io.ReadFull(conn, input)
				a.mu.Lock()
				if err == nil {
					f.input = string(input)
				}
				a.mu.Unlock()
				if err == nil {
					_, _ = io.WriteString(conn, "progress")
				}
				<-f.release
			}
			if f.tailStart != nil {
				<-f.tailStart
				_, _ = io.WriteString(conn, terminalTailPayload)
			}
			// The hijacked endpoint may be a raw socket or the TTY escape model.
			if closer, ok := conn.(interface{ CloseWrite() error }); ok {
				_ = closer.CloseWrite()
			}
		}()
		if f.fault == "start accepted but its reply times out" {
			a.mu.Unlock()
			<-r.Context().Done()
			a.mu.Lock()
			return true
		}
		out.WriteHeader(204)
		return true
	case strings.HasSuffix(path, "/resize") && r.Method == "POST":
		f.resizes = append(f.resizes, id+":"+r.URL.Query().Get("w")+"x"+r.URL.Query().Get("h"))
		out.WriteHeader(200)
		return true
	case strings.HasSuffix(path, "/wait") && r.Method == "POST" && a.started[id]:
		out.WriteHeader(200)
		out.(http.Flusher).Flush()
		a.mu.Unlock()
		if f.hold {
			select {
			case <-f.release:
			case <-r.Context().Done():
			}
		}
		a.mu.Lock()
		if r.Context().Err() == nil {
			resource["State"] = map[string]any{"Status": "exited", "Running": false, "Pid": 0, "StartedAt": "2026-01-01T00:00:00Z"}
			_ = json.NewEncoder(out).Encode(map[string]any{"StatusCode": f.exit})
		}
		return true
	case strings.HasSuffix(path, "/kill") && r.Method == "POST":
		resource["State"] = map[string]any{"Status": "exited", "Running": false, "Pid": 0, "StartedAt": "2026-01-01T00:00:00Z"}
		if f.killDelay > 0 {
			// The owned signal landed; its wait answer stays pending so a
			// controller under test remains observably mid-teardown.
			time.AfterFunc(f.killDelay, f.finish)
		} else {
			f.finish()
		}
		out.WriteHeader(204)
		return true
	case path == "/containers/"+id && r.Method == "DELETE" && f.tailStart != nil:
		f.tailOnce.Do(func() { close(f.tailStart) })
		a.mu.Unlock()
		select {
		case <-f.tailRead:
		case <-r.Context().Done():
		}
		a.mu.Lock()
	}
	return false
}
