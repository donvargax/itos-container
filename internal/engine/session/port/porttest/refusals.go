package porttest

import (
	"context"
	"errors"
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"slices"
	"sync"
	"sync/atomic"
)

type RefusalLog struct {
	mu     sync.Mutex
	Copies map[string][]byte
	Report port.RefusalReport
	Fault  string
}

func (l *RefusalLog) Begin(ctx context.Context, id string) (port.RefusalCopy, port.Outcome) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if ctx.Err() != nil || l.Fault == "begin" {
		return nil, port.Incomplete
	}
	report := l.Report
	report.Gaps = slices.Clone(report.Gaps)
	return &refusalCopy{log: l, id: id, fault: l.Fault, report: report}, ""
}

type refusalCopy struct {
	mu                sync.Mutex
	ended             atomic.Bool
	log               *RefusalLog
	id, fault         string
	data              []byte
	report            port.RefusalReport
	failed, committed bool
}

func (c *refusalCopy) Write(data []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ended.Load() || c.fault == "write" {
		c.failed = true
		return 0, errors.New("refusal copy unavailable")
	}
	c.data = append(c.data, data...)
	return len(data), nil
}
func (c *refusalCopy) Commit() (port.RefusalReport, port.Outcome) {
	if c.ended.Swap(true) {
		return port.RefusalReport{}, port.Incomplete
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failed || c.fault == "commit" {
		return port.RefusalReport{}, port.Incomplete
	}
	c.log.mu.Lock()
	defer c.log.mu.Unlock()
	if c.log.Copies == nil {
		c.log.Copies = map[string][]byte{}
	}
	c.log.Copies[c.id] = slices.Clone(c.data)
	c.committed = true
	report := c.report
	report.Gaps = slices.Clone(report.Gaps)
	return report, ""
}
func (c *refusalCopy) Close() { c.ended.Store(true); c.mu.Lock(); defer c.mu.Unlock(); c.data = nil }
