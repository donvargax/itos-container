package domain

import (
	"context"
	"time"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

// Runner preserves the unstarted-only Service contract while adding an explicit
// foreground lifecycle. The durable intent contains no command or stream data.
type Runner struct {
	Store     port.Store
	Engine    port.RunEngine
	Entropy   port.Entropy
	Workspace port.Workspace
	Registry  port.Registry
}

func (s Runner) Run(ctx context.Context, frozen approval.Durable, argv []string, streams port.Streams) port.RunResult {
	if s.Registry != nil {
		return s.registeredRun(ctx, frozen, argv, streams)
	}
	startup, cancel := context.WithTimeout(ctx, 120*time.Second)
	result, connection, attachment := s.launch(startup, frozen, argv, streams)
	cancel()
	if connection != nil {
		defer connection.Close()
	}
	if attachment != nil {
		defer attachment.Close()
	}
	if result.Outcome == "" {
		// Waiting for the workload is not a startup operation or a lifetime limit.
		result.Exit, result.Exited, result.Outcome = s.waitRun(ctx, connection, attachment, result.Reference, result.ID)
	}
	if result.Reference != "" {
		teardown, end := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		if interrupted, ok := context.Cause(ctx).(port.Interrupt); ok && !result.Exited {
			result.Exit, result.Exited = s.interrupt(teardown, result.Reference, interrupted.Signal)
			// Launch or wait already supplied the failure. Interruption must not
			// erase a more specific refusal while recovering the owned worker.
		}
		if result.Exited && attachment != nil {
			// Exit can precede the last output, including an interrupted worker's
			// diagnostics. Keep draining independent of the host signal, but inside
			// the shared teardown budget and before cleanup closes the connection.
			drain, stop := context.WithTimeout(teardown, 10*time.Second)
			if failure := attachment.Drain(drain); failure != "" {
				result.Outcome = failure
			}
			stop()
		}
		result.Cleanup = s.Cleanup(teardown, result.Reference).Outcome
		end()
	}
	return result
}

func (s Runner) waitRun(ctx context.Context, connection port.RunConnection, attachment port.Attachment, reference, id string) (int, bool, port.Outcome) {
	wait, stop := context.WithCancel(ctx)
	monitored := make(chan port.Outcome, 1)
	go func() {
		var failure port.Outcome
		select {
		case failure = <-attachment.Failures():
			stop()
		case <-wait.Done():
		}
		monitored <- failure
	}()
	exit, outcome := connection.Wait(wait, id)
	stop()
	failure := <-monitored
	if outcome == "" || failure != port.StdinClosed || ctx.Err() != nil {
		return exit, outcome == "", outcome
	}
	// A worker may close stdin on exit while input forwarding is still pending.
	// Confirm that the exact worker exited before reacquiring its wait response;
	// a running or unverifiable worker still requires failure and owned cleanup.
	probe, end := context.WithTimeout(ctx, 10*time.Second)
	defer end()
	// The launch connection retains the startup deadline. Reopen protected
	// authority and the original daemon within this one reconciliation budget,
	// independently of elapsed workload time or mutable checkout identity.
	var intent port.Intent
	failure = ""
	if s.Registry != nil {
		entry, f := s.Registry.Open(probe, reference, true)
		if f != "" {
			return 0, false, port.ExecutionFailed
		}
		defer entry.Close()
		if entry.Validate() != "" {
			return 0, false, port.ExecutionFailed
		}
		intent = entry.Intent()
		if r := entry.Record(); r.State == "stopped" {
			owned := intent.Record()
			if owned.Stage != "absent" || owned.ContainerID != id {
				return 0, false, port.ExecutionFailed
			}
			// Stop can finish removal without ever observing a workload exit.
			// Adopt completion without inventing exit zero or a wait failure.
			if r.WorkloadExit == nil {
				return 0, false, ""
			}
			return *r.WorkloadExit, true, ""
		}
	} else {
		intent, failure = s.Store.OpenCleanup(probe, reference)
	}
	if failure != "" {
		return 0, false, port.ExecutionFailed
	}
	defer intent.Close()
	fresh, failure := s.Engine.OpenRun(probe, intent.Binding())
	if failure != "" {
		return 0, false, port.ExecutionFailed
	}
	defer fresh.Close()
	record := intent.Record()
	if record.Stage != "owned" || record.ContainerID != id || intent.Validate() != "" || fresh.Check(probe) != "" {
		return 0, false, port.ExecutionFailed
	}
	o, status := fresh.Inspect(probe, id)
	if status != port.Observed || o.ID != id || !owns(record, o) || !o.StatePresent || o.Running || o.Status != "exited" {
		return 0, false, port.ExecutionFailed
	}
	exit, outcome = fresh.Wait(probe, id)
	return exit, outcome == "", outcome
}

// interrupt reacquires protected authority and a fresh engine connection. The
// launch connection's deadline cannot constrain teardown or authorize signals.
func (s Runner) interrupt(ctx context.Context, reference, signal string) (int, bool) {
	intent, failure := s.Store.OpenCleanup(ctx, reference)
	if failure != "" {
		return 0, false
	}
	defer intent.Close()
	c, failure := s.Engine.OpenRun(ctx, intent.Binding())
	if failure != "" {
		return 0, false
	}
	defer c.Close()
	record := intent.Record()
	if !port.HexID(record.ContainerID) || record.Stage != "owned" {
		return 0, false
	}
	if signalOwned(ctx, intent, c, signal) != "" {
		return 0, false
	}
	grace, end := context.WithTimeout(ctx, 10*time.Second)
	exit, outcome := c.Wait(grace, record.ContainerID)
	end()
	if outcome == "" {
		return exit, true
	}
	if signalOwned(ctx, intent, c, "SIGKILL") != "" {
		return 0, false
	}
	exit, outcome = c.Wait(ctx, record.ContainerID)
	return exit, outcome == ""
}

func signalOwned(ctx context.Context, intent port.Intent, c port.RunConnection, signal string) port.Outcome {
	if failure := intent.Validate(); failure != "" {
		return failure
	}
	if failure := c.Check(ctx); failure != "" {
		return failure
	}
	r := intent.Record()
	o, status := c.Inspect(ctx, r.ContainerID)
	if status != port.Observed || !o.StatePresent || o.ID != r.ContainerID || !owns(r, o) {
		return port.Refused
	}
	if !o.Running {
		return "" // Wait still observes the actual exit status.
	}
	if failure := intent.Save(r); failure != "" {
		return failure
	}
	if failure := intent.Validate(); failure != "" {
		return failure
	}
	if failure := c.Check(ctx); failure != "" {
		return failure
	}
	return c.Signal(ctx, r.ContainerID, signal)
}

func (s Runner) launch(ctx context.Context, frozen approval.Durable, argv []string, streams port.Streams) (port.RunResult, port.RunConnection, port.Attachment) {
	result := port.RunResult{}
	token, failure := s.Entropy.Token()
	if failure != "" || !port.HexID(token) {
		result.Outcome = port.IdentityFailed
		return result, nil, nil
	}
	intent, failure := s.Store.New(ctx, frozen, token)
	if failure != "" {
		result.Outcome = failure
		return result, nil, nil
	}
	defer intent.Close()
	result.Reference = port.Reference(frozen.Frozen.Project, token)
	if failure = s.Workspace.Pin(intent.Binding()); failure != "" {
		result.Outcome = failure
		return result, nil, nil
	}
	defer s.Workspace.Close()
	connection, failure := s.Engine.OpenRun(ctx, intent.Binding())
	if failure != "" {
		result.Outcome = failure
		return result, nil, nil
	}
	result.Outcome = validateLaunch(ctx, intent, connection, s.Workspace)
	if result.Outcome != "" {
		return result, connection, nil
	}
	record := intent.Record()
	id, failure := connection.CreateRun(ctx, record, argv)
	if failure != "" {
		result.Outcome = failure
		return result, connection, nil
	}
	if !port.HexID(id) {
		result.Outcome = port.Uncertain
		return result, connection, nil
	}
	observed, status := connection.Inspect(ctx, id)
	if status != port.Observed || !observed.StatePresent {
		result.Outcome = port.IsolationInspection
		return result, connection, nil
	}
	if observed.ID != id || !owns(record, observed) {
		result.Outcome = port.Refused
		return result, connection, nil
	}
	record.Stage, record.ContainerID = "owned", id
	if failure = intent.Save(record); failure != "" {
		result.Outcome = port.OwnershipWrite
		return result, connection, nil
	}
	if failure = connection.VerifyRun(ctx, id, argv); failure != "" {
		result.Outcome = failure
		return result, connection, nil
	}
	attachment, failure := connection.Attach(ctx, id, streams)
	if failure != "" {
		result.Outcome = failure
		return result, connection, nil
	}
	if failure = validateLaunch(ctx, intent, connection, s.Workspace); failure != "" {
		result.Outcome = failure
		return result, connection, attachment
	}
	// Re-read the actual boundary after attachment, immediately before start.
	if failure = connection.VerifyRun(ctx, id, argv); failure != "" {
		result.Outcome = failure
		return result, connection, attachment
	}
	result.Outcome = connection.Start(ctx, id)
	if result.Outcome == "" {
		result.ID = id
	}
	return result, connection, attachment
}

func validateLaunch(ctx context.Context, intent port.Intent, connection port.RunConnection, workspace port.Workspace) port.Outcome {
	if failure := intent.Validate(); failure != "" {
		return failure
	}
	if !workspace.Unchanged() {
		return port.Refused
	}
	return connection.Check(ctx)
}

// Cleanup reacquires the original record/snapshot with a fresh teardown budget;
// neither current approval nor an expired startup context can authorize removal.
func (s Runner) Cleanup(ctx context.Context, reference string) port.Result {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result := port.Result{Reference: reference}
	intent, failure := s.Store.OpenCleanup(ctx, reference)
	if failure != "" {
		result.Outcome = failure
		return result
	}
	defer intent.Close()
	connection, failure := s.Engine.OpenRun(ctx, intent.Binding())
	if failure != "" {
		result.Outcome = failure
		return result
	}
	defer connection.Close()
	result.Outcome = cleanupRun(ctx, intent, connection)
	return result
}

func cleanupRun(ctx context.Context, intent port.Intent, connection port.RunConnection) port.Outcome {
	if failure := intent.Validate(); failure != "" {
		return failure
	}
	if failure := connection.Check(ctx); failure != "" {
		return failure
	}
	record := intent.Record()
	id := record.ContainerID
	if id == "" {
		observed, status := connection.Inspect(ctx, record.Name)
		// As in unstarted cleanup, a name-only 404 cannot fence a delayed create.
		// Preserve the intent until a concrete owned ID can establish absence.
		if status != port.Observed {
			return port.Uncertain
		}
		if !owns(record, observed) {
			return port.Refused
		}
		id = observed.ID
	}
	observed, status := connection.Inspect(ctx, id)
	if status == port.NotFound && record.ContainerID != "" {
		return complete(intent, record, id)
	}
	if status == port.Changed {
		return port.Refused
	}
	if status != port.Observed || !observed.StatePresent {
		return port.Incomplete
	}
	if observed.ID != id || !owns(record, observed) || record.Stage == "absent" {
		return port.Refused
	}
	record.ContainerID, record.Stage = id, "owned"
	if failure := intent.Save(record); failure != "" {
		return port.OwnershipWrite
	}
	if failure := intent.Validate(); failure != "" {
		return failure
	}
	if failure := connection.Check(ctx); failure != "" {
		return failure
	}
	connection.RemoveRun(ctx, id)
	_, status = connection.Inspect(ctx, id)
	if status == port.Changed {
		return port.Refused
	}
	if status != port.NotFound {
		return port.Incomplete
	}
	return complete(intent, record, id)
}

func owns(record port.Record, observed port.Observation) bool {
	return port.HexID(observed.ID) && observed.Name == record.Name && observed.ImageID == record.ImageID &&
		observed.Token == record.NameToken() && observed.Project == record.Project && observed.Fingerprint == record.Fingerprint
}
