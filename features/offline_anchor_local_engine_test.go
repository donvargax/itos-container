package features

import (
	"net/http"
	"strings"
)

// Local Engine API volume state is independent of the controller's return
// value and records rejected and failed operations as well as successful ones.
func (a *sessionAPI) serveAnchorVolume(out http.ResponseWriter, e sessionRequest, index int) {
	if a.volumes == nil {
		a.volumes = map[string]map[string]any{}
	}
	status, value := 404, any(map[string]any{"message": "absent"})
	name := strings.TrimPrefix(e.Path, "/volumes/")
	switch {
	case e.Method == "POST" && e.Path == "/volumes/create":
		name, _ = e.Body["Name"].(string)
		v := map[string]any{"Name": name, "Driver": e.Body["Driver"], "Labels": e.Body["Labels"], "Options": e.Body["DriverOpts"], "Scope": "local", "CreatedAt": "2026-11-07T17:00:00Z", "Mountpoint": "/fixture/volumes/" + name + "/_data"}
		a.volumes[name] = v
		status, value = 201, v
	case e.Method == "GET" && a.volumes[name] != nil:
		status, value = 200, a.volumes[name]
	case e.Method == "DELETE" && a.volumes[name] != nil:
		delete(a.volumes, name)
		status, value = 204, nil
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
