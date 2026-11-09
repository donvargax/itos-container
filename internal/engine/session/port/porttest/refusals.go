package porttest

import (
	"context"
	"errors"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

type RefusalLog struct {
	Copies map[string][]byte
	Report port.RefusalReport
	Fault  string
}

func (*RefusalLog) Begin(context.Context, string) (port.RefusalCopy, port.Outcome) {
	return &refusalCopy{}, ""
}

type refusalCopy struct{}

func (*refusalCopy) Write([]byte) (int, error) { return 0, errors.New("copy unavailable") }
func (*refusalCopy) Commit() (port.RefusalReport, port.Outcome) {
	return port.RefusalReport{}, port.Incomplete
}
func (*refusalCopy) Close() {}
