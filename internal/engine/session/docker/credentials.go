package docker

import (
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"strings"
)

func safeCredentialMount(m mount.Mount, expected port.CredentialMount) bool {
	return expected.Target == port.CredentialTarget(expected.Kind) && expected.Target != "" && m.Type == mount.TypeBind && m.Source == expected.Source && m.Target == expected.Target && m.ReadOnly && m.BindOptions == nil && m.VolumeOptions == nil && m.TmpfsOptions == nil && m.ImageOptions == nil && m.Consistency == ""
}
func credentialObservation(r container.InspectResponse, o *port.Observation) {
	if r.HostConfig == nil {
		return
	}
	var expected []port.CredentialMount
	anchor := false
	for _, m := range r.HostConfig.Mounts {
		if m.Target == "/workspace" && !anchor && safeAnchorMount(m, m.Source) {
			o.AnchorName = m.Source
			anchor = true
			continue
		}
		kind := strings.TrimPrefix(m.Target, "/run/itos-container/credentials/")
		index := port.CredentialIndex(kind)
		if index < 0 || o.CredentialMounts[index] != (port.CredentialMount{}) {
			return
		}
		directory := strings.TrimSuffix(m.Source, "/"+kind)
		shape := port.CredentialStaging{Directory: directory, Stage: "intent", Kinds: port.CredentialKinds{Claude: kind == "claude-token", GitHub: kind == "github-token"}}
		credential := port.CredentialMount{Kind: kind, Source: m.Source, Target: m.Target}
		if !shape.Valid() || m.Source != directory+"/"+kind || !safeCredentialMount(m, credential) {
			return
		}
		o.CredentialMounts[index] = credential
		expected = append(expected, credential)
	}
	o.AnchorExact = anchor && safeMounts(r.Mounts, o.AnchorName, expected...)
	o.CredentialExact = o.AnchorExact
}
