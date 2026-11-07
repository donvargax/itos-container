package codec

import (
	"encoding/json"
	"path"

	manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func (JSON) EncodeAdmission(r port.AdmissionRecord) []byte { data, _ := json.Marshal(r); return data }
func (j JSON) DecodeAdmission(data []byte) (port.AdmissionRecord, port.Outcome) {
	bad := func() (port.AdmissionRecord, port.Outcome) { return port.AdmissionRecord{}, port.Invalid }
	if len(data) == 0 || len(data) > 65536 || !publicDepth(data) {
		return bad()
	}
	v, issues := j.Parser.Parse(data, true)
	if len(issues) != 0 || v.Kind != manifest.Object || (len(v.Fields) != 5 && len(v.Fields) != 6) {
		return bad()
	}
	if len(v.Fields) == 6 && (v.Fields["pending"].Kind != manifest.Boolean || v.Fields["pending"].Text != "true") {
		return bad()
	}
	if v.Fields["schema"].Kind != manifest.Number || v.Fields["schema"].Text != "1" || v.Fields["complete"].Kind != manifest.Boolean {
		return bad()
	}
	for _, name := range []string{"root", "id", "intent"} {
		if v.Fields[name].Kind != manifest.String {
			return bad()
		}
	}
	var r port.AdmissionRecord
	if json.Unmarshal(data, &r) != nil {
		return bad()
	}
	_, _, valid := port.ParseReference(r.Intent)
	if !valid || !port.HexID(r.ID) || !path.IsAbs(r.Root) || path.Clean(r.Root) != r.Root || (r.Pending && r.Complete) {
		return bad()
	}
	return r, ""
}
