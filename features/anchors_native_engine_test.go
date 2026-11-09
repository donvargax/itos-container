package features

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (a *offlineAPI) anchorsBefore(out http.ResponseWriter, r *http.Request, q offlineRequest, index int) bool {
	a.mu.Lock()
	s := a.w.anchors
	if s == nil || q.Method != "DELETE" || !strings.HasPrefix(q.Path, "/volumes/") {
		a.mu.Unlock()
		return false
	}
	fail, hold := s.failDelete, s.mode == "pair" && !s.held
	if hold {
		s.held = true
		close(s.ready)
	}
	if fail {
		a.requests[index].Status = http.StatusServiceUnavailable
	}
	a.mu.Unlock()
	if fail {
		http.Error(out, "slice-17 volume release fault", http.StatusServiceUnavailable)
		return true
	}
	if hold {
		select {
		case <-s.release:
		case <-r.Context().Done():
			// The product abandoned the call, normally at its call budget: the
			// park outlived what the fixture may hold inside one product call.
			a.mu.Lock()
			s.parkLost = true
			a.mu.Unlock()
			return true
		}
	}
	return false
}

// Hold only an actual successful native reply, after the recorder has saved the
// independently inspected full ID. Recovery gets ordinary forwarded responses.
func (a *offlineAPI) anchorsResponse(r *http.Request, q offlineRequest, status int) {
	a.mu.Lock()
	s := a.w.anchors
	hold := s != nil && !s.held && status >= 200 && status < 300 && anchorRecoveryOperation(q, s.mode)
	if hold {
		s.held = true
		close(s.ready)
	}
	a.mu.Unlock()
	if hold {
		select {
		case <-s.release:
		case <-r.Context().Done():
		}
	}
}

func (w *world) anchorsForeign() {
	w.anchorsStart()
	w.anchorsLive()
	if w.anchors.initial.Sequence > 0 {
		w.anchors.later = w.anchorsProgress()
	}
	w.anchorsStatus()
	r := w.recovery
	if offlineHex(r.id) {
		first := w.recoveryStart("stop", r.id, "--json")
		first.wait()
		r.stops = append(r.stops, first)
		// The stop terminates the live primary. Let its foreground controller
		// finish its own bounded failure reporting before the fixture replacement.
		if w.anchorsFailedRelease() {
			r.run.wait()
		}
		// Refusal or missing producer stays data for Then. Never substitute a
		// source while the original worker might still use it.
		if w.anchorsFailedRelease() {
			r.err = errors.Join(r.err, w.anchorsSubstituteVolume())
		}
		a := w.offline.api
		a.mu.Lock()
		w.anchors.failDelete = false
		w.anchors.foreignIndex = len(a.requests)
		a.mu.Unlock()
		w.anchors.foreignStatus = w.recoveryStart("status", "--json")
		w.anchors.foreignStatus.wait()
		w.anchors.foreignStop = w.recoveryStart("stop", r.id, "--json")
		w.anchors.foreignStop.wait()
	}
	other := r.second
	other.anchorsStart()
	other.anchorsLive()
	if other.anchors.initial.Sequence > 0 {
		other.anchors.later = other.anchorsProgress()
		other.anchorsKill()
	}
	other.anchorsStatus()
	other.anchorsStopTwice()
}

func (w *world) anchorsFailedRelease() bool {
	a, r := w.offline.api, w.recovery
	a.mu.Lock()
	defer a.mu.Unlock()
	confirmed := false
	for _, q := range a.requests {
		if q.Method == "GET" && q.Path == "/containers/"+r.worker+"/json" && q.Status == 404 {
			confirmed = true
		}
		if confirmed && q.Method == "DELETE" && strings.HasPrefix(q.Path, "/volumes/") && q.Status == 503 {
			return true
		}
	}
	return false
}

// Trusted fixture mutation, outside the product recorder. It is independently
// identity checked, after worker absence, and excluded from product evidence.
func (w *world) anchorsSubstituteVolume() error {
	if len(w.anchor.volumes) != 1 || w.doctor.real.read("/containers/"+w.recovery.worker+"/json", nil, 404) != nil {
		return errors.New("foreign substitution lacks worker absence and original volume")
	}
	for _, v := range w.anchor.volumes {
		if err := w.anchorsDeleteFixtureVolume(v); err != nil {
			return err
		}
		foreign := &anchorVolume{name: v.name, labels: maps.Clone(v.labels), options: maps.Clone(v.options)}
		foreign.labels["io.itos-container.project"] = "slice-17-foreign-" + isolationNonce()
		w.anchors.foreign = foreign
		body := map[string]any{"Name": foreign.name, "Driver": "local", "Labels": foreign.labels, "DriverOpts": foreign.options}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		request, err := http.NewRequestWithContext(ctx, "POST", "http://engine/volumes/create", bytes.NewReader(approvalJSON(body)))
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := w.doctor.real.client.Do(request)
		if err != nil {
			return err
		}
		_ = response.Body.Close()
		if response.StatusCode != 201 {
			return fmt.Errorf("foreign fixture create HTTP %d", response.StatusCode)
		}
		foreign.observed, err = w.offline.api.anchorInspect(foreign)
		return err
	}
	return errors.New("missing original volume")
}

func (w *world) anchorsDeleteFixtureVolume(v *anchorVolume) error {
	if _, err := w.offline.api.anchorInspect(v); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "DELETE", "http://engine/volumes/"+url.PathEscape(v.name), nil)
	if err != nil {
		return err
	}
	response, err := w.doctor.real.client.Do(request)
	if err != nil {
		return err
	}
	_ = response.Body.Close()
	if response.StatusCode != 204 {
		return fmt.Errorf("fixture volume removal HTTP %d", response.StatusCode)
	}
	return w.doctor.real.read("/volumes/"+url.PathEscape(v.name), nil, 404)
}

func (w *world) closeAnchorsForeign() error {
	if w.anchors == nil || w.anchors.foreign == nil {
		return nil
	}
	return w.anchorsDeleteFixtureVolume(w.anchors.foreign)
}

func (w *world) anchorsPair() {
	r, s := w.recovery, w.anchors
	other := r.second
	for _, p := range []*world{w, other} {
		p.anchorsStart()
		p.anchorsLive()
	}
	if s.initial.Sequence > 0 {
		w.anchorsKill()
	}
	w.anchorsStatus()
	if offlineHex(r.id) {
		// A's cross stop and B's status are whole traced product processes. The park
		// holds the product's volume-delete call, which the product abandons at its
		// call budget (1 s in the engine fixture), so only the second stop's start
		// and B's heartbeat and volume observations may happen inside it. The cross
		// stop runs before the park, on the B ID B's live fixture already recorded.
		// B's status runs after both A stops, once A's worker and anchor are gone:
		// that is the one public status of B, so it must see any damage A's recovery
		// did to B's resources, lease or session. Each keeps its assertions.
		if offlineHex(other.recovery.id) {
			s.cross = w.recoveryStart("stop", other.recovery.id, "--json")
			s.cross.wait()
		}
		first := w.recoveryStart("stop", r.id, "--json")
		timer := time.NewTimer(32 * time.Second)
		select {
		case <-s.ready:
			s.reached = true
		case <-first.done:
		case <-timer.C:
		}
		timer.Stop()
		second := w.recoveryStart("stop", r.id, "--json")
		r.stops = append(r.stops, first, second)
		other.anchors.later = other.anchorsProgress()
		other.anchorsCapture()
		close(s.release)
		first.wait()
		second.wait()
		other.anchorsStatus()
		a := w.offline.api
		a.mu.Lock()
		if s.parkLost {
			r.err = errors.Join(r.err, errors.New("fixture parked A's volume release past the product call budget"))
		}
		a.mu.Unlock()
		var err error
		s.completed, err = w.anchorsCompletionRecords()
		r.err = errors.Join(r.err, err)
		a.mu.Lock()
		s.repeatedIndex = len(a.requests)
		a.mu.Unlock()
		repeat := w.recoveryStart("stop", r.id, "--json")
		repeat.wait()
		r.stops = append(r.stops, repeat)
	}
	r.after = w.recoveryStart("status", "--json")
	r.after.wait()
}
