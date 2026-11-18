package docker

import (
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/moby/moby/api/types/container"
	"maps"
	"slices"
	"strings"
)

func homeEnvironment(environment []string) []string {
	values := map[string]string{}
	for _, value := range environment {
		key, v, _ := strings.Cut(value, "=")
		values[key] = v
	}
	for key, value := range map[string]string{"HOME": "/home/itos", "TMPDIR": "/tmp", "XDG_CONFIG_HOME": "/home/itos/.config", "XDG_DATA_HOME": "/home/itos/.local/share", "XDG_STATE_HOME": "/home/itos/.local/state", "XDG_CACHE_HOME": "/home/itos/.cache"} {
		values[key] = value
	}
	result := make([]string, 0, len(values))
	for _, key := range slices.Sorted(maps.Keys(values)) {
		result = append(result, key+"="+values[key])
	}
	return result
}
func safeHomeWorkerHost(h *container.HostConfig, r port.Record) bool {
	if r.Home == (port.HomeReference{}) {
		return safeWorkerHost(h, r)
	}
	if !r.Home.Valid() || len(h.Mounts) != 2+len(r.Credentials.Mounts()) {
		return false
	}
	copy := *h
	copy.Mounts = nil
	for _, m := range h.Mounts {
		if m.Target == "/home/itos" {
			if !safeVolumeMount(m, r.Home.Name, "/home/itos") {
				return false
			}
		} else {
			copy.Mounts = append(copy.Mounts, m)
		}
	}
	return safeWorkerHost(&copy, r)
}
func safeHomeMounts(mounts []container.MountPoint, r port.Record) bool {
	return safeMounts(mounts, r.Anchor.Name, r.Home.Name, r.Credentials.Mounts()...)
}
