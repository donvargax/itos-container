package features

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// This is a local HTTP Engine API fixture. It records actual SDK calls and
// changes only an inspection response, leaving independently observed truth intact.
func (a *sessionAPI) serveOffline(out http.ResponseWriter, request *http.Request, entry sessionRequest, index int) {
	s := a.w.session
	reply := func(status int, value any) {
		a.requests[index].Status = status
		if object, ok := value.(map[string]any); ok {
			a.requests[index].Response = approvalClone(object)
		}
		out.Header().Set("Content-Type", "application/json")
		out.WriteHeader(status)
		if value != nil {
			_, _ = out.Write(approvalJSON(value))
		}
	}
	if entry.Method == "POST" && entry.Path == "/containers/create" {
		id := approvalHash(map[string]any{"fixture-created": s.token})
		config := approvalClone(entry.Body)
		host := config["HostConfig"]
		delete(config, "HostConfig")
		root := a.w.doctor.checkout
		c := map[string]any{"Id": id, "Name": "/" + request.URL.Query().Get("name"), "Image": config["Image"], "Config": config, "HostConfig": host,
			"State":           map[string]any{"Status": "created", "Running": false, "Pid": 0, "StartedAt": "0001-01-01T00:00:00Z"},
			"NetworkSettings": map[string]any{"Networks": map[string]any{"none": map[string]any{}}},
			"Mounts":          []any{map[string]any{"Type": "bind", "Source": root, "Destination": "/workspace", "RW": true, "Propagation": "rprivate"}}}
		if mounts, ok := object(host)["Mounts"].([]any); ok && len(mounts) == 1 && object(mounts[0])["Type"] == "volume" {
			c["Mounts"] = []any{map[string]any{"Type": "volume", "Name": object(mounts[0])["Source"], "Driver": "local", "Source": "/fixture/volume/_data", "Destination": "/workspace", "RW": true, "Propagation": ""}}
		}
		a.local[id], a.observed[id] = c, approvalClone(c)
		a.created = append(a.created, id)
		if s.anchorFault == "worker create reply lost and name lookup absent" {
			reply(503, map[string]any{"message": "lost create response"})
			return
		}
		reply(201, map[string]any{"Id": id})
		return
	}
	target := strings.TrimPrefix(entry.Path, "/containers/")
	id, operation, _ := strings.Cut(target, "/")
	c := a.local[id]
	if id == "itos-container-"+s.token {
		for _, candidate := range a.local {
			if candidate["Name"] == "/"+id {
				c = candidate
			}
		}
		if s.anchorFault == "worker create reply lost and name lookup absent" {
			c = nil
		}
	}
	if c == nil {
		reply(404, map[string]any{"message": "absent"})
		a.absent[id] = true
		return
	}
	switch {
	case entry.Method == "GET" && operation == "json":
		inspected := approvalClone(c)
		if s.launch != nil && s.launch.faultActive && s.launch.cause == "ownership" {
			object(object(inspected["Config"])["Labels"])["io.itos-container.intent"] = strings.Repeat("f", 64)
		}
		count := 0
		for _, r := range a.requests {
			if r.Method == "GET" && strings.HasPrefix(r.Path, "/containers/") {
				count++
			}
		}
		// Ownership observation succeeds first. The fresh full-boundary observation
		// is faulty, and cleanup can subsequently revalidate the independent identity.
		if count == 2 && s.boundary != "" {
			if s.boundary == "unavailable boundary inspection" {
				reply(503, map[string]any{"message": "fixture unavailable"})
				return
			}
			if err := offlineInspectionDefect(inspected, s.boundary); err != nil {
				a.problem = err
				reply(500, nil)
				return
			}
		}
		reply(200, inspected)
	case entry.Method == "POST" && operation == "attach":
		conn, buf, err := out.(http.Hijacker).Hijack()
		if err != nil {
			a.problem = err
			return
		}
		a.attached = conn
		a.requests[index].Status = 101
		_, _ = buf.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
		_ = buf.Flush()
	case entry.Method == "POST" && operation == "start":
		if a.attached == nil {
			reply(400, nil)
			return
		}
		c["State"] = map[string]any{"Status": "running", "Running": true, "Pid": 123, "StartedAt": "2026-01-01T00:00:00Z"}
		_ = a.attached.SetWriteDeadline(time.Now().Add(2 * time.Second))
		for i, text := range []string{"slice-13-workload-out\n", "slice-13-workload-err\n"} {
			frame := []byte{byte(i + 1), 0, 0, 0, 0, 0, 0, byte(len(text))}
			_, _ = a.attached.Write(append(frame, []byte(text)...))
		}
		_ = a.attached.Close()
		reply(204, nil)
	case entry.Method == "POST" && operation == "wait":
		c["State"] = map[string]any{"Status": "exited", "Running": false, "Pid": 0, "StartedAt": "2026-01-01T00:00:00Z"}
		exit := 0
		if s.launch != nil {
			exit = s.launch.child
		}
		reply(200, map[string]any{"StatusCode": exit})
	case entry.Method == "POST" && operation == "kill":
		c["State"] = map[string]any{"Status": "exited", "Running": false, "Pid": 0, "StartedAt": "2026-01-01T00:00:00Z"}
		reply(204, nil)
	case entry.Method == "DELETE" && operation == "":
		if s.anchorFault == "worker removal fails and full ID remains" || s.anchorFault == "worker removal and absence remain unknown" {
			reply(503, map[string]any{"message": "slice-17-private-response"})
			return
		}
		if s.launch != nil && s.launch.faultActive && s.launch.cause == "temporary" {
			reply(503, map[string]any{"message": "slice-13-private-response"})
			return
		}
		delete(a.local, id)
		reply(204, nil)
	default:
		reply(403, map[string]any{"message": "fixture refuses unexpected operation"})
	}
}

func offlineInspectionDefect(c map[string]any, defect string) error {
	config, host := object(c["Config"]), object(c["HostConfig"])
	mounts, _ := host["Mounts"].([]any)
	switch defect {
	case "network mode bridge":
		host["NetworkMode"] = "bridge"
	case "additional network attachment":
		object(object(c["NetworkSettings"])["Networks"])["bridge"] = map[string]any{}
	case "published port":
		host["PortBindings"] = map[string]any{"80/tcp": []any{map[string]any{"HostPort": "8080"}}}
	case "root user":
		config["User"] = "0:0"
	case "different primary GID":
		uid, _, _ := strings.Cut(fmt.Sprint(config["User"]), ":")
		config["User"] = uid + ":98765"
	case "supplementary group":
		host["GroupAdd"] = []string{"0"}
	case "writable root":
		host["ReadonlyRootfs"] = false
	case "extra host bind":
		host["Binds"] = []string{"/:/host"}
	case "different checkout bind source":
		object(mounts[0])["Source"] = "/other"
	case "shared bind propagation":
		object(mounts[0])["BindOptions"] = map[string]any{"Propagation": "shared"}
	case "missing tmpfs":
		delete(host, "Tmpfs")
	case "named volume", "anonymous volume":
		v := map[string]any{"Type": "volume", "Destination": "/data"}
		if defect == "named volume" {
			v["Name"] = "foreign"
		}
		c["Mounts"] = append(c["Mounts"].([]any), v)
	case "missing CapDrop ALL":
		delete(host, "CapDrop")
	case "added capability":
		host["CapAdd"] = []string{"SYS_ADMIN"}
	case "missing no-new-privileges":
		delete(host, "SecurityOpt")
	case "privileged mode":
		host["Privileged"] = true
	case "host PID mode":
		host["PidMode"] = "host"
	case "host IPC mode":
		host["IpcMode"] = "host"
	case "host UTS mode":
		host["UTSMode"] = "host"
	case "extra device mapping":
		host["Devices"] = []any{map[string]any{"PathOnHost": "/dev/x", "PathInContainer": "/dev/x", "CgroupPermissions": "rwm"}}
	case "disabled seccomp confinement":
		host["SecurityOpt"] = []string{"no-new-privileges", "seccomp=unconfined"}
	case "pids limit other than 4096":
		host["PidsLimit"] = 4095
	case "restart policy always":
		object(host["RestartPolicy"])["Name"] = "always"
	case "auto-remove enabled":
		host["AutoRemove"] = true
	case "image entrypoint retained":
		config["Entrypoint"] = []string{"sh"}
	case "changed command argument":
		config["Cmd"] = []string{"changed"}
	case "wrong working directory":
		config["WorkingDir"] = "/"
	case "changed literal environment":
		config["Env"] = []string{"APP=changed"}
	case "missing security configuration":
		delete(c, "HostConfig")
	default:
		return fmt.Errorf("unknown offline boundary defect %q", defect)
	}
	return nil
}
