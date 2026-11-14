package disk

import (
	"context"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

// stagedLogName is the one file a session directory stages. No caller chooses
// it, so none can aim a staged write at a record, a lock or a path.
const stagedLogName = "egress-refusals.jsonl"

// Stage stages nothing yet: the staged log's tests are committed first, red.
func (s Registry) Stage(context.Context, port.Candidate, string) (port.StagedLog, port.Failure) {
	return nil, ""
}
