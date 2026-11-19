package features

import (
	"encoding/json"
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
	a.execStopBarrier()
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
			a.hijackClose(out, request)
			return
		case "create ignores cancellation past ten seconds", "original 120-second startup budget expires before start":
			a.requests[index].Status = http.StatusOK
			a.hijackUntilGone(out, request)
			return
		}
	}
	execID := approvalHash(map[string]any{"slice-38-exec": id, "index": len(a.requests)})
	instance := map[string]any{"ID": execID, "Container": id, "Running": false,
		"Cmd": body["Cmd"], "Env": body["Env"], "User": body["User"], "WorkingDir": body["WorkingDir"],
		"AttachStdin": body["AttachStdin"], "AttachStdout": body["AttachStdout"], "AttachStderr": body["AttachStderr"],
		"Tty": body["Tty"], "ExitCode": nil, "ExitPresent": false}
	if e := a.w.exec; e != nil {
		if e.execLie == "returned exec belongs to another container" {
			instance["Container"] = strings.Repeat("f", 64)
		}
		if e.execLie == "returned exec ID differs from the created ID" {
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
		a.execStopBarrier()
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
				a.hijackClose(out, request)
				return
			case "start accepted but marked API error returned":
				a.requests[index].Status = http.StatusInternalServerError
				instance["Running"] = true
				a.deliverMarked(out, http.StatusInternalServerError)
				return
			case "start ignores cancellation past ten seconds":
				a.requests[index].Status = http.StatusOK
				instance["Running"] = true
				a.hijackUntilGone(out, request)
				return
			}
		}
		a.requests[index].Status = http.StatusOK
		a.execStart(out, request, instance)
	case strings.HasSuffix(id, "/json") && entry.Method == "GET":
		instance, known := a.execs[strings.TrimSuffix(id, "/json")]
		if !known {
			a.requests[index].Status = http.StatusNotFound
			_, _ = out.Write([]byte(`{"message":"No such exec"}`))
			return
		}
		observed := approvalClone(instance)
		running, _ := observed["Running"].(bool)
		exit := map[string]any{"ID": observed["ID"], "Running": running, "Container": observed["Container"], "ProcessConfig": map[string]any{"argv": observed["Cmd"], "environment": observed["Env"]}}
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
func (a *sessionAPI) execStart(out http.ResponseWriter, request *http.Request, instance map[string]any) {
	hijacker, ok := out.(http.Hijacker)
	if !ok {
		a.problem = errString("session fixture cannot hijack exec stream")
		return
	}
	conn, _, err := hijacker.Hijack()
	if err != nil {
		a.problem = errString(err.Error())
		return
	}
	instance["Running"] = true
	event := a.w.exec.eventSchedule
	prefix := func() bool {
		if e := a.w.exec; e != nil && e.prefixOnce != nil {
			e.prefixOnce()
		}
		return event == ""
	}
	go func() {
		defer func() { _ = conn.Close() }()
		for _, stream := range a.execProgrammedStreams(instance) {
			frame := make([]byte, 8)
			frame[0] = stream[0]
			putUint32(frame[4:], uint32(len(stream)-1))
			_, _ = conn.Write(append(frame, stream[1:]...))
		}
		if prefix() {
			a.settleExec(instance)
			return
		}
		switch event {
		case "attachment transport lost":
			// The engine's side of the attachment dies after the prefix; the
			// controller observes a lost attachment, not an exit.
			return
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
func (a *sessionAPI) hijackClose(out http.ResponseWriter, request *http.Request) {
	conn, _, err := out.(http.Hijacker).Hijack()
	if err != nil {
		a.problem = errString(err.Error())
		return
	}
	_ = conn.Close()
	<-request.Context().Done()
}

// hijackUntilGone accepts the request and holds the reply open until the
// controller abandons the connection, so its cancellation deadline expires
// against an answer that never comes.
func (a *sessionAPI) hijackUntilGone(out http.ResponseWriter, request *http.Request) {
	conn, _, err := out.(http.Hijacker).Hijack()
	if err != nil {
		a.problem = errString(err.Error())
		return
	}
	defer func() { _ = conn.Close() }()
	timer := time.NewTimer(2 * time.Minute)
	defer timer.Stop()
	select {
	case <-request.Context().Done():
	case <-timer.C:
	}
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
		case "ownership replaced before signal":
			if a.w.execSubstitutionArmed() {
				if err := a.w.execSubstitute(); err != nil {
					a.problem = err
				}
			}
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
	a.requests[index].Status = http.StatusNoContent
	out.WriteHeader(http.StatusNoContent)
}

// execStopBarrier pauses an exec transition so an independent stop can
// complete first. The handler releases the recorder's lock while it waits:
// the stop's own requests must reach the fixture.
func (a *sessionAPI) execStopBarrier() {
	e := a.w.exec
	if e == nil || e.stopBoundary == "" || e.stopReady == nil || e.stopSignaled {
		return
	}
	e.stopSignaled = true
	ready, release := e.stopReady, e.stopRelease
	a.mu.Unlock()
	close(ready)
	<-release
	a.mu.Lock()
}

func errString(text string) error { return &execFixtureError{text} }

type execFixtureError struct{ message string }

func (e *execFixtureError) Error() string { return e.message }

func putUint32(data []byte, value uint32) {
	data[0] = byte(value >> 24)
	data[1] = byte(value >> 16)
	data[2] = byte(value >> 8)
	data[3] = byte(value)
}
