package codec

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"

	manifest "github.com/donvargax/itos-container/internal/engine/manifest/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func (JSON) EncodePublic(r port.PublicRecord) []byte { data, _ := json.Marshal(r); return data }
func (j JSON) DecodePublic(data []byte) (port.PublicRecord, port.Outcome) {
	bad := func() (port.PublicRecord, port.Outcome) { return port.PublicRecord{}, port.Invalid }
	if len(data) == 0 || len(data) > 65536 || !publicDepth(data) {
		return bad()
	}
	v, issues := j.Parser.Parse(data, true)
	if len(issues) != 0 || v.Kind != manifest.Object || len(v.Fields) != 7 {
		return bad()
	}
	for _, name := range []string{"id", "project", "intent", "state"} {
		if v.Fields[name].Kind != manifest.String {
			return bad()
		}
	}
	if v.Fields["schema"].Kind != manifest.Number || v.Fields["schema"].Text != "1" {
		return bad()
	}
	for _, name := range []string{"workload_exit", "last_problem"} {
		if _, ok := v.Fields[name]; !ok {
			return bad()
		}
	}
	var r port.PublicRecord
	if json.Unmarshal(data, &r) != nil {
		return bad()
	}
	project, _, valid := port.ParseReference(r.Intent)
	if !port.HexID(r.ID) || !port.HexID(r.Project) || !valid || project != r.Project {
		return bad()
	}
	switch r.State {
	case "preparing", "ready", "running", "stopping", "stopped", "failed":
	default:
		return bad()
	}
	if r.WorkloadExit != nil {
		if v.Fields["workload_exit"].Kind != manifest.Number || strconv.Itoa(*r.WorkloadExit) != v.Fields["workload_exit"].Text || *r.WorkloadExit < 0 || *r.WorkloadExit > 255 {
			return bad()
		}
	}
	if r.LastProblem != nil {
		switch *r.LastProblem {
		case port.TeardownTimeout, port.StreamTimeout, port.EngineLost, port.Unsafe, port.Invalid, port.Refused, port.Uncertain, port.Incomplete, port.Environment, port.EnvironmentMissing, port.Internal, port.IntentWrite, port.OwnershipWrite, port.CompletionWrite, port.ExecutionFailed, port.IsolationInspection, port.IsolationUnverified:
		default:
			return bad()
		}
	}
	return r, ""
}

func publicDepth(data []byte) bool {
	d := json.NewDecoder(bytes.NewReader(data))
	depth := 0
	for {
		token, err := d.Token()
		if err == io.EOF {
			return depth == 0
		}
		if err != nil {
			return false
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{', '[':
				depth++
				if depth > 16 {
					return false
				}
			case '}', ']':
				depth--
			}
		}
	}
}
