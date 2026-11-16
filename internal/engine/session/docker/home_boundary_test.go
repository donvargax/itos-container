package docker

import (
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"strings"
	"testing"
)

func TestHomeWorkerRequiresBothExactRequestedMounts(t *testing.T) {
	id := strings.Repeat("a", 64)
	r := port.Record{Home: port.HomeReference{Scope: id, ID: id, Name: "itos-container-home-" + id}}
	for _, mounts := range [][]mount.Mount{nil, {{Type: mount.TypeVolume, Source: r.Home.Name, Target: "/home/itos"}}, {{}, {}, {}}} {
		if safeHomeWorkerHost(&container.HostConfig{Mounts: mounts}, r) {
			t.Fatal("incomplete requested mounts accepted", mounts)
		}
	}
	r.Home.Scope = "invalid"
	if safeHomeWorkerHost(&container.HostConfig{Mounts: []mount.Mount{{}, {}}}, r) {
		t.Fatal("invalid home record accepted")
	}
}

func TestHomeWorkerRejectsAlteredMountOptions(t *testing.T) {
	for _, defect := range []string{"valid", "type", "source", "read only", "bind options", "missing options", "copy", "subpath", "driver", "labels", "duplicate", "missing actual", "actual type", "actual name", "actual driver", "actual read only", "propagation", "duplicate actual"} {
		t.Run(defect, func(t *testing.T) {
			id := strings.Repeat("a", 64)
			r := port.Record{Home: port.HomeReference{Scope: id, ID: id, Name: "itos-container-home-" + id}}
			r.Anchor.Name = "checkout"
			pids, init := int64(4096), false
			h := &container.HostConfig{NetworkMode: "none", ReadonlyRootfs: true, CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges"},
				Resources: container.Resources{PidsLimit: &pids}, RestartPolicy: container.RestartPolicy{Name: "no"}, IpcMode: "private", CgroupnsMode: "private", Init: &init, LogConfig: container.LogConfig{Type: "none"}, Tmpfs: map[string]string{"/tmp": "rw,nosuid,nodev,mode=1777"},
				Mounts: []mount.Mount{{Type: mount.TypeVolume, Source: "checkout", Target: "/workspace", VolumeOptions: &mount.VolumeOptions{NoCopy: true}}, {Type: mount.TypeVolume, Source: r.Home.Name, Target: "/home/itos", VolumeOptions: &mount.VolumeOptions{NoCopy: true}}}}
			actual := []container.MountPoint{{Type: mount.TypeVolume, Name: "checkout", Driver: "local", Destination: "/workspace", RW: true}, {Type: mount.TypeVolume, Name: r.Home.Name, Driver: "local", Destination: "/home/itos", RW: true}}
			m := &h.Mounts[1]
			switch defect {
			case "type":
				m.Type = mount.TypeBind
			case "source":
				m.Source = "other"
			case "read only":
				m.ReadOnly = true
			case "bind options":
				m.BindOptions = &mount.BindOptions{}
			case "missing options":
				m.VolumeOptions = nil
			case "copy":
				m.VolumeOptions.NoCopy = false
			case "subpath":
				m.VolumeOptions.Subpath = "other"
			case "driver":
				m.VolumeOptions.DriverConfig = &mount.Driver{Name: "other"}
			case "labels":
				m.VolumeOptions.Labels = map[string]string{"other": "true"}
			case "duplicate":
				h.Mounts[0] = *m
			case "missing actual":
				actual = actual[:1]
			case "actual type":
				actual[1].Type = mount.TypeBind
			case "actual name":
				actual[1].Name = "other"
			case "actual driver":
				actual[1].Driver = "other"
			case "actual read only":
				actual[1].RW = false
			case "propagation":
				actual[1].Propagation = mount.PropagationShared
			case "duplicate actual":
				actual = append(actual, actual[1])
			}
			if got := safeHomeWorkerHost(h, r) && safeHomeMounts(actual, r); got != (defect == "valid") {
				t.Fatal("home mount boundary", got)
			}
		})
	}
}
