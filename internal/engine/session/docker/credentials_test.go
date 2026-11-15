package docker

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"io"
	"net/http"
	"strings"
	"testing"
)

func credentialSDKFixture(t *testing.T, tokens ...[]byte) f.CredentialEngineFixture {
	t.Helper()
	var resource *container.InspectResponse
	var traffic [][]byte
	var requested []port.CredentialMount
	var paths = []byte("synthetic-credential\n")
	if len(tokens) > 0 {
		paths = append([]byte(nil), tokens[0]...)
	}
	var acquired []byte
	engine, b, _ := sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, b port.Binding) {
		path := strings.TrimPrefix(r.URL.Path, "/v1.56")
		body, _ := io.ReadAll(r.Body)
		traffic = append(traffic, append([]byte(r.Method+" "+r.URL.String()+"\n"), body...))
		var response any
		status := 200
		switch {
		case path == "/info":
			response = map[string]string{"ID": b.EngineID}
		case strings.HasPrefix(path, "/images/"):
			response = map[string]any{"Id": b.ImageID, "Config": map[string]any{}}
		case path == "/containers/create":
			var request struct {
				container.Config
				HostConfig *container.HostConfig
			}
			if json.Unmarshal(body, &request) != nil || request.HostConfig == nil {
				t.Error("unreadable raw create request")
				w.WriteHeader(400)
				return
			}
			h := request.HostConfig
			if !h.ReadonlyRootfs || h.IpcMode != "private" || h.RestartPolicy.Name != "no" || request.User != b.UID+":"+b.GID || len(request.Entrypoint) != 0 || h.Init == nil || *h.Init || len(h.CapDrop) != 1 || h.CapDrop[0] != "ALL" {
				t.Error("credential create changed hardening")
			}
			resource = &container.InspectResponse{ID: strings.Repeat("c", 64), Name: "/" + r.URL.Query().Get("name"), Image: b.ImageID, Config: &request.Config, HostConfig: h, State: &container.State{Status: "created"}, NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{"none": {}}}}
			for _, m := range h.Mounts {
				if m.Type == mount.TypeBind {
					if !m.ReadOnly {
						t.Error("writable credential bind")
					}
					kind := strings.TrimPrefix(m.Target, "/run/itos-container/credentials/")
					requested = append(requested, port.CredentialMount{Kind: kind, Source: m.Source, Target: m.Target})
					resource.Mounts = append(resource.Mounts, container.MountPoint{Type: m.Type, Source: m.Source, Destination: m.Target, RW: !m.ReadOnly})
				} else {
					resource.Mounts = append(resource.Mounts, container.MountPoint{Type: m.Type, Name: m.Source, Driver: "local", Destination: m.Target, RW: !m.ReadOnly})
				}
			}
			response = map[string]string{"Id": resource.ID}
			status = 201
		case strings.HasSuffix(path, "/json"):
			if resource == nil {
				status = 404
			} else {
				response = resource
			}
		case strings.HasSuffix(path, "/start"):
			acquired = append([]byte(nil), paths...)
			status = 500
			response = map[string]string{"message": "lost acknowledgement"}
		case r.Method == "DELETE":
			resource = nil
			acquired = nil
			status = 204
		default:
			t.Error("unexpected credential API request")
			status = 400
		}
		data, _ := json.Marshal(response)
		traffic = append(traffic, append([]byte(http.StatusText(status)+"\n"), data...))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status != 204 {
			_, _ = w.Write(data)
		}
	})
	r := f.CredentialIntent(port.CredentialKinds{Claude: true, GitHub: true})
	r.Stage = "intent"
	r.Credentials.Stage = "owned"
	r.Credentials.Device = "1"
	r.Credentials.Inode = "2"
	b.Credentials = r.Credentials.Kinds
	return f.CredentialEngineFixture{Engine: engine, Binding: b, Record: r, Mounts: func() []port.CredentialMount { return requested }, Traffic: func() [][]byte { return traffic }, Unlink: func() { paths = nil }, Acquired: func() []byte { return acquired }, Finish: func() {
		if resource != nil {
			resource.State = &container.State{Status: "running", Running: true, Pid: 123, StartedAt: "2026-01-01T00:00:00Z"}
		}
	}}
}
func credentialMemoryFixture() f.CredentialEngineFixture {
	b, _, e := f.Fixture()
	r := f.CredentialIntent(port.CredentialKinds{Claude: true, GitHub: true})
	r.Stage = "intent"
	r.Credentials.Stage = "owned"
	r.Credentials.Device = "1"
	r.Credentials.Inode = "2"
	b.Credentials = r.Credentials.Kinds
	engine := &f.RunEngine{Engine: e, StartFailure: port.ExecutionFailed}
	return f.CredentialEngineFixture{Engine: engine, Binding: b, Record: r, Mounts: func() []port.CredentialMount { return engine.CredentialMounts }, Traffic: func() [][]byte { return nil }, Unlink: func() { engine.CredentialPaths = nil }, Acquired: func() []byte { return engine.CredentialAcquired }, Finish: func() {
		o := e.Resources[e.CreateID]
		o.Started = true
		o.Running = true
		o.Status = "running"
		e.Resources[e.CreateID] = o
	}}
}
func TestCredentialMountContract(t *testing.T) {
	t.Run("memory", func(t *testing.T) { f.CredentialMountContract(t, credentialMemoryFixture) })
	t.Run("sdk", func(t *testing.T) {
		f.CredentialMountContract(t, func() f.CredentialEngineFixture { return credentialSDKFixture(t) })
	})
}
func TestCredentialUncertainStartContract(t *testing.T) {
	t.Run("memory", func(t *testing.T) { f.CredentialUncertainStartContract(t, credentialMemoryFixture) })
	t.Run("sdk", func(t *testing.T) {
		f.CredentialUncertainStartContract(t, func() f.CredentialEngineFixture { return credentialSDKFixture(t) })
	})
}

func credentialLeak(surface []byte, marker []byte) bool {
	if bytes.Contains(surface, marker) {
		return true
	}
	for offset := 0; offset < 3; offset++ {
		encoded := base64.StdEncoding.EncodeToString(append(make([]byte, offset), marker...))
		start := (offset*8 + 5) / 6
		end := (offset + len(marker)) * 8 / 6
		if bytes.Contains(surface, []byte(encoded[start:end])) {
			return true
		}
	}
	return false
}
func TestCredentialProvisioningAbsence(t *testing.T) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal("marker setup failed")
	}
	marker := []byte("itos-synthetic-credential-" + hex.EncodeToString(random[:]))
	if !credentialLeak(marker, marker) {
		t.Fatal("raw detector positive control failed")
	}
	for offset := 0; offset < 3; offset++ {
		if !credentialLeak([]byte(base64.StdEncoding.EncodeToString(append(make([]byte, offset), marker...))), marker) {
			t.Fatal("encoded detector positive control failed")
		}
	}
	fixture := credentialSDKFixture(t, marker)
	c, fail := fixture.Engine.OpenRun(t.Context(), fixture.Binding)
	if fail != "" {
		t.Fatal("engine setup failed")
	}
	defer c.Close()
	_, fail = c.CreateRun(t.Context(), fixture.Record, []string{"probe"})
	if fail != "" {
		t.Fatal("provisioning positive control refused", fail)
	}
	for _, surface := range fixture.Traffic() {
		if credentialLeak(surface, marker) {
			t.Fatal("credential marker in Engine API traffic")
		}
	}
}
