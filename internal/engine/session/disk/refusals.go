package disk

import (
	"context"
	"errors"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// RefusalLog copies a stopped proxy's stdout into the session's protected
// directory through approval's staged log, storing every record but ready.
type RefusalLog struct {
	Store   Store
	Storage approval.SessionLogs
}

// Begin refuses every copy yet: the writer's tests are committed first, red.
func (l RefusalLog) Begin(context.Context, string) (port.RefusalCopy, port.Outcome) {
	return nil, port.Internal
}

// refusalCopy declares only what the tests read until the writer is built.
type refusalCopy struct{ held []byte }

func (c *refusalCopy) Write([]byte) (int, error) {
	return 0, errors.New("refusal copy unavailable")
}
func (c *refusalCopy) Commit() (port.RefusalReport, port.Outcome) {
	return port.RefusalReport{}, port.Internal
}
func (c *refusalCopy) Close() {}
