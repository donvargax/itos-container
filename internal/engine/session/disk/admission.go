package disk

import (
	"context"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/session/domain"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func admissionName(index, id, uid string) string { return registryKey(index+"/"+id, uid) + ".json" }

func (s Registry) reserve(ctx context.Context, c approval.Candidate, r port.PublicRecord) (approval.AdmissionDirectory, port.Outcome) {
	d, f := s.Storage.Admission(ctx, c)
	if f != "" {
		return nil, storageFailure(f, true)
	}
	success := false
	defer func() {
		if !success {
			d.Close()
		}
	}()
	if failure := s.checkAdmission(ctx, c, d); failure != "" {
		return nil, failure
	}
	name := admissionName(c.Project, r.ID, c.UID)
	previous, f := d.Read(name)
	if f != "" {
		return nil, storageFailure(f, false)
	}
	if previous != nil {
		return nil, port.Refused
	}
	record := port.AdmissionRecord{Schema: 1, Root: c.Root, ID: r.ID, Intent: r.Intent, Pending: true}
	if f = d.Publish(name, nil, s.Codec.EncodeAdmission(record)); f != "" {
		return nil, storageFailure(f, true)
	}
	success = true
	return d, ""
}

func (s Registry) checkAdmission(ctx context.Context, c approval.Candidate, d approval.AdmissionDirectory) port.Outcome {
	roots, failure := s.reservationRoots(ctx, c, d)
	if failure != "" {
		return storageFailure(failure, false)
	}
	for _, root := range roots {
		if domain.NestedCheckouts(c.Root, root) {
			return port.CheckoutOverlap
		}
	}
	return ""
}

// reservationRoots is shared by admission and control-state separation. Pending
// claims with no public record prove that launch authority was never granted;
// every other unresolved claim survives controller loss and uncertain cleanup.
func (s Registry) reservationRoots(ctx context.Context, c approval.Candidate, d approval.AdmissionDirectory) ([]string, approval.Failure) {
	names, f := d.Names()
	if f != "" {
		return nil, f
	}
	known := map[string]port.AdmissionRecord{}
	var roots []string
	if len(names) > 0 {
		roots = []string{}
	}
	for _, name := range names {
		data, f := d.Read(name)
		if f != "" {
			return nil, f
		}
		r, failure := s.Codec.DecodeAdmission(data)
		if failure != "" {
			return nil, "state.invalid"
		}
		index := registryKey(r.Root, c.UID)
		if name != admissionName(index, r.ID, c.UID) {
			return nil, "state.invalid"
		}
		known[index+"/"+r.ID] = r
		if !r.Complete && !r.Pending {
			roots = append(roots, r.Root)
		}
	}
	records, f := d.Sessions()
	if f != "" {
		return nil, f
	}
	if len(records) > 0 && roots == nil {
		roots = []string{}
	}
	for _, raw := range records {
		claim, knownClaim := known[raw.Index+"/"+raw.ID]
		if raw.Data == nil && knownClaim && claim.Pending {
			continue
		}
		r, failure := s.Codec.DecodePublic(raw.Data)
		if failure != "" || r.ID != raw.ID {
			return nil, "state.invalid"
		}
		if knownClaim {
			// Final output may time out after durable owned cleanup. That
			// diagnostic does not revoke completion or reopen the session.
			if claim.Intent != r.Intent || (claim.Complete && (r.State != "stopped" || (r.LastProblem != nil && *r.LastProblem != port.StreamTimeout))) {
				return nil, "state.invalid"
			}
			if !claim.Complete {
				roots = append(roots, claim.Root)
			}
			continue
		}
		i, failure := s.Store.transaction(ctx, r.Intent, false)
		if failure != "" {
			switch failure {
			case port.Unsafe, port.Refused:
				return nil, "state.unsafe"
			case port.Invalid:
				return nil, "state.invalid"
			default:
				return nil, "state.unavailable"
			}
		}
		root := i.Binding().Checkout.Root
		if registryKey(root, c.UID) != raw.Index {
			return nil, "state.invalid"
		}
		roots = append(roots, root)
	}
	if !d.Unchanged() {
		return nil, "state.path"
	}
	return roots, ""
}

// Reservations reads all projects and daemons through the shared protected
// location port. It never opens worker-controlled checkouts or operation locks.
type Reservations struct {
	Locations approval.Locations
	Store     Store
	Codec     port.PublicCodec
}

func (s Reservations) Roots(ctx context.Context, c approval.Candidate, path string) ([]string, approval.Failure) {
	d, f := s.Locations.Inspect(ctx, c, path)
	if f != "" {
		return nil, f
	}
	defer d.Close()
	store := s.Store
	store.Storage = s.Locations.At(path)
	registry := Registry{Store: store, Codec: s.Codec}
	return registry.reservationRoots(ctx, c, d)
}

func (s *publicSession) completeAdmission() port.Outcome {
	if s.record.State != "stopped" {
		return ""
	}
	i := s.intent.Record()
	if !i.Completed() {
		// Public progress alone never releases an unresolved reservation.
		return ""
	}
	b := s.intent.Binding()
	c := approval.Candidate{UID: b.UID, GID: b.GID, Root: b.Checkout.Root, Project: registryKey(b.Checkout.Root, b.UID)}
	d, f := s.registry.Storage.Admission(s.intent.ctx, c)
	if f != "" {
		return storageFailure(f, true)
	}
	defer d.Close()
	if failure := s.Validate(); failure != "" {
		return failure
	}
	name := admissionName(c.Project, s.record.ID, c.UID)
	previous, f := d.Read(name)
	if f != "" {
		return storageFailure(f, false)
	}
	r := port.AdmissionRecord{Schema: 1, Root: c.Root, ID: s.record.ID, Intent: s.record.Intent}
	completed := false
	if previous != nil {
		old, failure := s.registry.Codec.DecodeAdmission(previous)
		if failure != "" {
			return failure
		}
		completed = old.Complete
		old.Complete = false
		old.Pending = false
		if old != r {
			return port.Refused
		}
	}
	r.Complete = true
	if completed {
		f = d.Sync(name, previous)
	} else {
		f = d.Publish(name, previous, s.registry.Codec.EncodeAdmission(r))
	}
	if f != "" {
		return storageFailure(f, true)
	}
	return ""
}
