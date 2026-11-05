// Package domain owns the intent/create/inspect/remove state machine.
package domain

import (
	"context"
	"time"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

type Service struct {
	Store   port.Store
	Engine  port.Engine
	Entropy port.Entropy
}

func (s Service) Create(ctx context.Context, frozen approval.Durable) port.Result {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	token, failure := s.Entropy.Token()
	if failure != "" || !port.HexID(token) {
		return port.Result{Outcome: port.IdentityFailed}
	}
	result := port.Result{Reference: port.Reference(frozen.Frozen.Project, token)}
	intent, failure := s.Store.New(ctx, frozen, token)
	if failure != "" {
		result.Outcome = failure
		return result
	}
	defer intent.Close()
	connection, failure := s.Engine.Open(ctx, intent.Binding())
	if failure != "" {
		result.Outcome = failure
		return result
	}
	defer connection.Close()
	if failure = intent.Validate(); failure != "" {
		result.Outcome = failure
		return result
	}
	if failure = connection.Check(ctx); failure != "" {
		result.Outcome = failure
		return result
	}
	record := intent.Record()
	id, failure := connection.Create(ctx, record)
	if failure != "" || !port.HexID(id) {
		result.Outcome = port.Uncertain
		return result
	}
	observed, status := connection.Inspect(ctx, id)
	if status != port.Observed || !observed.StatePresent {
		result.Outcome = port.Uncertain
		return result
	}
	if !matches(record, observed) || observed.ID != id {
		result.Outcome = port.Refused
		return result
	}
	if failure = intent.Validate(); failure != "" {
		result.Outcome = failure
		return result
	}
	record.Stage, record.ContainerID = "owned", id
	if failure = intent.Save(record); failure != "" {
		result.Outcome = port.OwnershipWrite
		return result
	}
	result.Outcome, result.ID = port.Owned, id
	return result
}

func (s Service) Cleanup(ctx context.Context, reference string) port.Result {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result := port.Result{Outcome: port.Invalid}
	if _, _, ok := port.ParseReference(reference); !ok {
		return result
	}
	result.Reference = reference
	intent, failure := s.Store.Open(ctx, reference)
	if failure != "" {
		result.Outcome = failure
		return result
	}
	defer intent.Close()
	connection, failure := s.Engine.Open(ctx, intent.Binding())
	if failure != "" {
		result.Outcome = failure
		return result
	}
	defer connection.Close()
	result.Outcome = s.cleanup(ctx, intent, connection)
	return result
}
func (s Service) cleanup(ctx context.Context, intent port.Intent, connection port.Connection) port.Outcome {
	if failure := intent.Validate(); failure != "" {
		return failure
	}
	if failure := connection.Check(ctx); failure != "" {
		return failure
	}
	record := intent.Record()
	id := record.ContainerID
	if id == "" {
		byName, status := connection.Inspect(ctx, record.Name)
		if status == port.Changed {
			return port.Refused
		}
		// A missing random name cannot fence a delayed create request.
		if status != port.Observed || !byName.StatePresent {
			return port.Uncertain
		}
		if !matches(record, byName) {
			return port.Refused
		}
		id = byName.ID
	}
	observed, status := connection.Inspect(ctx, id)
	if status == port.Changed {
		return port.Refused
	}
	if status == port.Unavailable || (status == port.Observed && !observed.StatePresent) {
		return port.Incomplete
	}
	if status == port.NotFound {
		if record.ContainerID == "" {
			return port.Uncertain
		}
		return complete(intent, record, id)
	}
	if observed.ID != id || !matches(record, observed) || record.Stage == "absent" {
		return port.Refused
	}
	// Revalidate both authorities at the destructive boundary, even after a
	// matching inspect. Labels and the create response never authorize deletion.
	if failure := intent.Validate(); failure != "" {
		return failure
	}
	if failure := connection.Check(ctx); failure != "" {
		return failure
	}
	connection.Remove(ctx, id)
	_, status = connection.Inspect(ctx, id)
	if status == port.Changed {
		return port.Refused
	}
	if status != port.NotFound {
		return port.Incomplete
	}
	return complete(intent, record, id)
}
func complete(intent port.Intent, record port.Record, id string) port.Outcome {
	record.ContainerID, record.Stage = id, "absent"
	if failure := intent.Save(record); failure != "" {
		return port.CompletionWrite
	}
	return port.Complete
}
func matches(record port.Record, observed port.Observation) bool {
	return port.HexID(observed.ID) && observed.Name == record.Name && observed.ImageID == record.ImageID &&
		observed.Token == record.Name[len("itos-container-"):] && observed.Project == record.Project && observed.Fingerprint == record.Fingerprint &&
		observed.StatePresent && observed.Status == "created" && !observed.Running && observed.PID == 0 && !observed.Started
}
