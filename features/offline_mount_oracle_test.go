package features

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func mountOracleFixture() (*isolationWorld, *isolationRun) {
	token, id, session := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	name := "itos-container-anchor-" + token
	labels := map[string]any{"io.itos-container.intent": token, "io.itos-container.project": "project", "io.itos-container.fingerprint": "fingerprint"}
	volumeLabels := approvalClone(labels)
	volumeLabels["io.itos-container.session"], volumeLabels["io.itos-container.kind"] = session, "checkout-anchor"
	volumeLabels["io.itos-container.device"], volumeLabels["io.itos-container.inode"] = "2049", "42"
	anchor := map[string]any{"name": name, "session": session, "source": "/proc/123/fd/7", "device": "2049", "inode": "42", "created": "2026-11-07T00:00:00Z", "stage": "owned"}
	record := map[string]any{"schema": 2, "stage": "owned", "container_id": id, "name": "itos-container-" + token, "project": "project", "fingerprint": "fingerprint", "anchor": anchor}
	volume := map[string]any{"Name": name, "Driver": "local", "Scope": "local", "CreatedAt": anchor["created"], "Mountpoint": "/var/lib/docker/volumes/" + name + "/_data", "Labels": volumeLabels, "Options": map[string]any{"type": "none", "o": "bind", "device": anchor["source"]}}
	worker := map[string]any{
		"Id": id, "Config": map[string]any{"Labels": labels},
		"HostConfig": map[string]any{"ReadonlyRootfs": true, "Tmpfs": map[string]any{"/tmp": "rw,nosuid,nodev,mode=1777"}, "Mounts": []any{map[string]any{"Type": "volume", "Source": name, "Target": "/workspace", "VolumeOptions": map[string]any{"NoCopy": true}}}},
		"Mounts":     []any{map[string]any{"Type": "volume", "Name": name, "Driver": "local", "Source": volume["Mountpoint"], "Destination": "/workspace", "RW": true}},
	}
	s := &isolationWorld{w: &world{approval: &approvalWorld{uid: "1001", project: "project", fingerprint: "fingerprint", settings: map[string]any{"checkout": map[string]any{"device": "2049", "inode": "42"}}}}}
	r := &isolationRun{id: id, sessionID: session, requests: []offlineRequest{{Method: "POST", Path: "/containers/" + id + "/start", Observation: worker, Anchor: volume, Record: sessionRecord{Bytes: string(approvalJSON(record)), UID: "1001", Mode: 0600, Inode: "90"}}}}
	return s, r
}

func TestOfflineMountOracleAcceptsOnlyExactOwnedAnchor(t *testing.T) {
	s, run := mountOracleFixture()
	if err := s.mountBoundary(run); err != nil {
		t.Fatalf("independently observed owned anchor rejected: %v", err)
	}
	for _, fault := range []string{"missing-volume", "unrelated-volume", "driver", "scope", "created", "source", "options", "labels", "session", "registration", "inode", "record", "record-mode", "record-owner", "worker", "worker-project", "mount-source", "mount-target", "mount-readonly", "extra-volume", "pathname-bind", "copy", "extra-request", "extra-tmpfs"} {
		t.Run(fault, func(t *testing.T) {
			s, run := mountOracleFixture()
			r := &run.requests[0]
			m := object(r.Observation["Mounts"].([]any)[0])
			h := object(r.Observation["HostConfig"])
			switch fault {
			case "missing-volume":
				r.Anchor = nil
			case "unrelated-volume":
				r.Anchor["Name"], m["Name"] = "unrelated", "unrelated"
			case "driver":
				r.Anchor["Driver"] = "other"
			case "scope":
				r.Anchor["Scope"] = "global"
			case "created":
				r.Anchor["CreatedAt"] = "replacement"
			case "source":
				object(r.Anchor["Options"])["device"] = "/checkout"
			case "options":
				object(r.Anchor["Options"])["extra"] = "value"
			case "labels":
				delete(object(r.Anchor["Labels"]), "io.itos-container.kind")
			case "session":
				object(r.Anchor["Labels"])["io.itos-container.session"] = strings.Repeat("d", 64)
			case "registration":
				run.sessionID = strings.Repeat("d", 64)
			case "inode":
				object(s.w.approval.settings["checkout"])["inode"] = "43"
			case "record":
				r.Record.Bytes = "{}"
			case "record-mode":
				r.Record.Mode = 0666
			case "record-owner":
				r.Record.UID = "2000"
			case "worker":
				r.Observation["Id"] = strings.Repeat("e", 64)
			case "worker-project":
				object(object(r.Observation["Config"])["Labels"])["io.itos-container.project"] = "unrelated"
			case "mount-source":
				m["Source"] = "/other"
			case "mount-target":
				m["Destination"] = "/other"
			case "mount-readonly":
				m["RW"] = false
			case "extra-volume":
				r.Observation["Mounts"] = append(r.Observation["Mounts"].([]any), approvalClone(m))
			case "pathname-bind":
				m["Type"] = "bind"
			case "copy":
				object(object(h["Mounts"].([]any)[0])["VolumeOptions"])["NoCopy"] = false
			case "extra-request":
				h["Mounts"] = append(h["Mounts"].([]any), map[string]any{"Type": "volume", "Source": "other", "Target": "/other"})
			case "extra-tmpfs":
				object(h["Tmpfs"])["/other"] = "rw"
			}
			if s.mountBoundary(run) == nil {
				t.Fatal("incomplete or substituted anchor boundary accepted")
			}
		})
	}
}

func TestOfflineMountObservationReadsTheNativeVolume(t *testing.T) {
	_, run := mountOracleFixture()
	r := run.requests[0]
	name := r.Anchor["Name"].(string)
	reads := 0
	w := offlineRecorderWorld(t, http.HandlerFunc(func(out http.ResponseWriter, request *http.Request) {
		if request.Method != "GET" || request.URL.Path != "/volumes/"+name {
			t.Error("unexpected volume observation", request.Method, request.URL.Path)
			out.WriteHeader(http.StatusForbidden)
			return
		}
		reads++
		_ = json.NewEncoder(out).Encode(r.Anchor)
	}))
	a := w.offline.api
	a.volumes = map[string]*anchorVolume{name: {name: name, labels: approvalClone(object(r.Anchor["Labels"])), options: approvalClone(object(r.Anchor["Options"]))}}
	observed, err := a.observeMountAnchor(r.Observation)
	if err != nil || reads != 1 || observed["CreatedAt"] != r.Anchor["CreatedAt"] || observed["Mountpoint"] != r.Anchor["Mountpoint"] {
		t.Fatal("missing independent volume inspection", observed, reads, err)
	}
	object(r.Anchor["Options"])["device"] = "/substituted"
	if _, err = a.observeMountAnchor(r.Observation); err == nil || reads != 2 {
		t.Fatal("cached request authorized a changed native volume", reads, err)
	}
	delete(a.volumes, name)
	if _, err = a.observeMountAnchor(r.Observation); err == nil || reads != 2 {
		t.Fatal("unrecorded worker anchor was inspected or accepted", reads, err)
	}
}
