package main

import (
	"archive/tar"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	rp "github.com/donvargax/itos-container/internal/engine/readiness/port"
	rf "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	sd "github.com/donvargax/itos-container/internal/engine/session/docker"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/volume"
)

type homeSDKFixture struct {
	mu       sync.Mutex
	f        *homeLocalFixture
	t        *testing.T
	endpoint string
	workers  map[string]container.InspectResponse
	anchors  map[string]volume.Volume
	streams  map[string]net.Conn
	created  []container.InspectResponse
	// imageEnv adds synthetic image defaults and inspected records every worker
	// inspection reply, as altered by the launch fixture's defect.
	imageEnv  []string
	inspected []container.InspectResponse
}
type homeSDKReadiness struct{ a *homeSDKFixture }

func (e homeSDKReadiness) OpenCatalog(selection string, budget time.Duration) (rp.Session, rp.Failure) {
	return e.Open(selection, "", budget)
}

func (a *homeSDKFixture) workerEvents() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.f.engine.Events)
}

func (e homeSDKReadiness) Open(_, _ string, _ time.Duration) (rp.Session, rp.Failure) {
	f := e.a.f
	if f.engine.Unavailable {
		return nil, rp.EngineUnavailable
	}
	if f.engine.Replaced {
		return nil, rp.EngineChanged
	}
	healthy := rf.HealthySession()
	info := healthy.InfoData
	info.ID = f.engine.DaemonID
	return &homeSDKPin{Readiness: sf.Readiness{EndpointValue: e.a.endpoint, Current: true, VersionValue: healthy.VersionData, InfoValue: info}, f: f}, rp.None
}

type homeSDKPin struct {
	sf.Readiness
	f *homeLocalFixture
}

func (p *homeSDKPin) Unchanged() bool { return p.Readiness.Unchanged() && !p.f.engine.Replaced }
func homeSDK(t *testing.T, f *homeLocalFixture) *homeSDKFixture {
	t.Helper()
	root, err := os.MkdirTemp("", "slice-34-sdk-")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "engine.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	a := &homeSDKFixture{f: f, t: t, endpoint: "unix://" + path, workers: map[string]container.InspectResponse{}, anchors: map[string]volume.Volume{}, streams: map[string]net.Conn{}}
	server := &http.Server{Handler: http.HandlerFunc(a.serve), ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Close()
		for _, c := range a.streams {
			_ = c.Close()
		}
		_ = os.RemoveAll(root)
	})
	return a
}
func (a *homeSDKFixture) engine() sp.RunEngine {
	return sd.Engine{Readiness: homeSDKReadiness{a}, Selection: a.endpoint}
}
func (a *homeSDKFixture) event(name string) {
	if a.f.engine.HomeBefore != nil {
		a.f.engine.HomeBefore(name)
	}
	a.f.engine.HomeEvents = append(a.f.engine.HomeEvents, name)
}
func (a *homeSDKFixture) serve(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/v1.56")
	write := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	f := a.f
	switch {
	case path == "/info":
		write(map[string]string{"ID": f.engine.DaemonID})
	case strings.HasPrefix(path, "/images/"):
		write(map[string]any{"Id": f.m.b.ImageID, "Config": map[string]any{"Env": append([]string{"HOME=/image-home", "TMPDIR=/image-temp", "XDG_CONFIG_HOME=/image-config", "XDG_DATA_HOME=/image-data", "XDG_STATE_HOME=/image-state", "XDG_CACHE_HOME=/image-cache"}, a.imageEnv...)}})
	case path == "/volumes/create":
		var v volume.Volume
		var body struct {
			Name, Driver       string
			Labels, DriverOpts map[string]string
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			w.WriteHeader(400)
			return
		}
		v = volume.Volume{Name: body.Name, Driver: body.Driver, Scope: "local", Labels: body.Labels, Options: body.DriverOpts, CreatedAt: "2026-11-16T01:00:00Z"}
		if body.Labels["io.itos-container.kind"] == "harness-home" {
			a.event("allocate")
			if _, exists := f.engine.Homes[v.Name]; exists {
				a.t.Error("occupied home reached VolumeCreate")
			}
			f.engine.Homes[v.Name] = sp.HomeObservation{Name: v.Name, Created: v.CreatedAt, Driver: v.Driver, Scope: v.Scope, Token: body.Labels["io.itos-container.token"], Key: body.Labels["io.itos-container.scope"], ExactLabels: len(body.Labels) == 3, EmptyOptions: len(body.DriverOpts) == 0}
			f.engine.Roots[v.Name] = sf.HomeRoot{UID: "0", GID: "0", Mode: 0755, Directory: true}
			if f.engine.HomeFault == "create" {
				w.WriteHeader(500)
				return
			}
		} else {
			a.anchors[v.Name] = v
		}
		w.WriteHeader(201)
		write(v)
	case strings.HasPrefix(path, "/volumes/"):
		name := strings.TrimPrefix(path, "/volumes/")
		if v, exists := a.anchors[name]; exists {
			if r.Method == "DELETE" {
				delete(a.anchors, name)
				w.WriteHeader(204)
			} else {
				write(v)
			}
			return
		}
		if r.Method == "DELETE" {
			a.event("remove")
			if r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true" {
				a.t.Error("forced home removal")
			}
			if f.engine.HomeFault == "remove" {
				w.WriteHeader(409)
				return
			}
			if r.Context().Err() != nil {
				return
			}
			delete(f.engine.Homes, name)
			delete(f.engine.Roots, name)
			w.WriteHeader(204)
			return
		}
		a.event("inspect")
		if f.engine.HomeFault == "inspect" {
			w.WriteHeader(500)
			return
		}
		o, exists := f.engine.Homes[name]
		if !exists {
			w.WriteHeader(404)
			return
		}
		labels := map[string]string{"io.itos-container.kind": "harness-home", "io.itos-container.scope": o.Key, "io.itos-container.token": o.Token}
		if !o.ExactLabels {
			labels["foreign"] = "true"
		}
		options := map[string]string{}
		if !o.EmptyOptions {
			options["device"] = "/outside"
		}
		write(volume.Volume{Name: o.Name, CreatedAt: o.Created, Driver: o.Driver, Scope: o.Scope, Labels: labels, Options: options})
	case path == "/containers/create":
		var body struct {
			container.Config
			HostConfig *container.HostConfig
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.HostConfig == nil {
			w.WriteHeader(400)
			return
		}
		id := fmt.Sprintf("%064x", len(a.created)+100)
		value := container.InspectResponse{ID: id, Name: "/" + r.URL.Query().Get("name"), Image: body.Image, Config: &body.Config, HostConfig: body.HostConfig, State: &container.State{Status: "created"}, NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{"none": {}}}}
		for _, m := range body.HostConfig.Mounts {
			value.Mounts = append(value.Mounts, container.MountPoint{Type: m.Type, Name: m.Source, Driver: "local", Destination: m.Target, RW: !m.ReadOnly})
			if m.Target == "/home/itos" {
				f.engine.Workers[id] = m.Source
			}
		}
		a.workers[id] = value
		a.created = append(a.created, value)
		f.engine.Events = append(f.engine.Events, "create")
		f.engine.Resources[id] = sp.Observation{ID: id, Name: strings.TrimPrefix(value.Name, "/"), ImageID: value.Image, Token: body.Labels["io.itos-container.intent"], Project: body.Labels["io.itos-container.project"], Fingerprint: body.Labels["io.itos-container.fingerprint"], StatePresent: true, Status: "created"}
		w.WriteHeader(201)
		write(map[string]string{"Id": id})
	case strings.HasPrefix(path, "/containers/"):
		id, operation, _ := strings.Cut(strings.TrimPrefix(path, "/containers/"), "/")
		value, exists := a.workers[id]
		if !exists {
			for _, v := range a.workers {
				if strings.TrimPrefix(v.Name, "/") == id {
					value, exists = v, true
					id = v.ID
					break
				}
			}
		}
		if !exists {
			w.WriteHeader(404)
			return
		}
		switch operation {
		case "json":
			data, _ := json.Marshal(value)
			var observed container.InspectResponse
			_ = json.Unmarshal(data, &observed)
			if f.boundary != "" {
				homeSDKDefect(&observed, f.boundary)
			}
			a.inspected = append(a.inspected, observed)
			write(observed)
		case "archive":
			name := f.engine.Workers[id]
			root := f.engine.Roots[name]
			if r.Method == "PUT" {
				a.event("initialize")
				if r.Context().Err() != nil {
					return
				}
				if f.engine.HomeFault == "initialize" {
					w.WriteHeader(500)
					return
				}
				header, err := tar.NewReader(r.Body).Next()
				if err != nil || header.Typeflag != tar.TypeDir || (header.Name != "." && header.Name != "/") || r.URL.Query().Get("path") != "/home/itos" || r.URL.Query().Get("copyUIDGID") != "true" {
					a.t.Error("unsafe initialization archive", header, r.URL)
					w.WriteHeader(400)
					return
				}
				// Moby skips an existing directory's dot entry. A successful
				// copy reply alone does not prove the metadata was applied.
				if header.Name == "." {
					w.WriteHeader(200)
					return
				}
				root.UID, root.GID, root.Mode, root.Directory = strconv.Itoa(header.Uid), strconv.Itoa(header.Gid), int(header.Mode), true
				f.engine.Roots[name] = root
				w.WriteHeader(200)
				return
			}
			a.event("root")
			if f.engine.HomeFault == "root unavailable" {
				w.WriteHeader(500)
				return
			}
			mode := os.FileMode(root.Mode)
			kind := byte(tar.TypeReg)
			if root.Directory {
				mode |= os.ModeDir
				kind = tar.TypeDir
			}
			stat, _ := json.Marshal(container.PathStat{Name: "itos", Mode: mode})
			w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(stat))
			if f.engine.HomeFault == "root archive" {
				_, _ = w.Write([]byte("incomplete archive"))
				return
			}
			var data bytes.Buffer
			tw := tar.NewWriter(&data)
			uid, _ := strconv.Atoi(root.UID)
			gid, _ := strconv.Atoi(root.GID)
			_ = tw.WriteHeader(&tar.Header{Name: "itos/", Typeflag: kind, Mode: int64(root.Mode), Uid: uid, Gid: gid})
			_ = tw.Close()
			_, _ = w.Write(data.Bytes())
		case "attach":
			c, buf, err := w.(http.Hijacker).Hijack()
			if err != nil {
				a.t.Error(err)
				return
			}
			a.streams[id] = c
			_, _ = buf.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
			_ = buf.Flush()
		case "start":
			f.engine.Events = append(f.engine.Events, "start")
			value.State = &container.State{Status: "exited", StartedAt: "2026-11-16T01:00:00Z"}
			a.workers[id] = value
			if c := a.streams[id]; c != nil {
				_ = c.Close()
				delete(a.streams, id)
			}
			w.WriteHeader(204)
		case "wait":
			write(map[string]int{"StatusCode": f.engine.Exit})
		case "kill":
			value.State = &container.State{Status: "exited", StartedAt: "2026-11-16T01:00:00Z"}
			a.workers[id] = value
			w.WriteHeader(204)
		case "":
			if r.Method != "DELETE" || r.URL.Query().Get("v") == "1" {
				a.t.Error("unexpected worker request", r.Method, r.URL)
				w.WriteHeader(400)
				return
			}
			delete(a.workers, id)
			delete(f.engine.Resources, id)
			w.WriteHeader(204)
		default:
			a.t.Error("unexpected worker operation", r.Method, r.URL)
			w.WriteHeader(400)
		}
	default:
		a.t.Error("unexpected SDK operation", r.Method, r.URL)
		w.WriteHeader(400)
	}
}
func homeSDKDefect(v *container.InspectResponse, defect string) {
	for i, m := range v.HostConfig.Mounts {
		if m.Target != "/home/itos" {
			continue
		}
		switch defect {
		case "wrong home volume":
			v.HostConfig.Mounts[i].Source = "foreign"
		case "host bind in place of home volume":
			v.HostConfig.Mounts[i].Type = mount.TypeBind
		case "read-only home mount":
			v.HostConfig.Mounts[i].ReadOnly = true
		case "image copy enabled on home mount":
			v.HostConfig.Mounts[i].VolumeOptions.NoCopy = false
		case "home target inside checkout":
			v.HostConfig.Mounts[i].Target = "/workspace/home"
		}
	}
	switch defect {
	case "HOME inherited from image":
		v.Config.Env = append(v.Config.Env, "HOME=/image")
	case "XDG_STATE_HOME inherited from image":
		v.Config.Env = append(v.Config.Env, "XDG_STATE_HOME=/image")
	case "TMPDIR points into persistent home":
		v.Config.Env = append(v.Config.Env, "TMPDIR=/home/itos")
	case "another unapproved mount":
		v.Mounts = append(v.Mounts, container.MountPoint{Type: mount.TypeVolume, Name: "foreign", Destination: "/foreign", RW: true})
	default:
		openCodeInspectionDefect(v, defect)
	}
}

// Seed recorded resources from independent fixture bytes, never a product run.
func (a *homeSDKFixture) seed() {
	a.mu.Lock()
	defer a.mu.Unlock()
	f := a.f
	for _, dir := range f.m.storage.Directories {
		for _, data := range dir.Files {
			var r sp.Record
			if json.Unmarshal(data, &r) != nil || r.Stage != "owned" && r.Stage != "intent" {
				continue
			}
			if r.ContainerID == "" {
				for id, observed := range f.engine.Resources {
					if observed.Name == r.Name {
						r.ContainerID = id
					}
				}
			}
			o, exists := f.engine.Resources[r.ContainerID]
			if !exists {
				continue
			}
			if _, exists = a.workers[r.ContainerID]; exists {
				continue
			}
			config := &container.Config{Image: r.ImageID, User: r.UID + ":" + r.GID, Labels: map[string]string{"io.itos-container.intent": r.NameToken(), "io.itos-container.project": r.Project, "io.itos-container.fingerprint": r.Fingerprint}}
			host := &container.HostConfig{}
			value := container.InspectResponse{ID: r.ContainerID, Name: "/" + r.Name, Image: r.ImageID, Config: config, HostConfig: host, State: &container.State{Status: container.ContainerState(o.Status), Running: o.Running, Pid: o.PID}}
			if o.Started {
				value.State.StartedAt = "2026-11-16T00:00:00Z"
			}
			if r.Schema >= 2 {
				host.Mounts = append(host.Mounts, mount.Mount{Type: mount.TypeVolume, Source: r.Anchor.Name, Target: "/workspace", VolumeOptions: &mount.VolumeOptions{NoCopy: true}})
				value.Mounts = append(value.Mounts, container.MountPoint{Type: mount.TypeVolume, Name: r.Anchor.Name, Driver: "local", Destination: "/workspace", RW: true})
				a.anchors[r.Anchor.Name] = volume.Volume{Name: r.Anchor.Name, CreatedAt: r.Anchor.Created, Driver: "local", Scope: "local", Options: map[string]string{"type": "none", "o": "bind", "device": r.Anchor.Source}, Labels: map[string]string{"io.itos-container.kind": "checkout-anchor", "io.itos-container.intent": r.NameToken(), "io.itos-container.project": r.Project, "io.itos-container.fingerprint": r.Fingerprint, "io.itos-container.session": r.Anchor.Session, "io.itos-container.device": r.Anchor.Device, "io.itos-container.inode": r.Anchor.Inode}}
			}
			if r.Home.Name != "" {
				host.Mounts = append(host.Mounts, mount.Mount{Type: mount.TypeVolume, Source: r.Home.Name, Target: "/home/itos", VolumeOptions: &mount.VolumeOptions{NoCopy: true}})
				value.Mounts = append(value.Mounts, container.MountPoint{Type: mount.TypeVolume, Name: r.Home.Name, Driver: "local", Destination: "/home/itos", RW: true})
				f.engine.Workers[r.ContainerID] = r.Home.Name
			}
			a.workers[r.ContainerID] = value
		}
	}
}
