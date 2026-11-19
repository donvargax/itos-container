package domain

import (
	"context"
	"time"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// launchSuperseded is internal to registered launch: only a loaded record can
// establish that stop won. A transient Open failure still requires teardown.
const launchSuperseded port.Outcome = "launch-superseded"

func registrationFailure(f port.Outcome) port.Outcome {
	switch f {
	case port.CheckoutOverlap:
		return f
	case port.Refused, port.Unsafe, port.Invalid:
		return port.RegistrationOwnership
	case port.IdentityFailed, port.Internal:
		return port.RegistrationInternal
	default:
		return port.RegistrationTemporary
	}
}
func (s Runner) registeredRun(ctx context.Context, frozen approval.Durable, argv []string, streams port.Streams) port.RunResult {
	s.Registry = finiteRegistry{s.Registry}
	s.Engine = finiteEngine{s.Engine}
	startup, end := budget.Startup(ctx)
	defer end()
	state := &startupState{}
	startup = context.WithValue(startup, startupKey{}, state)
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
	// Publish no-create evidence before the public registration can be visible.
	type intended struct {
		intent  port.Intent
		failure port.Outcome
	}
	made, err := budget.Acquire(startup, "intent lock acquisition and intent publication", func(call context.Context) intended {
		i, f := s.Store.Prepare(call, frozen, token)
		return intended{i, f}
	}, func(r intended) {
		if r.intent != nil {
			r.intent.Close()
		}
	})
	i, failure := made.intent, made.failure
	if err != nil {
		return startupResult(ctx, startup, err, "intent lock acquisition and intent publication")
	}
	if failure != "" {
		result.Outcome = failure
		return result
	}
	binding := i.Binding()
	i.Close()
	if !binding.Egress.Valid() {
		result.Outcome = port.Invalid
		return result
	}
	if binding.Egress != (port.EgressGrant{}) && s.Refusals == nil {
		result.Outcome = port.Internal
		return result
	}
	type leased struct {
		lease   port.Lease
		failure port.Outcome
	}
	acquired, err := budget.Acquire(startup, "session registration", func(call context.Context) leased {
		l, f := s.Registry.Register(call, frozen, id, token)
		return leased{l, f}
	}, func(r leased) {
		if r.lease != nil {
			r.lease.Close()
		}
	})
	lease, failure := acquired.lease, acquired.failure
	if err != nil {
		return startupResult(ctx, startup, err, "session registration")
	}
	if failure != "" {
		result.Outcome = registrationFailure(failure)
		return result
	}
	defer lease.Close()
	if bound, ok := s.Registry.(port.RunRegistry); ok {
		s.Registry = bound.ForRun(frozen)
	}
	result.Reference, result.SessionID = port.Reference(frozen.Frozen.Project, token), id
	launch, finishLaunch := watchLaunchTerminal(startup, streams.Terminal)
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	s.Workspace = finiteWorkspace{s.Workspace, launch, gate}
	c, attachment, resource, failure := s.registeredLaunch(launch, id, argv, streams)
	loss := finishLaunch()
	result.ID, result.Outcome = resource, failure
	result.HomeID = state.homeID
	result.Started, result.Operation = state.started, state.operation
	if state.detectorExit != nil {
		result.Exit, result.Exited = *state.detectorExit, true
	}
	if result.Outcome != "" {
		result.MountDiagnostic = state.mountDiagnostic
	}
	if failure != port.Refused && failure != port.IsolationUnverified {
		if loss != "" {
			result.Outcome = loss
		} else if startup.Err() != nil {
			result.Outcome = startupFailure(startup)
		}
	}
	end()
	if c != nil {
		defer c.Close()
	}
	if attachment != nil {
		defer attachment.Close()
	}
	if result.Outcome == "" && !result.Exited {
		result.Exit, result.Exited, result.Outcome, result.Operation = s.waitForeground(ctx, c, attachment, streams.Terminal, state.binding, state.record, id, result.ID)
	}
	teardown, finish := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer finish()
	result.Deadline, _ = teardown.Deadline()
	teardown = context.WithValue(teardown, runGraceKey{}, result.Deadline.Add(-30*time.Second+budget.GraceDuration))
	// Keep the final second of the original total for diagnostic delivery. The
	// command must be able to report exhaustion before main exits, while an
	// unread diagnostic sink still cannot extend the thirty-second deadline.
	teardown, finishCleanup := context.WithDeadline(teardown, result.Deadline.Add(-time.Second))
	defer finishCleanup()
	if interrupted, ok := context.Cause(ctx).(port.Interrupt); ok && (result.Outcome == "" || result.Outcome == port.ExecutionFailed || result.Outcome == port.Uncertain) {
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
	if result.Signal != "" {
		signal = result.Signal
	}
	if result.Outcome == launchSuperseded {
		// Stop already won a transition. Observe its result without repeating or
		// rewriting cleanup; an unstarted workload never becomes successful.
		result.Outcome = port.Incomplete
		observed := (Recovery{Registry: s.Registry, Engine: s.Engine, Refusals: s.Refusals, Homes: s.Homes, Credentials: s.Credentials}).status(teardown, id)
		result.Cleanup = port.Incomplete
		if len(observed.Failures) == 0 && len(observed.Entries) == 1 && observed.Entries[0].Cleanup == "complete" {
			result.Cleanup = port.Complete
		}
		if result.Cleanup != port.Complete {
			result.Outcome = ""
		}
		return result
	}
	// A cancelled start request may already have made the worker live. Use
	// immediate owned teardown if the controller never acknowledged a live TTY.
	racedStart := loss != "" && !state.acknowledged
	recovery := Recovery{Registry: s.Registry, Engine: s.Engine, Refusals: s.Refusals, Homes: s.Homes, Credentials: s.Credentials, immediate: result.Started && (result.Outcome == port.MountRefused || racedStart)}
	// The output subscription is independent of termination and removal. A blocked
	// consumer must never hold the operation lock or prevent owned cleanup.
	drain := startDrain(teardown, attachment, streams.Terminal)
	if !result.Exited && attachment != nil && !recovery.immediate {
		prepared := recovery.stop(teardown, id, signal, nil, false)
		if len(prepared.Entries) == 1 && prepared.Entries[0].WorkloadExit != nil {
			result.Exit, result.Exited = *prepared.Entries[0].WorkloadExit, true
		}
		if f := recoveryCleanup(prepared, &result.RefusalLog); f != port.Complete {
			result.Cleanup = f
			result.Stream = <-drain
			if budget.Cause(teardown) != nil {
				result.Teardown = port.TeardownTimeout
			}
			return result
		}
	}
	var exit *int
	if result.Exited {
		exit = &result.Exit
	}
	cleaned := recovery.stop(teardown, id, signal, exit, true)
	result.Cleanup = recoveryCleanup(cleaned, &result.RefusalLog)
	if !result.Exited && len(cleaned.Entries) == 1 && cleaned.Entries[0].WorkloadExit != nil {
		result.Exit, result.Exited = *cleaned.Entries[0].WorkloadExit, true
	}
	result.Stream = <-drain
	if budget.Cause(teardown) != nil {
		result.Cleanup = port.Incomplete
		result.Teardown = port.TeardownTimeout
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

func (s Runner) registeredLaunch(ctx context.Context, id string, argv []string, streams port.Streams) (connection port.RunConnection, attachment port.Attachment, worker string, outcome port.Outcome) {
	entry, failure := s.Registry.Open(ctx, id, false)
	if failure != "" {
		return nil, nil, "", failure
	}
	if entry.Record().State != "preparing" {
		entry.Close()
		return nil, nil, "", launchSuperseded
	}
	intent := entry.Intent()
	if ctx.Err() != nil {
		entry.Close()
		return nil, nil, "", startupFailure(ctx)
	}
	if failure = s.Workspace.Pin(intent.Binding()); failure != "" {
		entry.Close()
		return nil, nil, "", failure
	}
	c, failure := s.Engine.OpenRun(ctx, intent.Binding())
	if failure != "" {
		s.Workspace.Close()
		entry.Close()
		return nil, nil, "", failure
	}
	defer closeAnchorWorkspace(c, s.Workspace)
	if streams.Terminal != nil {
		terminal, ok := c.(port.TerminalConnection)
		if !ok {
			entry.Close()
			return c, nil, "", port.TerminalUnavailable
		}
		if failure := terminal.SetTerminal(streams.Terminal.Initial()); failure != "" {
			entry.Close()
			return c, nil, "", failure
		}
		// A client that is already gone must not have its worker created.
		if failure := lostClientTerminal(streams.Terminal); failure != "" {
			entry.Close()
			return c, nil, "", failure
		}
	}
	if ctx.Err() != nil {
		entry.Close()
		return c, nil, "", startupFailure(ctx)
	}
	if intent.Binding().Home {
		if s.Homes == nil {
			entry.Close()
			return c, nil, "", port.HomeUnavailable
		}
		if failure = s.Homes.prepare(ctx, entry, c); failure != "" {
			entry.Close()
			return c, nil, "", failure
		}
	}
	resource, failure := createRegistered(ctx, entry, c, s.Workspace, argv, s.prepareCredentials)
	if failure == "" && intent.Binding().Home {
		failure = c.VerifyRun(ctx, resource, argv)
	}
	if failure == "" && intent.Binding().Home {
		failure = s.Homes.verify(ctx, entry, c, true)
	}
	if failure != "" {
		_ = cleanupCredentials(ctx, entry.Intent(), s.Credentials)
	}
	entry.Close()
	if failure != "" {
		return c, nil, "", failure
	}
	if ctx.Err() != nil {
		return c, nil, resource, startupFailure(ctx)
	}
	// Release the operation lock between create and start. A stop that acquires
	// it here is terminal; the fresh record below prevents a stale continuation.
	entry, failure = s.Registry.Open(ctx, id, false)
	if failure != "" {
		return c, nil, "", failure
	}
	defer entry.Close()
	defer func() {
		f := cleanupCredentials(ctx, entry.Intent(), s.Credentials)
		if outcome == "" && f != "" {
			outcome = f
		}
	}()
	if entry.Record().State != "ready" {
		return c, nil, "", launchSuperseded
	}
	intent = entry.Intent()
	if intent.Record().ContainerID != resource {
		return c, nil, "", port.Refused
	}
	if failure = startupOperation(ctx, "boundary inspection", func(call context.Context) port.Outcome { return c.VerifyRun(call, resource, argv) }); failure != "" {
		return c, nil, "", failure
	}
	// A client that hung up across the create edge must not be attached to.
	if failure := lostClientTerminal(streams.Terminal); failure != "" {
		return c, nil, "", failure
	}
	if streams.Terminal != nil && streams.Out != nil {
		// The attach stream can carry a worker's first frame before Start
		// replies. Let the client see it only once launch has settled, so
		// reacting to that frame gets normal foreground signal and exit handling.
		launched := make(chan struct{})
		streams.Out = terminalLaunchOutput{streams.Out, launched}
		defer close(launched)
	}
	var a port.Attachment
	failure = startupOperation(ctx, "attach establishment", func(call context.Context) port.Outcome {
		var f port.Outcome
		a, f = c.Attach(call, resource, streams)
		return f
	})
	if failure != "" {
		return c, nil, "", failure
	}
	if ctx.Err() != nil {
		return c, a, resource, startupFailure(ctx)
	}
	if failure = entry.ValidateLaunch(); failure != "" {
		return c, a, "", failure
	}
	if failure = validateLaunch(ctx, intent, c, s.Workspace); failure != "" {
		return c, a, "", failure
	}
	if failure = startupOperation(ctx, "boundary inspection", func(call context.Context) port.Outcome { return c.VerifyRun(call, resource, argv) }); failure != "" {
		return c, a, "", failure
	}
	if failure = verifyAnchor(ctx, intent.Record(), c); failure != "" {
		return c, a, "", failure
	}
	if failure = verifyEgressStart(ctx, intent, c); failure != "" {
		return c, a, "", failure
	}
	if intent.Binding().Home {
		if failure = s.Homes.verify(ctx, entry, c, false); failure != "" {
			return c, a, "", failure
		}
	}
	if state, ok := ctx.Value(startupKey{}).(*startupState); ok {
		state.binding, state.record = intent.Binding(), intent.Record()
	}
	// A client that hung up across the attach edge must not be started for.
	if failure := lostClientTerminal(streams.Terminal); failure != "" {
		return c, a, "", failure
	}
	if failure = startupOperation(ctx, "start acknowledgement", func(call context.Context) port.Outcome {
		if state, ok := ctx.Value(startupKey{}).(*startupState); ok {
			state.started = true
		}
		return c.Start(call, resource)
	}); failure != "" {
		if ctx.Err() != nil {
			return c, a, resource, failure
		}
		return c, a, "", failure
	}
	if ctx.Err() != nil {
		return c, a, resource, port.ExecutionFailed
	}
	if loss := lostClientTerminal(streams.Terminal); loss != "" {
		return c, a, resource, loss
	}
	if state, ok := ctx.Value(startupKey{}).(*startupState); ok {
		state.acknowledged = true
	}
	detector, failure := detectAnchor(ctx, intent, c, s.Workspace)
	r := entry.Record()
	r.MountDetector = detector
	if failure == "" {
		r.State = "running"
	}
	saved := entry.Save(r)
	if failure != "" {
		return c, a, resource, failure
	}
	if saved != "" {
		if ctx.Err() != nil {
			return c, a, resource, port.ExecutionFailed
		}
		return c, a, resource, saved
	}
	return c, a, resource, ""
}
func createRegistered(ctx context.Context, entry port.Session, c port.RunConnection, workspace port.Workspace, argv []string, credentialPreparation ...func(context.Context, port.Intent) port.Outcome) (string, port.Outcome) {
	i := entry.Intent()
	if failure := entry.ValidateLaunch(); failure != "" {
		return "", failure
	}
	if failure := validateLaunch(ctx, i, c, workspace); failure != "" {
		return "", failure
	}
	r := i.Record()
	if r.Stage == "reserved" && r.ContainerID == "" {
		if failure := prepareAnchor(ctx, i, c, workspace, entry.Record().ID); failure != "" {
			return "", failure
		}
		if failure := prepareEgress(ctx, i, c); failure != "" {
			return "", failure
		}
		if len(credentialPreparation) != 0 {
			if failure := credentialPreparation[0](ctx, i); failure != "" {
				return "", failure
			}
		} else if i.Binding().StagingKinds != (port.CredentialKinds{}) {
			return "", port.CredentialRefused
		}
		r = i.Record()
		// Once this durable write succeeds a lost create reply is uncertain.
		// Stop holds the same operation lock and cannot cancel across this edge.
		r.Stage = "intent"
		if failure := i.Save(r); failure != "" {
			return "", failure
		}
	}
	if r.Stage != "intent" || r.ContainerID != "" {
		return "", port.Refused
	}
	var id string
	failure := startupOperation(ctx, "create response", func(call context.Context) port.Outcome {
		var f port.Outcome
		id, f = c.CreateRun(call, r, argv)
		return f
	})
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
