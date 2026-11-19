package features

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// The recording Engine API's exec endpoints. Every request is journaled
// before dispatch, exactly as the rest of the fixture does, so Then steps
// can prove an admission refusal created or started nothing. The fixture
// serves execs only for the owned worker ID it recorded, and refuses
// anything else with the observer's canary, which no product output may
// contain.
func (a *sessionAPI) serveExecCreate(out http.ResponseWriter, request *http.Request, entry sessionRequest, index int) {
	if entry.Method != "POST" {
		a.requests[index].Status = http.StatusForbidden
		http.Error(out, "session fixture refuses request "+doctorCanary, http.StatusForbidden)
		return
	}
	id := strings.TrimSuffix(strings.TrimPrefix(entry.Path, "/containers/"), "/exec")
	if a.w.doctor.real == nil && id != a.w.exec.worker {
		a.requests[index].Status = http.StatusForbidden
		http.Error(out, "session fixture refuses request "+doctorCanary, http.StatusForbidden)
		return
	}
	body := entry.Body
	if !execCreateRequestValid(body) {
		a.requests[index].Status = http.StatusForbidden
		http.Error(out, "session fixture refuses request "+doctorCanary, http.StatusForbidden)
		return
	}
	if e := a.w.exec; e != nil {
		// Launch faults: an accepted create whose acknowledgement the
		// controller never receives, and a create reply held past the
		// controller's cancellation deadline. Neither starts work by itself.
		switch e.launchFault {
		case "create accepted but response lost":
			a.requests[index].Status = http.StatusOK
			a.hijackClose(out)
			return
		case "create ignores cancellation past ten seconds", "original 120-second startup budget expires before start":
			a.requests[index].Status = http.StatusOK
			a.hijackUntilGone(out)
			return
		}
	}
	execID := approvalHash(map[string]any{"slice-38-exec": id, "index": len(a.requests)})
	instance := map[string]any{"ID": execID, "Container": id, "Running": false,
		"Cmd": body["Cmd"], "Env": body["Env"], "User": body["User"], "WorkingDir": body["WorkingDir"],
		"AttachStdin": body["AttachStdin"], "AttachStdout": body["AttachStdout"], "AttachStderr": body["AttachStderr"],
		"Tty": body["Tty"], "ExitCode": nil, "ExitPresent": false}
	if e := a.w.exec; e != nil {
		if e.substituteAuthority != "" && e.execLie == "returned exec belongs to another container" {
			instance["Container"] = strings.Repeat("f", 64)
		}
		if e.substituteAuthority != "" && e.execLie == "returned exec ID differs from the created ID" {
			instance["ID"] = approvalHash(map[string]any{"slice-38-lying-exec": execID})
		}
		programme := []any{}
		if e.execStdout != "" {
			programme = append(programme, []any{float64(1), e.execStdout})
		}
		if e.execStderr != "" {
			programme = append(programme, []any{float64(2), e.execStderr})
		}
		instance["streams"] = programme
		instance["programmedExit"] = float64(e.execExit)
		switch e.exitObservation {
		case "23", "0":
			// The exit observation is only served after the streams settle.
		case "null":
			instance["ExitPresent"] = true
		case "missing":
			instance["ExitPresent"] = false
		}
		// The observation barrier for a pre-start revalidation: the create
		// reply carries what the product asked for, and only its next
		// revalidation can see the substituted authority.
		if a.w.execSubstitutionArmed() && e.substituteBoundary == "before exec start" {
			if err := a.w.execSubstitute(); err != nil {
				a.problem = err
			}
		}
	}
	a.execs[execID] = instance
	a.requests[index].Status = http.StatusCreated
	out.Header().Set("Content-Type", "application/json")
	out.WriteHeader(http.StatusCreated)
	_, _ = out.Write(approvalJSON(map[string]any{"Id": execID}))
}

func execCreateRequestValid(body map[string]any) bool {
	if body == nil {
		return false
	}
	command, ok := body["Cmd"].([]any)
	if !ok || len(command) == 0 {
		return false
	}
	if body["Tty"] != false {
		return false
	}
	for _, key := range []string{"AttachStdout", "AttachStderr"} {
		if body[key] != true {
			return false
		}
	}
	return true
}

func (a *sessionAPI) serveExec(out http.ResponseWriter, request *http.Request, entry sessionRequest, index int) {
	id := strings.TrimPrefix(entry.Path, "/exec/")
	switch {
	case strings.HasSuffix(id, "/start") && entry.Method == "POST":
		instance, known := a.execs[strings.TrimSuffix(id, "/start")]
		if !known {
			a.requests[index].Status = http.StatusForbidden
			http.Error(out, "session fixture refuses request "+doctorCanary, http.StatusForbidden)
			return
		}
		if e := a.w.exec; e != nil {
			// Launch faults on the start mutation: an accepted start whose
			// attach stream is lost, a marked API error, and a start reply
			// held past the controller's cancellation deadline. All three
			// can have started work, so the controller must tear the whole
			// session down.
			switch e.launchFault {
			case "start accepted but attach response lost":
				a.requests[index].Status = http.StatusOK
				instance["Running"] = true
				a.hijackClose(out)
				return
			case "start accepted but marked API error returned":
				a.requests[index].Status = http.StatusInternalServerError
				instance["Running"] = true
				a.deliverMarked(out, http.StatusInternalServerError)
				return
			case "start ignores cancellation past ten seconds":
				a.requests[index].Status = http.StatusOK
				instance["Running"] = true
				a.hijackUntilGone(out)
				return
			}
		}
		a.requests[index].Status = http.StatusSwitchingProtocols
		a.execStart(out, instance)
	case strings.HasSuffix(id, "/json") && entry.Method == "GET":
		instance, known := a.execs[strings.TrimSuffix(id, "/json")]
		if !known {
			a.requests[index].Status = http.StatusNotFound
			_, _ = out.Write([]byte(`{"message":"No such exec"}`))
			return
		}
		observed := approvalClone(instance)
		running, _ := observed["Running"].(bool)
		exit := map[string]any{"ID": observed["ID"], "Running": running, "ContainerID": observed["Container"]}
		if len(a.w.exec.markers) != 0 {
			exit["fixture_detail"] = "slice-38-api-body-marker"
		}
		if present, _ := observed["ExitPresent"].(bool); present {
			if code, ok := observed["ExitCode"]; ok {
				exit["ExitCode"] = code
			} else {
				// A missing exit is distinct from a null one on the wire.
				exit["ExitCode"] = nil
			}
		}
		a.requests[index].Status = http.StatusOK
		out.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(out).Encode(exit)
	default:
		a.requests[index].Status = http.StatusForbidden
		http.Error(out, "session fixture refuses request "+doctorCanary, http.StatusForbidden)
	}
}

// execStart hijacks the connection and streams the instance's programmed
// output as stdcopy frames, then settles its exit observation. The event the
// row schedules may cut the attachment short after the deliberate prefix.
func (a *sessionAPI) execStart(out http.ResponseWriter, instance map[string]any) {
	hijacker, ok := out.(http.Hijacker)
	if !ok {
		a.problem = errors.New("session fixture cannot hijack exec stream")
		return
	}
	conn, buffered, err := hijacker.Hijack()
	if err != nil {
		a.problem = err
		return
	}
	_, err = buffered.WriteString("HTTP/1.1 101 UPGRADED\r\nConnection: Upgrade\r\nUpgrade: tcp\r\nContent-Type: application/vnd.docker.raw-stream\r\n\r\n")
	if err == nil {
		err = buffered.Flush()
	}
	if err != nil {
		a.problem = err
		_ = conn.Close()
		return
	}
	instance["Running"] = true
	event := a.w.exec.eventSchedule
	delivered := a.w.exec.eventDelivered
	streams := a.execProgrammedStreams(instance)
	go func() {
		defer func() { _ = conn.Close() }()
		for _, stream := range streams {
			frame := make([]byte, 8)
			frame[0] = stream[0]
			binary.BigEndian.PutUint32(frame[4:], uint32(len(stream)-1))
			_, _ = conn.Write(append(frame, stream[1:]...))
		}
		if event == "" {
			a.settleExec(instance)
			return
		}
		if delivered != nil {
			<-delivered
		}
		switch event {
		case "output sink failure":
			// Force a write after the harness has broken the sink.
			_, _ = conn.Write([]byte{1, 0, 0, 0, 0, 0, 0, 1, 'x'})
			a.settleExec(instance)
		case "SIGINT", "SIGTERM":
			a.settleExec(instance)
		case "primary exit and exec removed before inspection":
			a.mu.Lock()
			delete(a.local, a.w.exec.worker)
			if key, ok := instance["ID"].(string); ok {
				delete(a.execs, key)
			}
			a.mu.Unlock()
		}
	}()
}

// settleExec records the instance's exit observation the way the row
// programs it once the streams have settled.
func (a *sessionAPI) settleExec(instance map[string]any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	instance["Running"] = false
	switch a.w.exec.exitObservation {
	case "", "23", "0":
		instance["ExitPresent"] = true
		exit, _ := instance["programmedExit"].(float64)
		instance["ExitCode"] = exit
	}
}

// execProgrammedStreams returns the instance's stdcopy payloads: each entry
// is a frame byte followed by its bytes.
func (a *sessionAPI) execProgrammedStreams(instance map[string]any) [][]byte {
	programme, _ := instance["streams"].([]any)
	var out [][]byte
	for _, one := range programme {
		entry, _ := one.([]any)
		if len(entry) != 2 {
			continue
		}
		channel, _ := entry[0].(float64)
		text, _ := entry[1].(string)
		out = append(out, append([]byte{byte(channel)}, []byte(text)...))
	}
	return out
}

// deliverMarked answers with the marked API error body and records that the
// controller received it, so an unconsumed fixture cannot prove exclusion.
func (a *sessionAPI) deliverMarked(out http.ResponseWriter, status int) {
	if a.w.exec != nil {
		a.w.exec.markedDelivered++
	}
	http.Error(out, "engine failure slice-38-api-body-marker", status)
}

// hijackClose accepts the request and drops the connection without a reply.
func (a *sessionAPI) hijackClose(out http.ResponseWriter) {
	conn, _, err := out.(http.Hijacker).Hijack()
	if err != nil {
		a.problem = err
		return
	}
	_ = conn.Close()
}

// hijackUntilGone accepts the request and holds the reply open until the
// controller abandons the connection, so its cancellation deadline expires
// against an answer that never comes.
func (a *sessionAPI) hijackUntilGone(out http.ResponseWriter) {
	conn, _, err := out.(http.Hijacker).Hijack()
	if err != nil {
		a.problem = err
		return
	}
	defer func() { _ = conn.Close() }()
	a.mu.Unlock()
	defer a.mu.Lock()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
	_, _ = io.Copy(io.Discard, conn)
}

// serveExecKill applies a signal to the owned worker: the local fixture has
// no real process, so the container observation moves to exited and the
// journal records the request before dispatch. A cleanup fault can replace
// ownership at this boundary, or the engine can be unavailable throughout.
func (a *sessionAPI) serveExecKill(out http.ResponseWriter, entry sessionRequest, index int) {
	id := strings.TrimSuffix(strings.TrimPrefix(entry.Path, "/containers/"), "/kill")
	if a.w.doctor.real == nil && id != a.w.exec.worker {
		a.requests[index].Status = http.StatusForbidden
		http.Error(out, "session fixture refuses request "+doctorCanary, http.StatusForbidden)
		return
	}
	if e := a.w.exec; e != nil && !e.faultCleared {
		switch e.cleanupFault {
		case "engine unavailable through teardown deadline":
			a.requests[index].Status = http.StatusServiceUnavailable
			e.markedDelivered++
			http.Error(out, "engine unavailable slice-38-api-body-marker", http.StatusServiceUnavailable)
			return
		}
	}
	if c := a.local[id]; c != nil {
		object(c["State"])["Status"] = "exited"
		object(c["State"])["Running"] = false
	}
	for _, instance := range a.execs {
		if instance["Running"] == true && instance["Container"] == id {
			instance["Running"] = false
			if a.w.exec.exitObservation == "23" || a.w.exec.exitObservation == "0" {
				instance["ExitPresent"] = true
				instance["ExitCode"] = instance["programmedExit"]
			}
		}
	}
	a.requests[index].Status = http.StatusNoContent
	out.WriteHeader(http.StatusNoContent)
}
