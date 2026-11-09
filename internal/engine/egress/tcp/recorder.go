package tcp

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/donvargax/itos-container/internal/engine/egress/domain"
	"github.com/donvargax/itos-container/internal/engine/egress/port"
)

// recorder writes the refusal log: one JSON object and a newline to a write,
// numbered and written under one lock, so seq is the order of the writes. After
// a failed write, the limit or stop, it writes nothing more.
type recorder struct {
	out     port.Log
	numbers *domain.Log
	end     func(port.Failure)

	mu   sync.Mutex
	over bool
}

// write stamps and writes one record. It reports whether the caller may go on:
// false means the log has ended, with the failure that ended it already reported.
func (r *recorder) write(record port.Record) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.over {
		return false
	}
	stamped, full := r.numbers.Stamp(time.Now(), record)
	if r.emit(stamped) != nil {
		r.over = true
		r.end(port.LogFailed)
		return false
	}
	if full {
		last, _ := r.numbers.Stamp(time.Now(), port.Record{Event: port.LogFull})
		failure := port.LogExhausted
		if r.emit(last) != nil {
			failure = port.LogFailed
		}
		r.over = true
		r.end(failure)
		return false
	}
	return true
}

// emit encodes a record and writes it in one call.
func (r *recorder) emit(record port.Record) error {
	line, err := json.Marshal(record)
	if err == nil {
		_, err = r.out.Write(append(line, '\n'))
	}
	return err
}

// stop ends the log: no record is written after it returns.
func (r *recorder) stop() {
	r.mu.Lock()
	r.over = true
	r.mu.Unlock()
}
