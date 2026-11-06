package domain

import (
	"context"
	"time"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// launchSuperseded is internal to registered launch: only a loaded record can
// establish that stop won. A transient Open failure still requires teardown.
const launchSuperseded port.Outcome = "launch-superseded"

func registrationFailure(f port.Outcome) port.Outcome {
	switch f {
	case port.Refused, port.Unsafe, port.Invalid:
		return port.RegistrationOwnership
	case port.IdentityFailed, port.Internal:
		return port.RegistrationInternal
	default:
		return port.RegistrationTemporary
	}
}
func (s Runner) registeredRun(ctx context.Context, frozen approval.Durable, argv []string, streams port.Streams) port.RunResult {
	startup, end := context.WithTimeout(ctx, 120*time.Second)
	defer end()
	result := port.RunResult{}
	id, failure := s.Entropy.Token()
	if failure != "" || !port.HexID(id) {
		result.Outcome = port.RegistrationInternal
		return result
	}
	token, failure := s.Entropy.Token()
	if failure != "" || !port.HexID(token) {
		result.Outcome = port.RegistrationInternal
		return result
	}
	lease, failure := s.Registry.Register(startup, frozen, id, token)
	if failure != "" {
		result.Outcome = registrationFailure(failure)
		return result
	}
	defer lease.Close()
	i, failure := s.Store.New(startup, frozen, token)
	if failure != "" {
		result.Outcome = failure
		return result
	}
	i.Close()
	if bound, ok := s.Registry.(port.RunRegistry); ok {
		s.Registry = bound.ForRun(frozen)
	}
	result.Reference, result.SessionID = port.Reference(frozen.Frozen.Project, token), id
	c, attachment, resource, failure := s.registeredLaunch(startup, id, argv, streams)
	result.ID, result.Outcome = resource, failure
	result.Started = attachment != nil
	end()
	if c != nil {
		defer c.Close()
	}
	if attachment != nil {
		defer attachment.Close()
	}
	if result.Outcome == "" {
		result.Exit, result.Exited, result.Outcome = s.waitRun(ctx, c, attachment, id, result.ID)
	}
	teardown, finish := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer finish()
	result.Deadline, _ = teardown.Deadline()
	teardown = context.WithValue(teardown, runGraceKey{}, result.Deadline.Add(-20*time.Second))
	if interrupted, ok := context.Cause(ctx).(port.Interrupt); ok && (result.Outcome == "" || result.Outcome == port.ExecutionFailed) {
		result.Signal = interrupted.Signal
		result.Outcome = ""
	}
	if result.Outcome == waitUnreconciled {
		// A concurrent stop can outlast the short stdin probe. Wait for its
		// lock within this teardown total and accept only its exact completion,
		// before our own cleanup can change an incomplete session.
		result.Outcome = port.ExecutionFailed
		if entry, f := s.Registry.Open(teardown, id, true); f == "" {
			result.Exit, result.Exited, result.Outcome = stoppedRun(entry, result.ID)
			entry.Close()
		}
	}
	signal := "SIGTERM"
	if interrupted, ok := context.Cause(ctx).(port.Interrupt); ok {
		signal = interrupted.Signal
	}
	if result.Outcome == launchSuperseded {
		// Stop already won a transition. Observe its result without repeating or
		// rewriting cleanup; an unstarted workload never becomes successful.
		result.Outcome = port.Incomplete
		observed := (Recovery{Registry: s.Registry, Engine: s.Engine}).status(teardown, id)
		result.Cleanup = port.Incomplete
		if len(observed.Failures) == 0 && len(observed.Entries) == 1 && observed.Entries[0].Cleanup == "complete" {
			result.Cleanup = port.Complete
		}
		if result.Cleanup != port.Complete {
			result.Outcome = ""
		}
		return result
	}
	recovery := Recovery{Registry: s.Registry, Engine: s.Engine}
	// The output subscription is independent of termination and removal. A blocked
	// consumer must never hold the operation lock or prevent owned cleanup.
	drain := startDrain(teardown, attachment)
	if !result.Exited && attachment != nil {
		prepared := recovery.stop(teardown, id, signal, nil, false)
		if len(prepared.Entries) == 1 && prepared.Entries[0].WorkloadExit != nil {
			result.Exit, result.Exited = *prepared.Entries[0].WorkloadExit, true
		}
		if len(prepared.Failures) != 0 {
			result.Cleanup = prepared.Failures[0].Outcome
			result.Stream = <-drain
			if teardown.Err() != nil {
				result.Outcome = port.TeardownTimeout
			}
			return result
		}
	}
	var exit *int
	if result.Exited {
		exit = &result.Exit
	}
	cleaned := recovery.stop(teardown, id, signal, exit, true)
	result.Cleanup = port.Complete
	if len(cleaned.Failures) != 0 {
		result.Cleanup = cleaned.Failures[0].Outcome
	}
	if !result.Exited && len(cleaned.Entries) == 1 && cleaned.Entries[0].WorkloadExit != nil {
		result.Exit, result.Exited = *cleaned.Entries[0].WorkloadExit, true
	}
	result.Stream = <-drain
	if teardown.Err() != nil {
		result.Cleanup = port.Incomplete
		result.Outcome = port.TeardownTimeout
	}
	if result.Stream == port.StreamTimeout {
		// Preserve the safe typed cause even when the diagnostic pipe is unread.
		if entry, f := s.Registry.Open(teardown, id, false); f == "" {
			record := entry.Record()
			problem := port.StreamTimeout
			record.LastProblem = &problem
			if f := entry.Save(record); f != "" {
				result.Cleanup = f
			}
			entry.Close()
		}
	}

	return result
}

func (s Runner) registeredLaunch(ctx context.Context, id string, argv []string, streams port.Streams) (port.RunConnection, port.Attachment, string, port.Outcome) {
	entry, failure := s.Registry.Open(ctx, id, false)
	if failure != "" {
		return nil, nil, "", failure
	}
	if entry.Record().State != "preparing" {
		entry.Close()
		return nil, nil, "", launchSuperseded
	}
	intent := entry.Intent()
	if failure = s.Workspace.Pin(intent.Binding()); failure != "" {
		entry.Close()
		return nil, nil, "", failure
	}
	defer s.Workspace.Close()
	c, failure := s.Engine.OpenRun(ctx, intent.Binding())
	if failure != "" {
		entry.Close()
		return nil, nil, "", failure
	}
	resource, failure := createRegistered(ctx, entry, c, s.Workspace, argv)
	entry.Close()
	if failure != "" {
		return c, nil, "", failure
	}
	// Release the operation lock between create and start. A stop that acquires
	// it here is terminal; the fresh record below prevents a stale continuation.
	entry, failure = s.Registry.Open(ctx, id, false)
	if failure != "" {
		return c, nil, "", failure
	}
	defer entry.Close()
	if entry.Record().State != "ready" {
		return c, nil, "", launchSuperseded
	}
	intent = entry.Intent()
	if intent.Record().ContainerID != resource {
		return c, nil, "", port.Refused
	}
	if failure = c.VerifyRun(ctx, resource, argv); failure != "" {
		return c, nil, "", failure
	}
	a, failure := c.Attach(ctx, resource, streams)
	if failure != "" {
		return c, nil, "", failure
	}
	if failure = entry.ValidateLaunch(); failure != "" {
		return c, a, "", failure
	}
	if failure = validateLaunch(ctx, intent, c, s.Workspace); failure != "" {
		return c, a, "", failure
	}
	if failure = c.VerifyRun(ctx, resource, argv); failure != "" {
		return c, a, "", failure
	}
	if failure = c.Start(ctx, resource); failure != "" {
		return c, a, "", failure
	}
	if ctx.Err() != nil {
		return c, a, resource, port.ExecutionFailed
	}
	r := entry.Record()
	r.State = "running"
	if failure = entry.Save(r); failure != "" {
		if ctx.Err() != nil {
			return c, a, resource, port.ExecutionFailed
		}
		return c, a, resource, failure
	}
	return c, a, resource, ""
}
func createRegistered(ctx context.Context, entry port.Session, c port.RunConnection, workspace port.Workspace, argv []string) (string, port.Outcome) {
	i := entry.Intent()
	if failure := entry.ValidateLaunch(); failure != "" {
		return "", failure
	}
	if failure := validateLaunch(ctx, i, c, workspace); failure != "" {
		return "", failure
	}
	r := i.Record()
	if r.Stage != "intent" || r.ContainerID != "" {
		return "", port.Refused
	}
	id, failure := c.CreateRun(ctx, r, argv)
	if failure != "" {
		return "", failure
	}
	if !port.HexID(id) {
		return "", port.Uncertain
	}
	o, status := c.Inspect(ctx, id)
	if status != port.Observed || !o.StatePresent {
		return "", port.IsolationInspection
	}
	if o.ID != id || !owns(r, o) {
		return "", port.Refused
	}
	r.Stage, r.ContainerID = "owned", id
	if failure = i.Save(r); failure != "" {
		return "", port.OwnershipWrite
	}
	p := entry.Record()
	p.State = "ready"
	if failure = entry.Save(p); failure != "" {
		return "", failure
	}
	return id, ""
}
