package features

import (
	"encoding/json"
	"net/http"
	"strings"
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
	execID := approvalHash(map[string]any{"slice-38-exec": id, "index": len(a.requests)})
	instance := map[string]any{"ID": execID, "Container": id, "Running": false,
		"Cmd": body["Cmd"], "Env": body["Env"], "User": body["User"], "WorkingDir": body["WorkingDir"],
		"AttachStdin": body["AttachStdin"], "AttachStdout": body["AttachStdout"], "AttachStderr": body["AttachStderr"],
		"Tty": body["Tty"], "ExitCode": nil, "ExitPresent": false}
	a.execs[execID] = instance
	header := make(http.Header)
	header.Set("Content-Type", "application/json")
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
		exit := map[string]any{"ID": id, "Running": running, "Container": observed["Container"], "ProcessConfig": map[string]any{"argv": observed["Cmd"], "environment": observed["Env"]}}
		if present, _ := observed["ExitPresent"].(bool); present {
			if code, ok := observed["ExitCode"]; ok {
				exit["ExitCode"] = code
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
// output as stdcopy frames, then closes. The instance's exit observation is
// served by its inspect endpoint.
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
	go func() {
		defer func() { _ = conn.Close() }()
		for _, stream := range a.execProgrammedStreams(instance) {
			frame := make([]byte, 8)
			frame[0] = stream[0]
			putUint32(frame[4:], uint32(len(stream)-1))
			_, _ = conn.Write(append(frame, stream[1:]...))
		}
	}()
}

// execProgrammedStreams returns the instance's stdcopy payloads: each entry
// is a frame byte followed by its bytes. An instance with no programme
// streams nothing and simply closes; later scenario groups program markers
// and tails before the event they schedule.
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

func errString(text string) error { return &execFixtureError{text} }

type execFixtureError struct{ message string }

func (e *execFixtureError) Error() string { return e.message }

func putUint32(data []byte, value uint32) {
	data[0] = byte(value >> 24)
	data[1] = byte(value >> 16)
	data[2] = byte(value >> 8)
	data[3] = byte(value)
}
