package domain

import (
	"context"
	"slices"
	"time"

	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// Recovery observes and removes registered sessions; it has no launch path.
type Recovery struct {
	Refusals  port.RefusalLog
	Registry  port.Registry
	Engine    port.RunEngine
	immediate bool
}

func recoveryResult() port.RecoveryResult {
	return port.RecoveryResult{Entries: []port.RecoveryEntry{}, Failures: []port.RecoveryFailure{}}
}
func recoveryFailure(id string, failure port.Outcome) port.RecoveryResult {
	r := recoveryResult()
	if !port.HexID(id) {
		id = ""
	}
	r.Failures = append(r.Failures, port.RecoveryFailure{Session: id, Outcome: failure})
	return r
}
func (s Recovery) Status(ctx context.Context) port.RecoveryResult {
	s.Registry = finiteRegistry{s.Registry}
	s.Engine = finiteEngine{s.Engine}
	ctx, cancel := context.WithTimeout(ctx, budget.CallDuration)
	defer cancel()
	ids, failure := s.Registry.List(ctx)
	if failure != "" {
		return recoveryFailure("", failure)
	}
	slices.Sort(ids)
	r := recoveryResult()
	for _, id := range ids {
		if !port.HexID(id) {
			r.Failures = append(r.Failures, port.RecoveryFailure{Outcome: port.Invalid})
			continue
		}
		one := s.status(ctx, id)
		r.Entries = append(r.Entries, one.Entries...)
		r.Failures = append(r.Failures, one.Failures...)
	}
	return r
}
func (s Recovery) status(ctx context.Context, id string) port.RecoveryResult {
	entry, failure := s.Registry.Open(ctx, id, true)
	if failure != "" {
		return recoveryFailure(id, failure)
	}
	defer entry.Close()
	e, failure := sessionEntry(entry)
	ctx = context.WithValue(ctx, recoveryAnchorKey{}, recoveryAnchorState{session: entry, entry: &e})
	noCreate := uncreated(entry)
	if failure == "" && noCreate {
		failure = entry.Validate()
		if failure == "" {
			e.Resource = "absent"
		}
	}
	// Validated no-create evidence proves worker absence for either schema.
	// An anchored session still needs an independent volume observation.
	if failure == "" && (!noCreate || entry.Intent().Record().Schema >= 2) {
		connection, f := s.Engine.OpenRun(ctx, entry.Intent().Binding())
		failure = f
		if failure == "" {
			defer connection.Close()
			if !noCreate {
				_, e.Resource, failure = inspectSession(ctx, entry, connection)
			}
			if entry.Intent().Record().Schema >= 2 {
				_, anchorFailure := inspectRecoveryAnchor(ctx, entry.Intent(), connection)
				if anchorFailure != "" {
					failure = anchorFailure
				}
				if failure == "" {
					failure = inspectEgress(ctx, entry.Intent(), connection)
				}
			}
		}
	}
	if failure == "" {
		r := entry.Record()
		e.Cleanup = "pending"
		// Start may succeed before the controller can publish running. Report
		// fresh owned progress without rewriting the record or undoing cleanup.
		if e.Resource == "running" && (r.State == "preparing" || r.State == "ready") {
			e.State = "running"
		}
		if r.State == "stopped" {
			if e.Resource == "absent" && entry.Intent().Record().Completed() && (e.Anchor == "" || e.Anchor == "absent") {
				e.Cleanup = "complete"
			} else {
				failure = port.Incomplete
			}
		}
	}
	if failure != "" {
		e.State, e.Cleanup = "failed", "unknown"
		if e.Anchor == "" {
			e.Resource = "unknown"
		}
	}
	finishEntry(&e)
	r := recoveryResult()
	r.Entries = append(r.Entries, e)
	if failure != "" {
		r.Failures = append(r.Failures, port.RecoveryFailure{Session: id, Outcome: failure})
	}
	return r
}
func sessionEntry(s port.Session) (port.RecoveryEntry, port.Outcome) {
	r := s.Record()
	controller, failure := s.Controller()
	e := port.RecoveryEntry{ID: r.ID, Project: r.Project, Fingerprint: s.Intent().Record().Fingerprint, State: r.State, Resource: "unknown", Controller: controller, Cleanup: "unknown", WorkloadExit: r.WorkloadExit, MountDetector: r.MountDetector}
	if s.Intent().Record().Schema >= 2 {
		e.Anchor = "unknown"
	}
	return e, failure
}
func finishEntry(e *port.RecoveryEntry) {
	e.StopCandidate = e.Controller == "absent" && e.Cleanup != "complete"
	if e.Cleanup != "complete" {
		e.Retry = []string{"itos", "container", "stop", e.ID}
	} else {
		e.Retry = nil
	}
}

func inspectSession(ctx context.Context, s port.Session, c port.RunConnection) (observation port.Observation, resource string, failure port.Outcome) {
	defer func() {
		if state, ok := ctx.Value(recoveryAnchorKey{}).(recoveryAnchorState); ok {
			state.entry.Resource = resource
		}
	}()
	// A wait or prior engine call may have exhausted the total. Expired
	// storage handles cannot distinguish changed ownership from lost time.
	if budget.Cause(ctx) != nil {
		return port.Observation{}, "unknown", port.Incomplete
	}
	if failure := s.Validate(); failure != "" {
		return port.Observation{}, "unknown", failure
	}
	if failure := c.Check(ctx); failure != "" {
		return port.Observation{}, "unknown", failure
	}
	r := s.Intent().Record()
	id := r.ContainerID
	if id == "" {
		o, status := c.Inspect(ctx, r.Name)
		if status == port.Changed {
			return o, "unknown", port.Refused
		}
		if status != port.Observed {
			return o, "unknown", port.Uncertain
		}
		if !owns(r, o) {
			return o, "unknown", port.Refused
		}
		id = o.ID
	}
	o, status := c.Inspect(ctx, id)
	if status == port.NotFound && r.ContainerID != "" {
		return o, "absent", ""
	}
	if status == port.Changed {
		return o, "unknown", port.Refused
	}
	if status != port.Observed || !o.StatePresent {
		return o, "unknown", port.Incomplete
	}
	if o.ID != id || !owns(r, o) || r.Stage == "absent" || (r.Schema >= 2 && (!o.AnchorExact || o.AnchorName != r.Anchor.Name)) {
		return o, "unknown", port.Refused
	}
	switch {
	case o.Status == "running" && o.Running:
		return o, "running", ""
	case o.Status == "created" && !o.Running && !o.Started && o.PID == 0:
		return o, "created", ""
	case o.Status == "exited" && !o.Running && o.PID == 0:
		return o, "exited", ""
	default:
		return o, "unknown", port.Incomplete
	}
}

func (s Recovery) Stop(ctx context.Context, id string) port.RecoveryResult {
	s.Registry = finiteRegistry{s.Registry}
	s.Engine = finiteEngine{s.Engine}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return s.stop(ctx, id, "SIGTERM", nil, true)
}
func (s Recovery) stop(ctx context.Context, id, signal string, exit *int, remove bool) port.RecoveryResult {
	report := port.RefusalReport{}
	ctx = context.WithValue(ctx, egressRecoveryKey{}, egressRecoveryState{log: s.Refusals, report: &report})
	if !port.HexID(id) {
		return recoveryFailure("", port.Invalid)
	}
	entry, failure := s.Registry.Open(ctx, id, false)
	if failure != "" {
		return recoveryFailure(id, failure)
	}
	defer entry.Close()
	// Controller presence is advisory for explicit cleanup. Original authority
	// is independently revalidated by stopSession before any mutation.
	e, _ := sessionEntry(entry)
	ctx = context.WithValue(ctx, recoveryAnchorKey{}, recoveryAnchorState{session: entry, entry: &e})
	if uncreated(entry) && entry.Intent().Record().Schema == 1 {
		failure = cancelUncreated(entry)
	} else {
		var connection port.RunConnection
		connection, failure = s.Engine.OpenRun(ctx, entry.Intent().Binding())
		if failure == "" {
			defer connection.Close()
			if entry.Intent().Record().Schema >= 2 {
				failure = stopAnchoredSession(ctx, entry, connection, signal, exit, remove, s.immediate)
			} else {
				failure = stopSession(ctx, entry, connection, signal, exit, remove)
			}
		}
	}
	if failure == "" {
		e.State, e.Resource, e.Cleanup = "stopped", "absent", "complete"
		if !remove && entry.Record().State != "stopped" {
			e.State, e.Resource, e.Cleanup = entry.Record().State, "unknown", "pending"
		}
	} else {
		e.State, e.Cleanup = "failed", "unknown"
		if e.Anchor == "" {
			e.Resource = "unknown"
		}
		// Persist uncertainty only for safely held, incomplete sessions. Never
		// replace a completed tombstone or overwrite substituted authority.
		r := entry.Record()
		if budget.Cause(ctx) == nil && r.State != "stopped" && retryableRecovery(failure) && entry.Validate() == "" {
			r.State, r.LastProblem = "failed", &failure
			if f := entry.Save(r); f != "" {
				failure = f
			}
		}
	}
	e.WorkloadExit = entry.Record().WorkloadExit
	finishEntry(&e)
	r := recoveryResult()
	r.Entries = append(r.Entries, e)
	if report.Outcome == port.RefusalLogGap {
		r.Failures = append(r.Failures, port.RecoveryFailure{Session: id, Outcome: port.RefusalLogGap, Gaps: report.Gaps})
	}
	if failure != "" {
		r.Failures = append(r.Failures, port.RecoveryFailure{Session: id, Outcome: failure})
	}
	return r
}

func uncreated(s port.Session) bool {
	r := s.Intent().Record()
	return r.ContainerID == "" && (r.Stage == "reserved" || r.Stage == "cancelled")
}

func cancelUncreated(s port.Session) port.Outcome {
	if failure := s.Validate(); failure != "" {
		return failure
	}
	r := s.Record()
	if r.WorkloadExit != nil {
		return port.Refused
	}
	i := s.Intent().Record()
	i.Stage = "cancelled"
	if failure := s.Intent().Save(i); failure != "" {
		return failure
	}
	r.State, r.LastProblem = "stopped", nil
	return s.Save(r)
}

func stopSession(ctx context.Context, s port.Session, c port.RunConnection, signal string, exit *int, remove bool) port.Outcome {
	r := s.Record()
	o, resource, failure := inspectSession(ctx, s, c)
	if failure != "" && !retryableRecovery(failure) {
		return failure
	}
	// Inspection can consume the remaining total after lock acquisition. Do
	// not turn that timeout into an ownership refusal by saving with expired
	// storage authority, or begin another progress write beyond the budget.
	if budget.Cause(ctx) != nil {
		return port.Incomplete
	}
	// A verified explicit workload result can fill an unknown result even when
	// another controller completed cleanup. Lifecycle and authority stay fixed.
	if r.WorkloadExit == nil && exit != nil {
		r.WorkloadExit = exit
		if f := s.Save(r); f != "" {
			return f
		}
	}
	if r.State != "stopped" {
		r.State, r.LastProblem = "stopping", nil
		if failure := s.Save(r); failure != "" {
			return failure
		}
	}
	if failure != "" {
		return failure
	}
	if resource == "absent" {
		return finishOwnedSession(ctx, s, c)
	}
	if r.State == "stopped" {
		return port.Refused
	}
	// Reconciliation is durable before a destructive request can use the ID.
	i := s.Intent().Record()
	i.ContainerID, i.Stage = o.ID, "owned"
	if failure = s.Intent().Save(i); failure != "" {
		return failure
	}
	if resource == "exited" && ctx.Value(runGraceKey{}) != nil && s.Record().WorkloadExit == nil {
		if exit, f := c.Wait(ctx, i.ContainerID); f == "" {
			if f = recordExit(s, exit); f != "" {
				return f
			}
		}
	}
	if resource == "running" {
		if failure = terminateSession(ctx, s, c, signal); failure != "" {
			return failure
		}
	}
	// Foreground teardown can release the operation lock to drain output, then
	// reacquire fresh authority for removal within the same teardown budget.
	if !remove {
		return ""
	}
	// Fresh ownership follows signals, grace and every storage update.
	_, resource, failure = inspectSession(ctx, s, c)
	if failure != "" {
		return failure
	}
	if resource == "absent" {
		return finishOwnedSession(ctx, s, c)
	}
	if resource == "running" {
		return port.Incomplete
	}
	if failure = s.Validate(); failure != "" {
		return failure
	}
	if failure = c.Check(ctx); failure != "" {
		return failure
	}
	c.Remove(ctx, i.ContainerID)
	_, resource, failure = inspectSession(ctx, s, c)
	if failure != "" {
		return failure
	}
	if resource != "absent" {
		return port.Incomplete
	}
	return finishOwnedSession(ctx, s, c)
}

func retryableRecovery(f port.Outcome) bool {
	switch f {
	case port.Incomplete, port.Uncertain, port.IntentWrite, port.OwnershipWrite, port.CompletionWrite, port.EnvironmentMissing:
		return true
	default:
		return false
	}
}
func terminateSession(ctx context.Context, s port.Session, c port.RunConnection, signal string) port.Outcome {
	deadline, foreground := ctx.Value(runGraceKey{}).(time.Time)
	// Explicit stop retains slice-11's contract. Foreground run supplies the
	// cancellation deadline so delivery time cannot restart its grace.
	failure := recoverySignal(ctx, s, c, signal)
	if failure != "" && (!foreground || !retryableSignal(failure)) {
		return failure
	}
	grace, end := context.WithTimeout(ctx, budget.GraceDuration)
	if foreground {
		end()
		grace, end = context.WithDeadline(ctx, deadline)
	}
	defer end()
	exit, failure := c.Wait(grace, s.Intent().Record().ContainerID)
	if failure == "" {
		return recordExit(s, exit)
	}
	_, resource, failure := inspectSession(ctx, s, c)
	if failure != "" {
		return failure
	}
	if resource != "running" {
		if foreground && resource == "exited" {
			if exit, f := c.Wait(ctx, s.Intent().Record().ContainerID); f == "" {
				return recordExit(s, exit)
			}
		}
		return ""
	}
	if foreground {
		// An early transport error cannot shorten the primary's promised grace.
		<-grace.Done()
		if budget.Cause(ctx) != nil {
			return port.Incomplete
		}
	}
	if failure = recoverySignal(ctx, s, c, "SIGKILL"); failure != "" {
		return failure
	}
	exit, failure = c.Wait(ctx, s.Intent().Record().ContainerID)
	if failure == "" {
		return recordExit(s, exit)
	}
	_, resource, failure = inspectSession(ctx, s, c)
	if failure != "" {
		return failure
	}
	if resource == "running" {
		return port.Incomplete
	}
	return ""
}

func recoverySignal(ctx context.Context, s port.Session, c port.RunConnection, signal string) port.Outcome {
	_, resource, failure := inspectSession(ctx, s, c)
	if failure != "" {
		return failure
	}
	if resource != "running" {
		return ""
	}
	if failure = s.Validate(); failure != "" {
		return failure
	}
	if failure = c.Check(ctx); failure != "" {
		return failure
	}
	return c.Signal(ctx, s.Intent().Record().ContainerID, signal)
}
func recordExit(s port.Session, exit int) port.Outcome {
	if exit < 0 || exit > 255 {
		return port.Incomplete
	}
	r := s.Record()
	if r.WorkloadExit != nil {
		return ""
	}
	r.WorkloadExit = &exit
	return s.Save(r)
}
func finishSession(s port.Session) port.Outcome {
	i := s.Intent().Record()
	if (i.Schema >= 2 && i.Anchor.Stage != "absent") || !i.EgressGone() {
		return port.Refused
	}
	if !port.HexID(i.ContainerID) {
		return port.Uncertain
	}
	i.Stage = "absent"
	if failure := s.Intent().Save(i); failure != "" {
		return failure
	}
	r := s.Record()
	if r.State == "stopped" {
		return s.Sync()
	}
	r.State, r.LastProblem = "stopped", nil
	return s.Save(r)
}

// A lost acknowledgement does not authorize retrying the original signal. Fresh
// inspection and the original grace deadline decide whether escalation is safe.
func retryableSignal(f port.Outcome) bool {
	return retryableRecovery(f) || f == port.ExecutionFailed || f == port.EngineLost
}

type runGraceKey struct{}
