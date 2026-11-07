package features

import (
	"net/http"
	"strings"
)

// Local Engine API volume state is independent of the controller's return
// value and records rejected and failed operations as well as successful ones.
func (a *sessionAPI) serveAnchorVolume(out http.ResponseWriter, request *http.Request, e sessionRequest, index int) {
	if a.volumes == nil {
		a.volumes = map[string]map[string]any{}
	}
	status, value := 404, any(map[string]any{"message": "absent"})
	name := strings.TrimPrefix(e.Path, "/volumes/")
	fault := ""
	if a.w.anchorLocalRecovery != nil {
		fault = a.w.anchorLocalRecovery.fault
	}
	if s := a.w.anchorLocalRecovery; s != nil && s.mode == "budget" && ((strings.HasSuffix(fault, "inspect") && e.Method == "GET") || (strings.HasSuffix(fault, "delete") && e.Method == "DELETE")) {
		s.reached = true
		a.mu.Unlock()
		<-request.Context().Done()
		a.mu.Lock()
		a.requests[index].Status = 503
		out.WriteHeader(503)
		return
	}
	switch {
	case e.Method == "POST" && e.Path == "/volumes/create":
		name, _ = e.Body["Name"].(string)
		v := map[string]any{"Name": name, "Driver": e.Body["Driver"], "Labels": e.Body["Labels"], "Options": e.Body["DriverOpts"], "Scope": "local", "CreatedAt": "2026-11-07T17:00:00Z", "Mountpoint": "/fixture/volumes/" + name + "/_data"}
		a.volumes[name] = v
		status, value = 201, v
	case e.Method == "GET" && fault == "volume delete reply lost and inspection unavailable" && a.w.anchorLocalRecovery.reached:
		status = 503
	case e.Method == "GET" && a.volumes[name] != nil:
		status, value = 200, approvalClone(a.volumes[name])
		a.anchorRecoveryBarrier(request, e)
	case e.Method == "DELETE" && a.volumes[name] != nil:
		if a.w.anchorLocalRecovery != nil {
			a.w.anchorLocalRecovery.reached = true
		}
		switch fault {
		case "volume deletion fails and the owned volume remains":
			status = 503
		case "volume deletion reports in use":
			status = 409
		case "volume delete acknowledged but volume still present":
			status, value = 204, nil
		default:
			delete(a.volumes, name)
			status, value = 204, nil
			if fault == "volume delete reply lost and inspection unavailable" {
				status = 503
			}
		}
	}
	a.requests[index].Status = status
	if v, ok := value.(map[string]any); ok {
		a.requests[index].Response = approvalClone(v)
	}
	out.Header().Set("Content-Type", "application/json")
	out.WriteHeader(status)
	if value != nil {
		_, _ = out.Write(approvalJSON(value))
	}
}
