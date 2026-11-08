package features

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func (a *sessionAPI) serveRecovery(out http.ResponseWriter, r *http.Request, q sessionRequest, index int) {
	l := a.w.recoveryLocal
	status := 200
	var body any
	hijacked := false
	defer func() {
		a.requests[index].Status = status
		if hijacked {
			return
		}
		out.Header().Set("Content-Type", "application/json")
		out.WriteHeader(status)
		if body != nil {
			_ = json.NewEncoder(out).Encode(body)
		}
	}()
	if q.Path == "/containers/create" && q.Method == "POST" && l.runFixture {
		a.recoveryCreate(r, q, index)
		status = 201
		body = map[string]any{"Id": a.w.session.seedID}
		return
	}
	if l.unavailable {
		status = 503
		body = map[string]string{"message": doctorCanary}
		return
	}
	if l.stall {
		a.mu.Unlock()
		select {
		case <-r.Context().Done():
		case <-time.After(35 * time.Second):
		}
		a.mu.Lock()
		status = 503
		return
	}
	parts := strings.Split(strings.TrimPrefix(q.Path, "/containers/"), "/")
	id := parts[0]
	c := a.local[id]
	if l.unavailableIDs[id] {
		status = 503
		return
	}
	if strings.HasPrefix(id, "itos-container-") {
		for key, value := range a.local {
			if value["Name"] == "/"+id && key != strings.Repeat("f", 64) {
				c = value
				id = key
				break
			}
		}
	}
	if len(parts) == 2 && parts[1] == "json" && q.Method == "GET" {
		if c == nil {
			status = 404
			body = map[string]string{"message": "absent"}
			a.absent[id] = true
			return
		}
		if l.malformed {
			body = map[string]any{"Id": id}
			return
		}
		body = approvalClone(c)
		l.inspectionCount++
		barrierAt := 1
		if l.mutation == "SIGKILL" {
			barrierAt = 3
		}
		if l.inspectReady != nil && l.inspectionCount == barrierAt {
			close(l.inspectReady)
			a.mu.Unlock()
			select {
			case <-l.inspectRelease:
			case <-r.Context().Done():
			}
			a.mu.Lock()
		}
		if l.mode == "name conflict" && id == a.w.session.seedID && !strings.HasPrefix(parts[0], "itos-container-") {
			object(body)["Id"] = strings.Repeat("d", 64)
		}
		return
	}
	if c == nil {
		status = 404
		return
	}
	switch {
	case len(parts) == 2 && parts[1] == "attach" && q.Method == "POST" && l.runFixture:
		conn, buffer, err := out.(http.Hijacker).Hijack()
		if err != nil {
			a.problem = err
			status = 500
			return
		}
		hijacked = true
		a.attached = conn
		status = 101
		_, _ = buffer.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
		_ = buffer.Flush()
	case len(parts) == 2 && parts[1] == "start" && q.Method == "POST" && l.runFixture:
		c["State"] = map[string]any{"Status": "running", "Running": true, "Pid": 123, "StartedAt": "2026-01-01T00:00:00Z"}
		status = 204
	case len(parts) == 2 && parts[1] == "kill" && q.Method == "POST":
		signal := r.URL.Query().Get("signal")
		if signal != "SIGTERM" && signal != "SIGKILL" {
			status = 403
			return
		}
		if signal == "SIGKILL" || !l.ignoreTERM {
			a.recoveryExit(c)
		}
		status = 204
	case len(parts) == 2 && parts[1] == "wait" && q.Method == "POST":
		if l.runFixture && !l.keepRunning {
			a.recoveryExit(c)
		}
		if object(c["State"])["Running"] == true {
			a.mu.Unlock()
			select {
			case <-r.Context().Done():
			case <-l.workerExit:
			}
			a.mu.Lock()
		}
		if l.runFixture && r.Context().Err() == nil {
			body = map[string]any{"StatusCode": l.runExit}
			return
		}
		// This fixture deliberately has no explicit observed exit. An inspect
		// default ExitCode must not turn unknown workload outcome into zero.
		status = 503
		body = map[string]string{"message": "no workload observation"}
	case len(parts) == 1 && q.Method == "DELETE" && id != strings.Repeat("f", 64):
		if object(c["State"])["Running"] == true || r.URL.Query().Get("v") == "true" {
			status = 403
			return
		}
		if l.failRemove {
			status = 503
			body = map[string]string{"message": "slice-11-engine-body-marker"}
			return
		}
		if l.removeReady != nil {
			ready, release := l.removeReady, l.removeRelease
			l.removeReady = nil
			close(ready)
			a.mu.Unlock()
			select {
			case <-release:
			case <-r.Context().Done():
			}
			a.mu.Lock()
		}
		delete(a.local, id)
		status = 204
		if l.afterRemoveReady != nil {
			ready, release := l.afterRemoveReady, l.afterRemoveRelease
			l.afterRemoveReady = nil
			close(ready)
			a.mu.Unlock()
			select {
			case <-release:
			case <-r.Context().Done():
			}
			a.mu.Lock()
		}
		if l.lostRemove {
			l.unavailable = true
			status = 503
			body = map[string]string{"message": "lost response"}
		}
	default:
		status = 403
	}
}
