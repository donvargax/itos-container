package disk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// readyLimit is the most bytes a ready record has before its newline, as the
// ready read bounds its line. A longer line is never ready and is never held.
const readyLimit = 2048

var (
	errRefusalCopyEnded  = errors.New("refusal copy closed")
	errRefusalCopyFailed = errors.New("refusal copy unavailable")
)

// RefusalLog copies a stopped proxy's stdout into the session's protected
// directory through approval's staged log, storing every record but ready.
type RefusalLog struct {
	Store   Store
	Storage approval.SessionLogs
}

// Begin stages a copy for the public session ID under the registry's own
// lookup key, so run's sessions and stop's sit under one candidate.
func (l RefusalLog) Begin(ctx context.Context, session string) (port.RefusalCopy, port.Outcome) {
	if !port.HexID(session) {
		return nil, port.Invalid
	}
	c, failure := Registry{Store: l.Store}.lookup()
	if failure != "" {
		return nil, failure
	}
	staged, f := l.Storage.Stage(ctx, c, session)
	if f != "" {
		return nil, storageFailure(f, false)
	}
	if staged == nil {
		return nil, port.Refused
	}
	return &refusalCopy{staged: staged}, ""
}

// refusalCopy releases its staged log exactly once, on every path. Write and
// Commit run one at a time, each to its end; Close alone never waits for them,
// because a copy whose call timed out is closed while a write or an fsync that
// cannot be interrupted may still be running.
type refusalCopy struct {
	// turn orders Write and Commit; only its holder calls the staged log, or
	// Close does once no holder remains.
	turn   sync.Mutex
	staged approval.StagedLog
	held   []byte
	long   bool
	// mu guards ended and writing, the only state Close reads.
	mu      sync.Mutex
	ended   bool
	writing bool
}

// Write stores every complete line it holds before it returns and keeps only an
// unfinished one. It reports every byte it was given, dropped and held included.
func (c *refusalCopy) Write(p []byte) (int, error) {
	c.turn.Lock()
	defer c.turn.Unlock()
	c.mu.Lock()
	if c.ended {
		c.mu.Unlock()
		return 0, errRefusalCopyEnded
	}
	c.writing = true
	c.mu.Unlock()
	var failure approval.Failure
	if out := c.lines(p); len(out) > 0 {
		failure = c.staged.Write(out)
	}
	c.mu.Lock()
	c.writing = false
	closed := c.ended
	c.mu.Unlock()
	if closed {
		// Close arrived while this Write ran and left the release to it.
		c.staged.Close()
	}
	if failure != "" {
		return 0, errRefusalCopyFailed
	}
	return len(p), nil
}

// lines returns the bytes to store now: every line that p completes, except a
// ready record, and the whole of a line already past readyLimit, which cannot
// be one. What remains held is an unfinished line of at most readyLimit bytes.
func (c *refusalCopy) lines(p []byte) []byte {
	var out []byte
	for {
		at := bytes.IndexByte(p, '\n')
		if at < 0 {
			c.held = append(c.held, p...)
			if c.long || len(c.held) > readyLimit {
				out, c.held, c.long = append(out, c.held...), c.held[:0], true
			}
			return out
		}
		c.held = append(c.held, p[:at]...)
		if c.long || len(c.held) > readyLimit || !readyRecord(c.held) {
			out = append(append(out, c.held...), '\n')
		}
		c.held, c.long, p = c.held[:0], false, p[at+1:]
	}
}

// Commit stores a held unfinished line as it is, then publishes. On a live copy
// it always runs the staged log's Publish, once: after a failed write the handle
// is poisoned, so Publish refuses and still removes the temporary. T-51 derives
// no report from the bytes.
func (c *refusalCopy) Commit() (port.RefusalReport, port.Outcome) {
	c.turn.Lock()
	defer c.turn.Unlock()
	c.mu.Lock()
	ended := c.ended
	c.ended = true
	c.mu.Unlock()
	if ended {
		return port.RefusalReport{}, port.Incomplete
	}
	if len(c.held) > 0 {
		_ = c.staged.Write(c.held)
	}
	if f := c.staged.Publish(); f != "" {
		return port.RefusalReport{}, storageFailure(f, false)
	}
	return port.RefusalReport{}, ""
}

// Close ends the copy at once. A Write still running closes the staged log when
// it returns; a Commit that had begun is not undone and releases it itself.
func (c *refusalCopy) Close() {
	c.mu.Lock()
	release := !c.ended && !c.writing
	c.ended = true
	c.mu.Unlock()
	if release {
		c.staged.Close()
	}
}

// readyRecord judges a line as the ready read does (session/docker's
// validReady): one JSON object with exactly the keys schema, seq, time and
// event, schema 1, seq 1, event ready and an RFC 3339 UTC time ending in Z, with
// nothing after the object. An adapter may not import a sibling adapter, and the
// port's allowlist has no JSON, so the judgement is repeated here.
func readyRecord(data []byte) bool {
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return false
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return false
		}
		key, ok := token.(string)
		if !ok || fields[key] != nil {
			return false
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return false
		}
		fields[key] = value
	}
	token, err = d.Token()
	if err != nil || token != json.Delim('}') || d.InputOffset() != int64(len(data)) || len(fields) != 4 {
		return false
	}
	var schema, seq int
	var event, stamp string
	if json.Unmarshal(fields["schema"], &schema) != nil || schema != 1 || json.Unmarshal(fields["seq"], &seq) != nil || seq != 1 || json.Unmarshal(fields["event"], &event) != nil || event != "ready" || json.Unmarshal(fields["time"], &stamp) != nil || !strings.HasSuffix(stamp, "Z") {
		return false
	}
	_, err = time.Parse(time.RFC3339Nano, stamp)
	return err == nil
}
