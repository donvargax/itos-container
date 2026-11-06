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
}

func (s Runner) Run(ctx context.Context, frozen approval.Durable, argv []string, streams port.Streams) port.RunResult {
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
		result.Exit, result.Outcome = connection.Wait(ctx, result.ID)
		result.Exited = result.Outcome == ""
		drain, end := context.WithTimeout(ctx, 10*time.Second)
		if failure := attachment.Drain(drain); failure != "" && result.Outcome == "" {
			result.Outcome = failure
		}
		end()
	}
	if result.Reference != "" {
		result.Cleanup = s.Cleanup(context.WithoutCancel(ctx), result.Reference).Outcome
	}
	return result
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
	intent, failure := s.Store.Open(ctx, reference)
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
