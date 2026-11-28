package features

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"time"
)

const (
	credentialCreateBarrier = "after accepted create before reply"
	credentialStartBarrier  = "after accepted start before reply"
	credentialRepeatReply   = "CREDRUN_REPEATED_START_REFUSED\n"
	credentialRemoveReply   = "CREDRUN_SYNTHETIC_REMOVE\n"
)

var (
	credentialEngineVersion    = regexp.MustCompile(`^/v[0-9]+\.[0-9]+/`)
	errCredentialReplyWithheld = errors.New("fixture withheld the engine reply")
)

type credentialRunFaults struct {
	LoseStart, FailRemoval bool
	Barrier                string
	StagedAbsent           func() (bool, error)
}

type credentialRunEnding struct {
	Method, Path                             string
	Arrived, Forwarded                       time.Time
	StagedAbsentAtArrival, Released, Expired bool
}

type credentialRunFaultLog struct {
	Worker                           string
	Creates, Starts, RefusedRemovals int
	StartAccepted                    time.Time
	StartReplyWithheld               bool
	Barrier                          string
	BarrierAt, BarrierEnded          time.Time
	Ending                           *credentialRunEnding
}

type credentialRunRecorder struct {
	armed, active               bool
	options                     credentialRunFaults
	log                         credentialRunFaultLog
	hold                        time.Duration
	accepted, held              chan string
	released, cleared           chan struct{}
	endingArrived, barrierEnded chan struct{}
	observationErr              error
}

func (j *credentialRunJournal) arm(options credentialRunFaults) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.faults.armed, j.faults.active = true, true
	j.faults.options = options
}

func (j *credentialRunJournal) accepted() <-chan string { return j.faults.accepted }
func (j *credentialRunJournal) held() <-chan string     { return j.faults.held }

func (j *credentialRunJournal) release() {
	j.mu.Lock()
	defer j.mu.Unlock()
	select {
	case <-j.faults.released:
	default:
		close(j.faults.released)
	}
}

func (j *credentialRunJournal) clear() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.faults.active = false
	select {
	case <-j.faults.cleared:
	default:
		close(j.faults.cleared)
	}
}

func (j *credentialRunJournal) faultLog() credentialRunFaultLog {
	j.mu.Lock()
	defer j.mu.Unlock()
	log := j.faults.log
	if log.Ending != nil {
		ending := *log.Ending
		log.Ending = &ending
	}
	return log
}

func (j *credentialRunJournal) faultObservationError() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.faults.observationErr
}

func credentialRunAPIPath(path string) string {
	return credentialEngineVersion.ReplaceAllString(path, "/")
}

func (j *credentialRunJournal) applyFault(w http.ResponseWriter, r *http.Request, entry *credentialJournalEntry) bool {
	path := credentialRunAPIPath(entry.metadata.Path)
	j.mu.Lock()
	f := &j.faults
	if !f.armed || f.log.Worker == "" {
		j.mu.Unlock()
		return false
	}
	worker := "/containers/" + f.log.Worker
	if r.Method == http.MethodPost && path == worker+"/start" {
		f.log.Starts++
		entry.firstStart = f.log.Starts == 1
		if f.active && (f.options.LoseStart || f.options.Barrier != "") && f.log.Starts > 1 {
			j.syntheticFault(w, entry, "repeated start", http.StatusConflict, credentialRepeatReply)
			j.mu.Unlock()
			return true
		}
	}
	remove := r.Method == http.MethodDelete && path == worker
	ending := remove || r.Method == http.MethodPost && (path == worker+"/kill" || path == worker+"/stop")
	if f.active && f.options.LoseStart && ending && f.log.Ending == nil {
		j.holdEnding(r.Context(), entry)
	}
	if f.active && f.options.FailRemoval && remove {
		f.log.RefusedRemovals++
		j.syntheticFault(w, entry, "removal refused", http.StatusServiceUnavailable, credentialRemoveReply)
		j.mu.Unlock()
		return true
	}
	j.mu.Unlock()
	return false
}

// Called with the journal locked. Reacquire it before publishing the outcome.
func (j *credentialRunJournal) holdEnding(ctx context.Context, entry *credentialJournalEntry) {
	f := &j.faults
	e := &credentialRunEnding{Method: entry.metadata.Method, Path: entry.metadata.Path, Arrived: entry.metadata.At}
	f.log.Ending = e
	if f.options.StagedAbsent != nil {
		e.StagedAbsentAtArrival, f.observationErr = f.options.StagedAbsent()
	}
	entry.metadata.Fault = "ending held"
	close(f.endingArrived)
	j.mu.Unlock()
	deadline := e.Arrived.Add(f.hold)
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	released := false
	select {
	case <-f.released:
		released = time.Now().Before(deadline) && ctx.Err() == nil
	case <-f.cleared:
	case <-ctx.Done():
	case <-timer.C:
	}
	j.mu.Lock()
	e.Forwarded, e.Released = time.Now(), released
	e.Expired = !released && !e.Forwarded.Before(deadline)
}

// Called with the journal locked. Synthetic responses use the same journal as upstream replies.
func (j *credentialRunJournal) syntheticFault(w http.ResponseWriter, entry *credentialJournalEntry, fault string, status int, body string) {
	entry.metadata.Fault, entry.metadata.Status = fault, status
	entry.metadata.ResponseHeaders = http.Header{"Content-Type": {"text/plain; charset=utf-8"}}
	_, _ = entry.response.data.WriteString(body)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func (j *credentialRunJournal) recordFaultReply(entry *credentialJournalEntry, response *http.Response) error {
	j.mu.Lock()
	armed := j.faults.armed
	j.mu.Unlock()
	if !armed || entry.metadata.Method != http.MethodPost {
		return nil
	}
	path := credentialRunAPIPath(entry.metadata.Path)
	if path == "/containers/create" && response.StatusCode == http.StatusCreated {
		return j.recordAcceptedCreate(entry, response)
	}
	j.mu.Lock()
	f := &j.faults
	if !f.active || f.log.Worker == "" || path != "/containers/"+f.log.Worker+"/start" ||
		response.StatusCode != http.StatusNoContent || !entry.firstStart {
		j.mu.Unlock()
		return nil
	}
	f.log.StartAccepted = time.Now()
	if f.options.Barrier == credentialStartBarrier {
		return j.holdBarrier(entry, response, nil)
	}
	if !f.options.LoseStart {
		j.mu.Unlock()
		return nil
	}
	j.withholdReply(entry, response, nil, "start reply withheld")
	f.accepted <- f.log.Worker
	j.mu.Unlock()
	ended := j.waitForClient(response.Request.Context())
	j.mu.Lock()
	f.log.StartReplyWithheld = ended
	j.mu.Unlock()
	return errCredentialReplyWithheld
}

func (j *credentialRunJournal) recordAcceptedCreate(entry *credentialJournalEntry, response *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(response.Body, credentialRunBodyLimit+1))
	if err != nil {
		return err
	}
	if len(body) > credentialRunBodyLimit {
		return errors.New("fixture create response exceeded the journal limit")
	}
	_ = response.Body.Close()
	response.Body = io.NopCloser(bytes.NewReader(body))
	var worker struct {
		ID string `json:"Id"`
	}
	_ = json.Unmarshal(body, &worker)
	j.mu.Lock()
	f := &j.faults
	f.log.Creates++
	if f.log.Creates == 1 {
		f.log.Worker = worker.ID
		if f.active && worker.ID != "" && f.options.Barrier == credentialCreateBarrier {
			return j.holdBarrier(entry, response, body)
		}
	}
	j.mu.Unlock()
	return nil
}

// Entered locked to publish the reply and barrier together before notifying the harness.
func (j *credentialRunJournal) holdBarrier(entry *credentialJournalEntry, response *http.Response, body []byte) error {
	f := &j.faults
	f.log.Barrier, f.log.BarrierAt = f.options.Barrier, time.Now()
	j.withholdReply(entry, response, body, f.log.Barrier)
	f.held <- f.log.Worker
	j.mu.Unlock()
	ctx := response.Request.Context()
	select {
	case <-ctx.Done():
	case <-f.cleared:
	}
	if ctx.Err() != nil {
		j.mu.Lock()
		f.log.BarrierEnded = time.Now()
		close(f.barrierEnded)
		j.mu.Unlock()
	}
	return errCredentialReplyWithheld
}

func (j *credentialRunJournal) waitForClient(ctx context.Context) bool {
	timer := time.NewTimer(engineCreateOutcomeAllowance)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return true
	case <-j.faults.cleared:
		return ctx.Err() != nil
	case <-timer.C:
		return ctx.Err() != nil
	}
}

func (j *credentialRunJournal) withholdReply(entry *credentialJournalEntry, response *http.Response, body []byte, fault string) {
	entry.metadata.Fault = fault
	entry.metadata.Status, entry.metadata.ResponseHeaders = response.StatusCode, response.Header.Clone()
	_, _ = entry.response.data.Write(body)
}
