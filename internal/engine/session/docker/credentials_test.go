package docker

import (
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
	"sync"
	"testing"
)

func TestCredentialCreateRefusesInvalidOrUnreadyStaging(t *testing.T) {
	for _, state := range []string{"ready", "grant mismatch", "unready"} {
		t.Run(state, func(t *testing.T) {
			fixture := credentialSDKFixture(t)
			connection, failure := fixture.Engine.OpenRun(t.Context(), fixture.Binding)
			if failure != "" {
				t.Fatal("credential connection setup failed", failure)
			}
			defer connection.Close()
			r := fixture.Record
			switch state {
			case "grant mismatch":
				r.Credentials.Kinds.GitHub = false
				r.Credentials.Files[1] = ""
				if !r.Credentials.Ready() || r.ValidCredentials(fixture.Binding) {
					t.Fatal("grant mismatch fixture must retain ready staging")
				}
			case "unready":
				r.Stage = "reserved"
				r.Credentials.Files = [2]string{}
				if !r.ValidCredentials(fixture.Binding) || r.Credentials.Ready() {
					t.Fatal("unready fixture must retain valid credential metadata")
				}
			}
			id, failure := connection.CreateRun(t.Context(), r, []string{"literal", "argument"})
			creates, starts := fixture.Counts()
			if state == "ready" {
				if id == "" || failure != "" || creates != 1 || starts != 0 {
					t.Fatal("ready staging create positive control failed", failure)
				}
			} else if id != "" || failure != port.CredentialRefused || creates != 0 || starts != 0 {
				t.Fatal("invalid or unready credential staging reached create", failure)
			}
		})
	}
}

func credentialSDKFixture(t *testing.T, tokens ...[]byte) f.CredentialEngineFixture {
	t.Helper()
	var mu sync.Mutex
	var resource *container.InspectResponse
	var traffic [][]byte
	var requested []port.CredentialMount
	var requestedArgv []string
	var defect string
	var refuseRemoval bool
	creates, starts := 0, 0
	var paths = []byte("synthetic-credential\n")
	if len(tokens) > 0 {
		paths = append([]byte(nil), tokens[0]...)
	}
	var acquired []byte
	engine, b, _ := sdkHandlerFixture(t, func(w http.ResponseWriter, r *http.Request, b port.Binding) {
		mu.Lock()
		defer mu.Unlock()
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
			creates++
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
			requestedArgv = append([]string(nil), request.Cmd...)
			if !h.ReadonlyRootfs || h.IpcMode != "private" || h.RestartPolicy.Name != "no" || request.User != b.UID+":"+b.GID || len(request.Entrypoint) != 0 || h.Init == nil || *h.Init || len(h.CapDrop) != 1 || h.CapDrop[0] != "ALL" {
				t.Error("credential create changed hardening")
			}
			if h.NetworkMode != "none" || h.Privileged || h.AutoRemove || h.PublishAllPorts || h.PidsLimit == nil || *h.PidsLimit != 4096 || h.LogConfig.Type != "none" || h.CgroupnsMode != "private" || h.PidMode != "" || h.UsernsMode != "" || len(h.CapAdd) != 0 || len(h.GroupAdd) != 0 || len(h.Devices) != 0 || len(h.DeviceRequests) != 0 || len(h.PortBindings) != 0 || len(h.Binds) != 0 || len(h.VolumesFrom) != 0 || len(h.Links) != 0 || len(h.ExtraHosts) != 0 || len(h.SecurityOpt) != 1 || h.SecurityOpt[0] != "no-new-privileges" || len(h.Tmpfs) != 1 || h.Tmpfs["/tmp"] != "rw,nosuid,nodev,mode=1777" {
				t.Error("raw credential request weakened host isolation")
			}
			if len(h.Mounts) == 0 || h.Mounts[0].Type != mount.TypeVolume || h.Mounts[0].Source != f.RunIntent(b).Anchor.Name || h.Mounts[0].Target != "/workspace" || h.Mounts[0].ReadOnly || h.Mounts[0].VolumeOptions == nil || !h.Mounts[0].VolumeOptions.NoCopy {
				t.Error("raw credential request changed checkout anchor")
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
				raw, _ := json.Marshal(resource)
				var observed container.InspectResponse
				_ = json.Unmarshal(raw, &observed)
				credentialDefect(&observed, defect)
				response = observed
			}
		case strings.HasSuffix(path, "/start"):
			starts++
			acquired = append([]byte(nil), paths...)
			status = 500
			response = map[string]string{"message": "lost acknowledgement"}
		case r.Method == "DELETE":
			if refuseRemoval {
				status = 500
				response = map[string]string{"message": "synthetic remove failure"}
				break
			}
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
	b.Environment = "{}"
	b.Workspace = "/workspace"
	r := f.CredentialIntent(port.CredentialKinds{Claude: true, GitHub: true})
	r.Stage = "intent"
	r.Credentials.Stage = "owned"
	r.Credentials.Device = "1"
	r.Credentials.Inode = "2"
	r.Credentials.Files = [2]string{"1:3", "1:4"}
	b.Credentials = r.Credentials.Kinds
	return f.CredentialEngineFixture{Argv: func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), requestedArgv...) }, Defect: func(f string) { mu.Lock(); defer mu.Unlock(); defect = f }, FailRemoval: func(f bool) { mu.Lock(); defer mu.Unlock(); refuseRemoval = f }, Counts: func() (int, int) { mu.Lock(); defer mu.Unlock(); return creates, starts }, Engine: engine, Binding: b, Record: r, Mounts: func() []port.CredentialMount {
		mu.Lock()
		defer mu.Unlock()
		return append([]port.CredentialMount(nil), requested...)
	}, Traffic: func() [][]byte { mu.Lock(); defer mu.Unlock(); return append([][]byte(nil), traffic...) }, Unlink: func() { mu.Lock(); defer mu.Unlock(); paths = nil }, Acquired: func() []byte { mu.Lock(); defer mu.Unlock(); return append([]byte(nil), acquired...) }, Finish: func() {
		mu.Lock()
		defer mu.Unlock()
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
	r.Credentials.Files = [2]string{"1:3", "1:4"}
	b.Credentials = r.Credentials.Kinds
	engine := &f.RunEngine{Engine: e, StartFailure: port.ExecutionFailed, CredentialPaths: []byte("synthetic-credential\n")}
	return f.CredentialEngineFixture{Argv: func() []string { return append([]string(nil), engine.Argv...) }, Defect: func(fault string) {
		o := e.Resources[e.CreateID]
		switch fault {
		case "host source", "material source":
			o.CredentialMounts[0].Source = "/original/token"
		case "host target", "material target":
			o.CredentialMounts[0].Target = "/wrong"
		case "owner":
			o.ImageID = "foreign"
		case "anchor":
			o.AnchorExact = false
		default:
			o.CredentialExact = false
		}
		e.Resources[e.CreateID] = o
	}, FailRemoval: func(f bool) {
		if f {
			e.Following = "still present"
		} else {
			e.Following = ""
		}
	}, Counts: func() (int, int) {
		return countCredentialEvent(engine.Events, "create"), countCredentialEvent(engine.Events, "start")
	}, Engine: engine, Binding: b, Record: r, Mounts: func() []port.CredentialMount { return engine.CredentialMounts }, Traffic: func() [][]byte { return nil }, Unlink: func() { engine.CredentialPaths = nil }, Acquired: func() []byte { return engine.CredentialAcquired }, Finish: func() {
		o := e.Resources[e.CreateID]
		o.Started = true
		o.Running = true
		o.Status = "running"
		e.Resources[e.CreateID] = o
	}}
}
func TestCredentialMountContract(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		credentialKindsContract(t, credentialMemoryFixture)
		f.CredentialMountRefusalContract(t, credentialMemoryFixture)
	})
	t.Run("sdk", func(t *testing.T) {
		credentialKindsContract(t, func() f.CredentialEngineFixture { return credentialSDKFixture(t) })
		f.CredentialMountRefusalContract(t, func() f.CredentialEngineFixture { return credentialSDKFixture(t) })
	})
}

func TestCredentialMountInspectionStdin(t *testing.T) {
	for _, fault := range []string{"tty", "closed stdin", "reusable stdin"} {
		t.Run(fault, func(t *testing.T) {
			fixture := credentialSDKFixture(t)
			c, fail := fixture.Engine.OpenRun(t.Context(), fixture.Binding)
			if fail != "" {
				t.Fatal("engine setup refused", fail)
			}
			defer c.Close()
			argv := []string{"probe"}
			id, fail := c.CreateRun(t.Context(), fixture.Record, argv)
			if fail != "" {
				t.Fatal("credential create refused", fail)
			}
			if fail := c.VerifyRun(t.Context(), id, argv); fail != "" {
				t.Fatal("healthy credential inspection refused", fail)
			}
			fixture.Defect(fault)
			if fail := c.VerifyRun(t.Context(), id, argv); fail != port.IsolationUnverified {
				t.Fatal("changed credential worker stdin configuration accepted", fail)
			}
			if creates, starts := fixture.Counts(); creates != 1 || starts != 0 {
				t.Fatal("inspection refusal created or started another worker")
			}
		})
	}
}
func TestCredentialUncertainStartContract(t *testing.T) {
	t.Run("memory", func(t *testing.T) { f.CredentialUncertainStartContract(t, credentialMemoryFixture) })
	t.Run("sdk", func(t *testing.T) {
		f.CredentialUncertainStartContract(t, func() f.CredentialEngineFixture { return credentialSDKFixture(t) })
	})
}

func TestCredentialProvisioningAbsence(t *testing.T) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal("marker setup failed")
	}
	marker := []byte("itos-synthetic-credential-" + hex.EncodeToString(random[:]))
	if !f.CredentialMarkerPresent(marker, marker) {
		t.Fatal("raw detector positive control failed")
	}
	for offset := 0; offset < 3; offset++ {
		if !f.CredentialMarkerPresent([]byte(base64.StdEncoding.EncodeToString(append(make([]byte, offset), marker...))), marker) {
			t.Fatal("encoded detector positive control failed")
		}
	}
	fixture := credentialSDKFixture(t, marker)
	c, fail := fixture.Engine.OpenRun(t.Context(), fixture.Binding)
	if fail != "" {
		t.Fatal("engine setup failed")
	}
	defer c.Close()
	id, fail := c.CreateRun(t.Context(), fixture.Record, []string{"probe"})
	if fail != "" {
		t.Fatal("provisioning positive control refused", fail)
	}
	if fail = c.VerifyRun(t.Context(), id, []string{"probe"}); fail != "" {
		t.Fatal("healthy provisioning inspection refused")
	}
	failure := c.Start(t.Context(), id)
	fixture.Unlink()
	fixture.Finish()
	_, _ = c.Inspect(t.Context(), id)
	if !f.CredentialMarkerPresent(fixture.Acquired(), marker) {
		t.Fatal("acquired reference positive control failed")
	}
	c.RemoveRun(t.Context(), id)
	if len(fixture.Acquired()) != 0 {
		t.Fatal("worker removal retained an acquired reference")
	}
	if f.CredentialMarkerPresent([]byte(failure), marker) {
		t.Fatal("credential marker in adapter failure")
	}
	for _, surface := range fixture.Traffic() {
		if f.CredentialMarkerPresent(surface, marker) {
			t.Fatal("credential marker in Engine API traffic")
		}
	}
}

func countCredentialEvent(events []string, name string) int {
	n := 0
	for _, event := range events {
		if event == name {
			n++
		}
	}
	return n
}
func credentialDefect(r *container.InspectResponse, fault string) {
	if fault == "" {
		return
	}
	if fault == "owner" {
		r.Config.Labels["io.itos-container.intent"] = "foreign"
		return
	}
	if fault == "anchor" {
		r.HostConfig.Mounts[0].Source = "foreign"
		return
	}
	switch fault {
	case "tty":
		r.Config.Tty = true
		return
	case "closed stdin":
		r.Config.OpenStdin = false
		return
	case "reusable stdin":
		r.Config.StdinOnce = false
		return
	}
	if strings.HasPrefix(fault, "host ") {
		m := &r.HostConfig.Mounts[1]
		switch fault {
		case "host source":
			m.Source = "/original/token"
		case "host target":
			m.Target = "/wrong"
		case "host type":
			m.Type = mount.TypeVolume
		case "host writable":
			m.ReadOnly = false
		case "host missing":
			r.HostConfig.Mounts = r.HostConfig.Mounts[:1]
		case "host duplicate":
			r.HostConfig.Mounts = append(r.HostConfig.Mounts, *m)
		}
	} else {
		m := &r.Mounts[1]
		switch fault {
		case "material source":
			m.Source = "/original/token"
		case "material target":
			m.Destination = "/wrong"
		case "material type":
			m.Type = mount.TypeVolume
		case "material writable":
			m.RW = true
		case "material missing":
			r.Mounts = r.Mounts[:1]
		case "material duplicate":
			r.Mounts = append(r.Mounts, *m)
		}
	}
}

func credentialKindsContract(t *testing.T, create func() f.CredentialEngineFixture) {
	t.Helper()
	for _, kinds := range []port.CredentialKinds{{Claude: true}, {GitHub: true}, {Claude: true, GitHub: true}, {}} {
		f.CredentialMountContract(t, func() f.CredentialEngineFixture {
			fixture := create()
			fixture.Binding.Credentials = kinds
			if kinds == (port.CredentialKinds{}) {
				fixture.Record = f.RunIntent(fixture.Binding)
			} else {
				fixture.Record.Credentials.Kinds = kinds
				if !kinds.Claude {
					fixture.Record.Credentials.Files[0] = ""
				}
				if !kinds.GitHub {
					fixture.Record.Credentials.Files[1] = ""
				}
			}
			return fixture
		})
	}
}

type credentialContractReporter struct{ failures int }

func (*credentialContractReporter) Helper()                 {}
func (r *credentialContractReporter) Errorf(string, ...any) { r.failures++ }
func TestCredentialUncertainStartOracleRejectsUnlinkRevocation(t *testing.T) {
	report := &credentialContractReporter{}
	f.CredentialUncertainStartContract(report, func() f.CredentialEngineFixture {
		fixture := credentialMemoryFixture()
		unlink := fixture.Unlink
		fixture.Unlink = func() { unlink(); fixture.Engine.(*f.RunEngine).CredentialAcquired = nil }
		return fixture
	})
	if report.failures == 0 {
		t.Fatal("uncertain-start oracle accepted unlink revoking an acquired reference")
	}
}
