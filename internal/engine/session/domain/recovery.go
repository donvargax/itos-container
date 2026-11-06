package domain

import (
	"context"
	"slices"
	"time"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// Recovery observes and removes registered sessions; it has no launch path.
type Recovery struct {
	Registry port.Registry
	Engine   port.RunEngine
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
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
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
	if failure == "" {
		connection, f := s.Engine.OpenRun(ctx, entry.Intent().Binding())
		failure = f
		if failure == "" {
			defer connection.Close()
			var observation port.Observation
			observation, e.Resource, failure = inspectSession(ctx, entry, connection)
			_ = observation
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
			if e.Resource == "absent" && entry.Intent().Record().Stage == "absent" {
				e.Cleanup = "complete"
			} else {
				failure = port.Incomplete
			}
		}
	}
	if failure != "" {
		e.State, e.Resource, e.Cleanup = "failed", "unknown", "unknown"
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
	return port.RecoveryEntry{ID: r.ID, Project: r.Project, Fingerprint: s.Intent().Record().Fingerprint, State: r.State, Resource: "unknown", Controller: controller, Cleanup: "unknown", WorkloadExit: r.WorkloadExit}, failure
}
func finishEntry(e *port.RecoveryEntry) {
	e.StopCandidate = e.Controller == "absent" && e.Cleanup != "complete"
	if e.Cleanup != "complete" {
		e.Retry = []string{"itos", "container", "stop", e.ID}
	} else {
		e.Retry = nil
	}
}

func inspectSession(ctx context.Context, s port.Session, c port.RunConnection) (port.Observation, string, port.Outcome) {
	// A wait or prior engine call may have exhausted the total. Expired
	// storage handles cannot distinguish changed ownership from lost time.
	if ctx.Err() != nil {
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
	if o.ID != id || !owns(r, o) || r.Stage == "absent" {
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
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return s.stop(ctx, id, "SIGTERM", nil, true)
}
func (s Recovery) stop(ctx context.Context, id, signal string, exit *int, remove bool) port.RecoveryResult {
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
	connection, failure := s.Engine.OpenRun(ctx, entry.Intent().Binding())
	if failure == "" {
		defer connection.Close()
		failure = stopSession(ctx, entry, connection, signal, exit, remove)
	}
	if failure == "" {
		e.State, e.Resource, e.Cleanup = "stopped", "absent", "complete"
		if !remove && entry.Record().State != "stopped" {
			e.State, e.Resource, e.Cleanup = entry.Record().State, "unknown", "pending"
		}
	} else {
		e.State, e.Resource, e.Cleanup = "failed", "unknown", "unknown"
		// Persist uncertainty only for safely held, incomplete sessions. Never
		// replace a completed tombstone or overwrite substituted authority.
		r := entry.Record()
		if ctx.Err() == nil && r.State != "stopped" && retryableRecovery(failure) && entry.Validate() == "" {
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
	if failure != "" {
		r.Failures = append(r.Failures, port.RecoveryFailure{Session: id, Outcome: failure})
	}
	return r
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
	if ctx.Err() != nil {
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
		if r.WorkloadExit == nil && exit != nil {
			r.WorkloadExit = exit
		}
		if failure := s.Save(r); failure != "" {
			return failure
		}
	}
	if failure != "" {
		return failure
	}
	if resource == "absent" {
		return finishSession(s)
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
		return finishSession(s)
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
	return finishSession(s)
}

func retryableRecovery(f port.Outcome) bool {
	switch f {
	case port.Incomplete, port.Uncertain, port.IntentWrite, port.OwnershipWrite:
		return true
	default:
		return false
	}
}
func terminateSession(ctx context.Context, s port.Session, c port.RunConnection, signal string) port.Outcome {
	if failure := recoverySignal(ctx, s, c, signal); failure != "" {
		return failure
	}
	grace, end := context.WithTimeout(ctx, 10*time.Second)
	exit, failure := c.Wait(grace, s.Intent().Record().ContainerID)
	end()
	if failure == "" {
		return recordExit(s, exit)
	}
	_, resource, failure := inspectSession(ctx, s, c)
	if failure != "" {
		return failure
	}
	if resource != "running" {
		return ""
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
