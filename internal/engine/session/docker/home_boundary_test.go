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
