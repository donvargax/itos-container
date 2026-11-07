// Package codec validates session records and reuses approval's frozen codec.
package codec

import (
	"bytes"
	"encoding/json"
	"strconv"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

type JSON struct {
	Approval approval.FrozenRecords
	Parser   manifest.Parser
}

func (JSON) Encode(r port.Record) []byte { data, _ := json.Marshal(r); return data }
func (j JSON) Decode(data []byte) (port.Record, port.Outcome) {
	bad := func() (port.Record, port.Outcome) { return port.Record{}, port.Invalid }
	if len(data) == 0 || len(data) > 65536 {
		return bad()
	}
	value, issues := j.Parser.Parse(data, true)
	if len(issues) != 0 || value.Kind != manifest.Object || len(value.Fields) != 14 {
		return bad()
	}
	for _, name := range []string{"project", "generation", "fingerprint", "snapshot", "uid", "gid", "engine_endpoint", "engine_id", "name", "image_reference", "image_id", "container_id", "stage"} {
		if value.Fields[name].Kind != manifest.String {
			return bad()
		}
	}
	if value.Fields["schema"].Kind != manifest.Number || value.Fields["schema"].Text != "1" {
		return bad()
	}
	var record port.Record
	if json.Unmarshal(data, &record) != nil {
		return bad()
	}
	if !port.HexID(record.Project) || !port.HexID(record.NameToken()) {
		return bad()
	}
	n, err := strconv.ParseUint(record.Generation, 10, 64)
	if err != nil || n == 0 || strconv.FormatUint(n, 10) != record.Generation {
		return bad()
	}
	switch record.Stage {
	case "intent", "reserved", "cancelled":
		if record.ContainerID != "" {
			return bad()
		}
	case "owned", "absent":
		if !port.HexID(record.ContainerID) {
			return bad()
		}
	default:
		return bad()
	}
	return record, ""
}
func (j JSON) Frozen(f approval.Frozen) (port.Binding, port.Outcome) {
	record, failure := j.Approval.FrozenRecord(f)
	if failure != "" {
		return port.Binding{}, port.Invalid
	}
	return binding(record, f), ""
}
func (j JSON) Snapshot(data []byte) (port.Binding, port.Outcome) {
	var envelope struct {
		Project, Generation, Fingerprint string
		Settings                         json.RawMessage
	}
	if len(data) > 1024*1024 || json.Unmarshal(data, &envelope) != nil {
		return port.Binding{}, port.Invalid
	}
	f := approval.Frozen{Project: envelope.Project, Generation: envelope.Generation, Fingerprint: envelope.Fingerprint, Settings: string(envelope.Settings)}
	record, failure := j.Approval.FrozenRecord(f)
	if failure != "" || !bytes.Equal(j.Approval.Encode(record), data) {
		return port.Binding{}, port.Invalid
	}
	return binding(record, f), ""
}
func binding(r approval.Record, f approval.Frozen) port.Binding {
	var settings struct {
		Checkout struct {
			Root, Device, Inode string
			GitDevice           string `json:"git_device"`
			GitInode            string `json:"git_inode"`
		}
		Engine   struct{ Endpoint, ID string }
		Image    struct{ Reference, ID string }
		Manifest struct {
			WorkspaceFolder string          `json:"workspaceFolder"`
			ContainerEnv    json.RawMessage `json:"containerEnv"`
		}
	}
	_ = json.Unmarshal([]byte(r.Canonical), &settings)
	b := port.Binding{Frozen: f, UID: r.UID, GID: r.GID, Endpoint: settings.Engine.Endpoint, EngineID: settings.Engine.ID, ImageReference: settings.Image.Reference, ImageID: settings.Image.ID}
	b.Checkout.Root, b.Checkout.Device, b.Checkout.Inode = settings.Checkout.Root, settings.Checkout.Device, settings.Checkout.Inode
	b.Checkout.GitDevice, b.Checkout.GitInode = settings.Checkout.GitDevice, settings.Checkout.GitInode
	b.Workspace, b.Environment = settings.Manifest.WorkspaceFolder, string(settings.Manifest.ContainerEnv)
	return b
}
