package features

import (
	"encoding/json"
	"net/http"
	"strings"
)

// These local engine objects model volume metadata only, never native mounts.
func localVolumeReply(volumes map[string]map[string]any, method, path string, body map[string]any) (int, any) {
	name := strings.TrimPrefix(path, "/volumes/")
	if method == "POST" && name == "create" {
		name, _ = body["Name"].(string)
		if volumes[name] == nil {
			volumes[name] = map[string]any{"Name": name, "Driver": body["Driver"], "Scope": "local", "CreatedAt": "2026-11-07T17:00:00Z", "Options": body["DriverOpts"], "Labels": body["Labels"]}
		}
		return 201, volumes[name]
	}
	if volumes[name] == nil {
		return 404, map[string]any{"message": "absent"}
	}
	if method == "GET" {
		return 200, volumes[name]
	}
	if method == "DELETE" {
		delete(volumes, name)
		return 204, nil
	}
	return 403, nil
}

func serveLocalVolume(out http.ResponseWriter, r *http.Request, volumes map[string]map[string]any) {
	var body map[string]any
	if r.Method == "POST" {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	status, reply := localVolumeReply(volumes, r.Method, doctorRequestPath(r.URL.Path), body)
	out.Header().Set("Content-Type", "application/json")
	out.WriteHeader(status)
	if reply != nil {
		_ = json.NewEncoder(out).Encode(reply)
	}
}

func localAnchorMount(host map[string]any) []any {
	mounts, _ := host["Mounts"].([]any)
	if len(mounts) != 1 {
		return nil
	}
	m := object(mounts[0])
	if m["Type"] != "volume" || m["Target"] != "/workspace" || object(m["VolumeOptions"])["NoCopy"] != true {
		return nil
	}
	return []any{map[string]any{"Type": "volume", "Name": m["Source"], "Driver": "local", "Destination": "/workspace", "RW": true}}
}
