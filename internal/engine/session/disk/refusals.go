package disk

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"slices"
	"strconv"
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
	return &refusalCopy{staged: staged, digest: newSha256hash()}, ""
}

// refusalCopy releases its staged log exactly once, on every path. Write and
// Commit run one at a time, each to its end; Close alone never waits for them,
// because a copy whose call timed out is closed while a write or an fsync that
// cannot be interrupted may still be running.
type refusalCopy struct {
	// turn orders Write and Commit; only its holder calls the staged log, or
	// Close does once no holder remains.
	turn      sync.Mutex
	staged    approval.StagedLog
	held      []byte
	long      bool
	sequences []uint64
	records   uint64
	digest    sha256hash
	// mu guards ended and writing, the only state Close reads, and is held
	// over Close's own release of the staged log and over no other call to it.
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
		if failure == "" {
			_, _ = c.digest.Write(out)
		}
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
// Every complete line's sequence is observed, ready included, so Commit judges
// the whole stream before readiness is filtered out of the stored bytes.
func (c *refusalCopy) lines(p []byte) []byte {
	var out []byte
	for {
		at := bytes.IndexByte(p, '\n')
		if at < 0 {
			c.held = append(c.held, p...)
			if c.long || len(c.held) > readyLimit {
				// A line already past readyLimit can never be ready; store its
				// bytes as they arrive. It stays unfinished until a newline.
				out = append(out, c.held...)
				c.held, c.long = c.held[:0], true
			}
			return out
		}
		c.held = append(c.held, p[:at]...)
		if seq, ok := lineSequence(c.held); ok {
			c.sequences = append(c.sequences, seq)
		}
		if c.long || len(c.held) > readyLimit || !readyRecord(c.held) {
			out = append(append(out, c.held...), '\n')
			c.records++
		}
		c.held, c.long, p = c.held[:0], false, p[at+1:]
	}
}

// Commit stores a held unfinished line as it is, then publishes the log with
// its own report beside it. On a live copy it always runs the staged log's
// Publish, once: after a failed write the handle is poisoned, so Publish
// refuses and still removes the temporary. The report derives from the bytes
// this copy staged: the published record count and the inclusive sequence
// ranges the source never delivered, ready's deliberate omission included.
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
		if f := c.staged.Write(c.held); f == "" {
			_, _ = c.digest.Write(c.held)
			if seq, ok := lineSequence(c.held); ok {
				c.sequences = append(c.sequences, seq)
			}
			c.records++
		}
	}
	report := port.RefusalReport{Records: c.records, Path: c.staged.Path()}
	report.Gaps = refusalGaps(c.sequences)
	if len(report.Gaps) > 0 {
		report.Outcome = port.RefusalLogGap
	}
	if f := c.staged.Attach(refusalReportJSON(c.digest.Sum(), report)); f != "" {
		c.staged.Publish()
		return port.RefusalReport{}, storageFailure(f, false)
	}
	if f := c.staged.Publish(); f != "" {
		return port.RefusalReport{}, storageFailure(f, false)
	}
	return report, ""
}

// Close ends the copy at once. A Write still running closes the staged log when
// it returns; a Commit that had begun is not undone and releases it itself.
// With neither running Close releases the staged log under mu, so a Write or a
// Commit that finds the copy ended returns only after that release. Neither
// holds mu across a call to the staged log, so Close still waits for none.
func (c *refusalCopy) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.ended && !c.writing {
		c.staged.Close()
	}
	c.ended = true
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

// sha256hash keeps the published file's digest over exactly the bytes staged,
// held lines included, without retaining a second copy of the log.
type sha256hash struct{ inner hash.Hash }

func newSha256hash() sha256hash                  { return sha256hash{inner: sha256.New()} }
func (h sha256hash) Write(p []byte) (int, error) { return h.inner.Write(p) }
func (h sha256hash) Sum() string                 { return hex.EncodeToString(h.inner.Sum(nil)) }

// lineSequence reads one complete line's sequence number. A line that is not a
// JSON object with a small non-negative integer seq observes nothing; it is
// still stored, never discarded.
func lineSequence(line []byte) (uint64, bool) {
	var record struct {
		Seq *json.Number `json:"seq"`
	}
	if json.Unmarshal(line, &record) != nil || record.Seq == nil {
		return 0, false
	}
	value, err := strconv.ParseUint(record.Seq.String(), 10, 32)
	if err != nil || value == 0 {
		return 0, false
	}
	return value, true
}

// refusalGaps returns the inclusive sequence ranges the source never delivered,
// from the first event a log must name to the last one it did. A deliberately
// filtered ready record still observes its sequence, so its omission from the
// published file is not itself a gap.
func refusalGaps(sequences []uint64) []port.RefusalGap {
	if len(sequences) == 0 {
		return nil
	}
	seen := slices.Clone(sequences)
	slices.Sort(seen)
	seen = slices.Compact(seen)
	var gaps []port.RefusalGap
	next := uint64(1)
	for _, seq := range seen {
		if seq > next {
			gaps = append(gaps, port.RefusalGap{First: next, Last: seq - 1})
		}
		next = seq + 1
	}
	return gaps
}

// refusalReportJSON renders the report a committed copy binds to its own bytes:
// the log's digest, its published record count and the ranges it never saw.
// The shape is the fixture's, so an independently seeded original reads back
// through the same port as a copy this adapter published.
func refusalReportJSON(digest string, report port.RefusalReport) []byte {
	missing := []map[string]uint64{}
	for _, gap := range report.Gaps {
		missing = append(missing, map[string]uint64{"first": gap.First, "last": gap.Last})
	}
	data, err := json.Marshal(map[string]any{"schema": 1, "sha256": digest, "records": report.Records, "missing": missing})
	if err != nil {
		return []byte("{}")
	}
	return data
}

// Read verifies the committed report against the log in the same session directory.
func (l RefusalLog) Read(ctx context.Context, session string) (port.RefusalLogReport, port.Outcome) {
	report := port.RefusalLogReport{State: "unknown", Missing: []port.RefusalGap{}}
	if !port.HexID(session) {
		return report, port.Invalid
	}
	c, failure := (Registry{Store: l.Store}).lookup()
	if failure != "" {
		return report, failure
	}
	evidence, f := l.Storage.Evidence(ctx, c, session)
	report.Path = evidence.Path
	if f != "" {
		return report, storageFailure(f, false)
	}
	if evidence.Log == nil {
		if evidence.Report != nil {
			return report, port.Incomplete
		}
		report.State = "pending"
		return report, ""
	}
	if evidence.Report == nil {
		return report, ""
	}
	var sidecar struct {
		Schema  int               `json:"schema"`
		SHA256  string            `json:"sha256"`
		Records *uint64           `json:"records"`
		Missing []port.RefusalGap `json:"missing"`
	}
	if json.Unmarshal(evidence.Report, &sidecar) != nil || sidecar.Schema != 1 || sidecar.Records == nil || !hexDigest(sidecar.SHA256) {
		return report, ""
	}
	if sha256.Sum256(evidence.Log) != mustDigest(sidecar.SHA256) {
		return report, ""
	}
	records := *sidecar.Records
	report.Records = &records
	report.Missing = append([]port.RefusalGap(nil), sidecar.Missing...)
	if report.Missing == nil {
		report.Missing = []port.RefusalGap{}
	}
	if len(report.Missing) > 0 {
		report.State = "gapped"
	} else {
		report.State = "stored"
	}
	return report, ""
}

func hexDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func mustDigest(value string) [sha256.Size]byte {
	sum, _ := hex.DecodeString(value)
	var digest [sha256.Size]byte
	copy(digest[:], sum)
	return digest
}
