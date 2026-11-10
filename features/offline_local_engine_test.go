package features

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"
)

// Local transport fixture for the older invocation/preflight scenarios. It
// never executes a workload; the real-engine cases own execution evidence.
type localOfflineAPI struct {
	mu               sync.Mutex
	w                *world
	resources        map[string]map[string]any
	volumes          map[string]map[string]any
	allocated        map[string]map[string]any
	created          map[string]map[string]any
	attached         map[string]net.Conn
	started, removed map[string]bool
	expected         map[int][]string
	problem          error
	recovery         *foregroundRecovery
	attachBarrier    func(string, string)
}

func (w *world) localOfflineFixture() {
	a := &localOfflineAPI{w: w, resources: map[string]map[string]any{}, volumes: map[string]map[string]any{}, allocated: map[string]map[string]any{}, created: map[string]map[string]any{}, attached: map[string]net.Conn{}, started: map[string]bool{}, removed: map[string]bool{}, expected: map[int][]string{}}
	w.doctor.engine.mu.Lock()
	w.doctor.engine.offline = a
	w.doctor.engine.mu.Unlock()
	w.doctor.cleanup = append(w.doctor.cleanup, func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		for _, c := range a.attached {
			_ = c.Close()
		}
	})
}

func (a *localOfflineAPI) serve(out http.ResponseWriter, r *http.Request) bool {
	path := doctorRequestPath(r.URL.Path)
	if !strings.HasPrefix(path, "/containers/") && !strings.HasPrefix(path, "/volumes/") && path != "/images/"+doctorImageID+"/json" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if strings.HasPrefix(path, "/volumes/") {
		serveLocalVolume(out, r, a.volumes)
		if r.Method == "POST" && path == "/volumes/create" {
			for name, v := range a.volumes {
				a.allocated[name] = approvalClone(v)
			}
		}
		return true
	}
	out.Header().Set("Content-Type", "application/json")
	if path == "/images/"+doctorImageID+"/json" {
		_ = json.NewEncoder(out).Encode(a.w.doctor.engine.image)
		return true
	}
	if path == "/containers/create" && r.Method == "POST" {
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(out, "bad create", 400)
			return true
		}
		id := fmt.Sprintf("%064x", len(a.created)+1)
		host := object(body["HostConfig"])
		labels := object(body["Labels"])
		token, _ := labels["io.itos-container.intent"].(string)
		pid, _ := r.Context().Value(runPeerKey{}).(int)
		if body["Image"] != doctorImageID || body["User"] != a.w.approval.uid+":"+a.w.approval.gid ||
			body["WorkingDir"] != object(a.w.approval.settings["manifest"])["workspaceFolder"] ||
			!reflect.DeepEqual(body["Cmd"], offlineStrings(a.expected[pid])) || !reflect.DeepEqual(body["Entrypoint"], []any{}) ||
			host["NetworkMode"] != "none" || host["ReadonlyRootfs"] != true || r.URL.Query().Get("name") != "itos-container-"+token {
			a.problem = fmt.Errorf("create lost frozen argv or settings")
			http.Error(out, "unsafe create", 400)
			return true
		}
		if err := a.record(token, "intent", ""); err != nil {
			a.problem = err
			http.Error(out, "intent missing", 400)
			return true
		}
		mounts := localAnchorMount(host)
		resource := map[string]any{"Id": id, "Name": "/" + r.URL.Query().Get("name"), "Image": doctorImageID, "Config": body, "HostConfig": host, "Mounts": mounts, "NetworkSettings": map[string]any{"Networks": map[string]any{"none": map[string]any{}}}, "State": map[string]any{"Status": "created", "Running": false, "Pid": 0}}
		a.created[id], a.resources[id] = resource, resource
		out.WriteHeader(201)
		_ = json.NewEncoder(out).Encode(map[string]string{"Id": id})
		return true
	}
	parts := strings.Split(strings.TrimPrefix(path, "/containers/"), "/")
	id := parts[0]
	resource := a.resources[id]
	if resource == nil {
		http.Error(out, `{"message":"absent"}`, 404)
		return true
	}
	token, _ := object(object(resource["Config"])["Labels"])["io.itos-container.intent"].(string)
	switch {
	case len(parts) == 2 && parts[1] == "json" && r.Method == "GET":
		_ = json.NewEncoder(out).Encode(resource)
	case len(parts) == 2 && parts[1] == "attach" && r.Method == "POST":
		if a.attachBarrier != nil {
			a.mu.Unlock()
			a.attachBarrier(token, id)
			a.mu.Lock()
		}
		conn, buf, err := out.(http.Hijacker).Hijack()
		if err != nil {
			a.problem = err
			return true
		}
		a.attached[id] = conn
		_, _ = buf.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
		_ = buf.Flush()
	case len(parts) == 2 && parts[1] == "start" && r.Method == "POST":
		if a.attached[id] == nil || a.record(token, "owned", id) != nil {
			a.problem = fmt.Errorf("start without attachment or durable ownership")
			http.Error(out, "bad start", 400)
			return true
		}
		a.started[id] = true
		if a.recovery != nil {
			a.recoveryStart(out, resource, id)
			return true
		}
		resource["State"] = map[string]any{"Status": "running", "Running": true, "Pid": 123, "StartedAt": "2026-01-01T00:00:00Z"}
		_ = a.attached[id].Close()
		out.WriteHeader(204)
	case len(parts) == 2 && parts[1] == "wait" && r.Method == "POST" && a.started[id]:
		// Foreground interrupts arrive after launch and its mount detector.
		// Separate startup-interrupt scenarios exercise cancellation before this.
		if f := a.recovery; f != nil {
			f.began.Do(func() { close(f.started) })
		}
		if f := a.recovery; f != nil && f.fault == "unread stdin" {
			f.waits++
			if f.waits == 1 {
				// Acknowledge before holding the worker past the launch connection's
				// shortened fixture deadline, observed from the actual port context.
				out.WriteHeader(200)
				out.(http.Flusher).Flush()
				a.mu.Unlock()
				expired := false
				if f.lateStdin {
					var deadline time.Time
					select {
					case deadline = <-f.startup:
					case <-r.Context().Done():
					}
					timer := time.NewTimer(time.Until(deadline))
					select {
					case <-timer.C:
						expired = !deadline.IsZero()
					case <-r.Context().Done():
					}
					timer.Stop()
				}
				a.mu.Lock()
				f.expired = expired
				resource["State"] = map[string]any{"Status": "exited", "Running": false, "Pid": 0, "StartedAt": "2026-01-01T00:00:00Z"}
				_ = a.attached[id].(*net.UnixConn).CloseRead()
				// Release buffered bytes after read shutdown so a writer already
				// parked in netpoll wakes and observes the closed input. Further
				// writes fail; most of the redirected file remains unread.
				f.discarded, a.problem = io.Copy(io.Discard, a.attached[id])
				if !f.trailing {
					_ = a.attached[id].(*net.UnixConn).CloseWrite()
				}
				// Only the second wait returns exit; the pending first wait must
				// be canceled by stdin closure. Keep stdout/stderr open to drain.
				a.mu.Unlock()
				<-r.Context().Done()
				a.mu.Lock()
				return true
			}
		}
		if a.recovery != nil && a.recovery.block {
			out.WriteHeader(200)
			out.(http.Flusher).Flush()
			a.mu.Unlock()
			select {
			case <-a.recovery.exit:
			case <-r.Context().Done():
			}
			a.mu.Lock()
		}
		// A canceled wait leaves the simulated worker running for owned signal
		// delivery; ending the HTTP subscription is not a workload exit.
		if r.Context().Err() != nil {
			return true
		}
		resource["State"] = map[string]any{"Status": "exited", "Running": false, "Pid": 0, "StartedAt": "2026-01-01T00:00:00Z"}
		exit := 0
		if a.recovery != nil {
			exit = a.recovery.exitCode
		}
		_ = json.NewEncoder(out).Encode(map[string]any{"StatusCode": exit})
		if a.recovery != nil && a.recovery.trailing && r.Context().Err() == nil {
			out.(http.Flusher).Flush()
			conn := a.attached[id]
			// Release output only when the real runner starts draining. A runner
			// which removes first gets no trailing bytes and fails the assertion.
			a.mu.Unlock()
			select {
			case <-a.recovery.draining:
			case <-a.recovery.done:
				a.mu.Lock()
				return true
			}
			for stream, data := range []string{recoveryTailOut, recoveryTailErr} {
				frame := make([]byte, 8)
				frame[0] = byte(stream + 1)
				binary.BigEndian.PutUint32(frame[4:], uint32(len(data)))
				_, _ = conn.Write(append(frame, []byte(data)...))
			}
			if a.recovery.lateStdin {
				_ = conn.(*net.UnixConn).CloseWrite()
			} else {
				_ = conn.Close()
			}
			a.mu.Lock()
		}
	case len(parts) == 2 && parts[1] == "kill" && r.Method == "POST" && a.recovery != nil:
		a.recovery.signals = append(a.recovery.signals, r.URL.Query().Get("signal"))
		a.recovery.notified.Do(func() { close(a.recovery.signaled) })
		if r.URL.Query().Get("signal") == "SIGKILL" || !strings.HasPrefix(a.recovery.fault, "ignored ") {
			a.recovery.finish.Do(func() { close(a.recovery.exit) })
			resource["State"] = map[string]any{"Status": "exited", "Running": false, "Pid": 0, "StartedAt": "2026-01-01T00:00:00Z"}
			if !a.recovery.trailing {
				_ = a.attached[id].Close()
			}
		}
		out.WriteHeader(204)
	case len(parts) == 1 && r.Method == "DELETE":
		if a.record(token, "owned", id) != nil {
			a.problem = fmt.Errorf("remove without durable ownership")
			http.Error(out, "bad remove", 400)
			return true
		}
		delete(a.resources, id)
		a.removed[id] = true
		if a.recovery != nil {
			a.recovery.finish.Do(func() { close(a.recovery.exit) })
		}
		// Removal ends the resource's attachment even if launch was refused
		// before start. Otherwise a healthy final drain waits on fixture-only I/O.
		if conn := a.attached[id]; conn != nil {
			_ = conn.Close()
		}
		out.WriteHeader(204)
	default:
		a.problem = fmt.Errorf("unexpected run request %s %s", r.Method, path)
		http.Error(out, "forbidden", http.StatusForbidden)
	}
	return true
}

func (a *localOfflineAPI) record(token, stage, id string) error {
	path := filepath.Join(a.w.doctor.env["XDG_STATE_HOME"], "itos-container", "intents", a.w.approval.project, token+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var r map[string]any
	if json.Unmarshal(data, &r) != nil || len(r) != 15 || r["schema"] != float64(2) || r["stage"] != stage || r["container_id"] != id || r["fingerprint"] != a.w.approval.fingerprint || r["image_id"] != doctorImageID {
		return fmt.Errorf("wrong protected intent")
	}
	anchor := object(r["anchor"])
	wantStage := "owned"
	if stage == "absent" {
		wantStage = "absent"
	}
	if len(anchor) != 7 || anchor["stage"] != wantStage || anchor["name"] != "itos-container-anchor-"+token || anchor["created"] == "" {
		return fmt.Errorf("wrong protected anchor")
	}
	return nil
}

func (w *world) localOfflineSuccess() error {
	if w.exit != 0 || w.stdout != "" || w.stderr != "" {
		return fmt.Errorf("foreground result: %s", w.report())
	}
	return w.localOfflineOwned()
}
func (w *world) localOfflineOwned() error {
	a := w.doctor.engine.offline
	if a == nil {
		return fmt.Errorf("missing local foreground fixture")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.problem != nil {
		return a.problem
	}
	if len(a.created) == 0 || len(a.resources) != 0 || len(a.volumes) != 0 {
		return fmt.Errorf("no worker or incomplete owned cleanup")
	}
	for id, r := range a.created {
		token, _ := object(object(r["Config"])["Labels"])["io.itos-container.intent"].(string)
		if !a.started[id] || !a.removed[id] {
			return fmt.Errorf("worker was not started and removed")
		}
		if err := a.record(token, "absent", id); err != nil {
			return err
		}
	}
	if len(a.allocated) != len(a.created) {
		return fmt.Errorf("run allocated unrelated volumes")
	}
	for _, resource := range a.created {
		token := fmt.Sprint(object(object(resource["Config"])["Labels"])["io.itos-container.intent"])
		v := a.allocated["itos-container-anchor-"+token]
		labels, opts := object(v["Labels"]), object(v["Options"])
		if v == nil || v["Driver"] != "local" || len(labels) != 7 || labels["io.itos-container.intent"] != token || labels["io.itos-container.project"] != w.approval.project || labels["io.itos-container.fingerprint"] != w.approval.fingerprint || labels["io.itos-container.kind"] != "checkout-anchor" || len(opts) != 3 || opts["type"] != "none" || opts["o"] != "bind" {
			return fmt.Errorf("run lost independently observed anchor ownership")
		}
	}
	for _, r := range w.doctor.engine.requests() {
		p := doctorRequestPath(r.Path)
		if strings.HasPrefix(p, "/volumes/") {
			name := strings.TrimPrefix(p, "/volumes/")
			if (r.Method == "POST" && name == "create" && r.Query == "") || (a.allocated[name] != nil && (r.Method == "GET" && r.Query == "" || r.Method == "DELETE" && (r.Query == "" || r.Query == "force=false"))) {
				continue
			}
			return fmt.Errorf("unowned volume operation")
		}
		if doctorAllowedRequest(r, w.doctor.image, true) || (r.Method == "GET" && p == "/images/"+doctorImageID+"/json") || (r.Method == "POST" && p == "/containers/create") {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(p, "/containers/"), "/")
		if !strings.HasPrefix(p, "/containers/") || a.created[parts[0]] == nil {
			return fmt.Errorf("unowned engine effect: %v", r)
		}
	}
	return w.doctorNoMarker()
}

func (w *world) localIntentPath(path string) bool {
	if w.doctor == nil || w.doctor.engine == nil || w.doctor.engine.offline == nil {
		return false
	}
	root := filepath.Join(w.doctor.env["XDG_STATE_HOME"], "itos-container", "intents")
	project := filepath.Join(root, w.approval.project)
	if path == root || path == project || path == filepath.Join(project, ".lock") {
		return true
	}
	a := w.doctor.engine.offline
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, resource := range a.created {
		token, _ := object(object(resource["Config"])["Labels"])["io.itos-container.intent"].(string)
		if path == filepath.Join(project, token+".json") {
			return true
		}
	}
	return false
}

func (w *world) localOfflineFirstRun() error {
	if len(w.run.results) != 2 || w.run.results[0].result.exit != 0 {
		return fmt.Errorf("first run did not execute")
	}
	if err := w.localOfflineOwned(); err != nil {
		return err
	}
	for _, r := range w.run.results[1].journal {
		if !doctorAllowedRequest(r, w.doctor.image, true) {
			return fmt.Errorf("refused invocation mutated engine")
		}
	}
	return nil
}

func (w *world) runThreeCompletions() error {
	if len(w.run.results) != 3 {
		return fmt.Errorf("missing invocations")
	}
	for _, o := range w.run.results {
		if o.approvalErr != nil {
			return o.approvalErr
		}
		if o.result.exit != 0 || o.result.stdout != "" || o.result.stderr != "" {
			return fmt.Errorf("wrong run result %+v", o.result)
		}
	}
	return nil
}
func (w *world) runThreeJournals() error {
	if err := w.localOfflineOwned(); err != nil {
		return err
	}
	for _, o := range w.run.results {
		var inspections []doctorRequest
		for _, r := range o.journal {
			if doctorAllowedRequest(r, w.doctor.image, true) {
				inspections = append(inspections, r)
			}
		}
		if err := w.runJournal(inspections, "complete", true); err != nil {
			return fmt.Errorf("PID %d: %w", o.pid, err)
		}
	}
	return nil
}
