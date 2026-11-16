package features

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Keep error identity for callers while bounding and quoting its displayed text.
// In particular, adding context must not discard an earlier barrier/guard error.
type lifecycleEvidenceError struct {
	text   string
	causes []error
}

func (e *lifecycleEvidenceError) Error() string   { return e.text }
func (e *lifecycleEvidenceError) Unwrap() []error { return e.causes }

func lifecycleQuoted(text string, limit int) string {
	if len(text) > limit {
		return strconv.Quote(text[:limit]) + fmt.Sprintf(" (truncated, omitted %d bytes)", len(text)-limit)
	}
	return strconv.Quote(text)
}

func lifecycleErrorText(err error) string {
	if err == nil {
		return "none"
	}
	return lifecycleQuoted(err.Error(), 512)
}

func lifecycleOffset(origin, at time.Time) string {
	if origin.IsZero() {
		return "origin unavailable"
	}
	if at.IsZero() {
		return "missing"
	}
	return at.Sub(origin).String()
}

func (w *world) lifecycleResultEvidence(code int) error {
	l := w.lifecycle
	if l == nil {
		l = &lifecycleNative{}
	}
	p := l.run
	if p == nil && l.control != nil && l.err != nil {
		// The main command never started. Its failed independent control already
		// carries bounded evidence; nesting it through another text cap loses the
		// control's request journal before the main row's first Then.
		var evidence *lifecycleEvidenceError
		if errors.As(l.err, &evidence) {
			return &lifecycleEvidenceError{text: "lifecycle healthy control failed before main invocation: " + evidence.Error(), causes: []error{l.err}}
		}
	}
	collected := false
	if p != nil {
		select {
		case <-p.done:
			collected = true
		default:
		}
	}
	journal, recorderErr := w.lifecycleJournal(p)
	causes := []error{l.err, recorderErr}
	process := "controller unstarted"
	stderr, processErr := "unavailable", "unavailable"
	if p != nil && !p.started.IsZero() {
		process = "controller not collected; exit unavailable"
	}
	if collected {
		// Wait/guard fields are written before closing done, never read earlier.
		causes = append(causes, p.err)
		classification := "after readiness deadline"
		if p.started.IsZero() || p.finished.IsZero() {
			classification = "timing unavailable"
		} else if p.finished.Before(p.started.Add(engineReadinessAllowance)) {
			classification = "before readiness deadline"
		}
		if l.harnessCancellation != "" {
			classification = "harness cancelled " + l.harnessCancellation
		}
		process = fmt.Sprintf("controller exit %d (%s), expected %d; elapsed=%s limit=%s",
			p.exit, classification, code, lifecycleOffset(p.started, p.finished), engineReadinessAllowance)
		if p.started.IsZero() {
			process = "controller unstarted; " + process
		}
		stderr = lifecycleQuoted(string(p.errout.bytes()), 1024)
		processErr = lifecycleErrorText(p.err)
		if p.exit == code && errors.Join(causes...) == nil && l.harnessCancellation == "" && !p.started.IsZero() && !p.finished.IsZero() {
			return nil
		}
	}
	return &lifecycleEvidenceError{causes: slices.DeleteFunc(causes, func(e error) bool { return e == nil }),
		text: fmt.Sprintf("lifecycle failure cause=%s; %s; mode=%s condition=%s; process/guard error=%s; recorder error=%s\nstderr=%s\n%s",
			lifecycleErrorText(l.err), process, lifecycleQuoted(l.mode, 128), lifecycleQuoted(l.condition, 128),
			processErr, lifecycleErrorText(recorderErr), stderr, journal)}
}

// Snapshot only the diagnostic allowlist under the recorder lock. No body,
// query, protected record, environment, argv or inspection/configuration map.
func (w *world) lifecycleJournal(p *recoveryProcess) (string, error) {
	if w.offline == nil || w.offline.api == nil {
		return "engine recorder unavailable; journal unavailable", nil
	}
	a := w.offline.api
	// A printed create always shows its outcome, never how long it had been open.
	a.awaitCreates()
	a.mu.Lock()
	defer a.mu.Unlock()
	origin := time.Time{}
	if p != nil {
		origin = p.started
	}
	if len(a.requests) == 0 {
		return "engine journal empty; total=0 omitted=0", a.problem
	}
	indices := lifecycleJournalSelection(a.requests)
	var out strings.Builder
	fmt.Fprintf(&out, "engine journal total=%d omitted=%d", len(a.requests), len(a.requests)-len(indices))
	if len(indices) < len(a.requests) {
		out.WriteString(" (truncated)")
	}
	out.WriteByte('\n')
	for _, i := range indices {
		e := a.requests[i]
		// Path normally has no query; strip it defensively even in synthetic input.
		path, _, _ := strings.Cut(e.Path, "?")
		fmt.Fprintf(&out, "#%d %s %s HTTP %d cancelled=%t arrival=%s reply=%s diagnostic=%s",
			i+1, lifecycleQuoted(e.Method, 16), lifecycleQuoted(path, 192), e.Status, e.Cancelled,
			lifecycleOffset(origin, e.At), lifecycleOffset(origin, e.Replied), lifecycleQuoted(e.Diagnostic, 256))
		if e.Method == "POST" && path == "/containers/create" {
			// Whether or not a reply followed: what the recorder spent before it
			// forwarded, and what the engine spent.
			fmt.Fprintf(&out, " forwarded=%s answer=%s", lifecycleOffset(origin, e.Forwarded), lifecycleOffset(origin, e.Answered))
		}
		if e.State != nil {
			// Type-check scalar values so a malformed field cannot dump a nested map.
			status, _ := e.State["Status"].(string)
			message, _ := e.State["Error"].(string)
			running, runningOK := e.State["Running"].(bool)
			exit, exitOK := e.State["ExitCode"].(float64)
			fmt.Fprintf(&out, " state status=%s running=%t observed=%t exit=%g observed=%t error=%s",
				lifecycleQuoted(status, 64), running, runningOK, exit, exitOK, lifecycleQuoted(message, 256))
		}
		out.WriteByte('\n')
	}
	return out.String(), a.problem
}

func lifecycleJournalSelection(entries []offlineRequest) []int {
	const limit = 12
	selected := map[int]bool{}
	// Preserve the first failed/refused/cancelled request and the latest failure,
	// even if both fall outside the ordinary progress tail.
	for i, e := range entries {
		if e.Status >= 400 || e.Cancelled {
			selected[i] = true
			break
		}
	}
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.Status >= 400 || e.Cancelled {
			selected[i] = true
			break
		}
	}
	selected[0] = true
	// Every create and the request before it, the image inspection that opens the
	// product's create call: together they say how that call's time was spent.
	for i, e := range entries {
		if e.Method == "POST" && e.Path == "/containers/create" && len(selected)+2 <= limit {
			selected[i] = true
			if i > 0 {
				selected[i-1] = true
			}
		}
	}
	for i := len(entries) - 1; i >= 0 && len(selected) < limit; i-- {
		selected[i] = true
	}
	indices := make([]int, 0, len(selected))
	for i := range selected {
		indices = append(indices, i)
	}
	slices.Sort(indices)
	return indices
}
