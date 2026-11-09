// Package porttest holds the egress port's in-memory fakes and its shared
// contract. It does no I/O, so an adapter's tests and the handler's can share it.
package porttest

import (
	"errors"
	"sync"
)

// Log is an in-memory port.Log: it keeps a copy of every line written to it.
type Log struct {
	mu    sync.Mutex
	lines [][]byte
}

// NewLog returns an empty log.
func NewLog() *Log { return &Log{} }

// Write keeps a copy of line, so a caller may reuse its buffer.
func (l *Log) Write(line []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, append([]byte(nil), line...))
	return len(line), nil
}

// Lines returns a copy of every write so far, in order.
func (l *Log) Lines() [][]byte {
	l.mu.Lock()
	defer l.mu.Unlock()
	lines := make([][]byte, len(l.lines))
	for i, line := range l.lines {
		lines[i] = append([]byte(nil), line...)
	}
	return lines
}

// FailingLog is a port.Log whose every write fails, as a closed pipe's does.
type FailingLog struct{}

// Write fails without keeping anything.
func (FailingLog) Write([]byte) (int, error) { return 0, errors.New("log write failed") }
