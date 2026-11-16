package disk

import (
	"bytes"
	"encoding/json"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// Execution evidence is scoped by the protected intent's project and token.
// It is published before the intent; it never changes the approval snapshot.
type egressEvidence struct {
	ProxyImageID string `json:"proxy_image_id"`
	Schema       int    `json:"schema"`
}

func executionBytes(id string) []byte {
	data, _ := json.Marshal(egressEvidence{ProxyImageID: id, Schema: 1})
	return data
}

func (t *intent) publishExecution(id string) port.Outcome {
	b := t.binding
	if b.Egress.Names == "" {
		if id != "" || b.Egress != (port.EgressGrant{}) {
			return port.Invalid
		}
		return ""
	}
	if id == "" || (b.Egress.ImageID != "" && b.Egress.ImageID != id) {
		return port.Invalid
	}
	b.Egress.ImageID = id
	if !b.Egress.Valid() {
		return port.Invalid
	}
	name := t.record.NameToken() + ".egress.json"
	old, f := t.directory.Read(name)
	if f != "" {
		return storageFailure(f, true)
	}
	if old != nil {
		return port.Refused
	}
	if f = t.directory.Publish(name, nil, executionBytes(id)); f != "" {
		return storageFailure(f, true)
	}
	t.binding = b
	return ""
}

// Reopening uses only original protected evidence, including for cleanup after
// policy, approval or cached-image changes. Missing/conflicting facts fail closed.
func (t *intent) execution(b port.Binding) (port.Binding, port.Outcome) {
	if b.Egress == (port.EgressGrant{}) {
		return b, ""
	}
	data, f := t.directory.Read(t.record.NameToken() + ".egress.json")
	if f != "" {
		return port.Binding{}, storageFailure(f, false)
	}
	var e egressEvidence
	if len(data) > 1024 || json.Unmarshal(data, &e) != nil || e.Schema != 1 || !bytes.Equal(data, executionBytes(e.ProxyImageID)) || (b.Egress.ImageID != "" && b.Egress.ImageID != e.ProxyImageID) {
		return port.Binding{}, port.Invalid
	}
	b.Egress.ImageID = e.ProxyImageID
	if !b.Egress.Valid() {
		return port.Binding{}, port.Invalid
	}
	return b, ""
}
