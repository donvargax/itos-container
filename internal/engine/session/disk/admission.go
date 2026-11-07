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
	names, f := d.Names()
	if f != "" {
		return storageFailure(f, false)
	}
	known := map[string]port.AdmissionRecord{}
	for _, name := range names {
		data, f := d.Read(name)
		if f != "" {
			return storageFailure(f, false)
		}
		r, failure := s.Codec.DecodeAdmission(data)
		if failure != "" {
			return failure
		}
		index := registryKey(r.Root, c.UID)
		if name != admissionName(index, r.ID, c.UID) {
			return port.Invalid
		}
		known[index+"/"+r.ID] = r
		if !r.Complete && !r.Pending && domain.NestedCheckouts(c.Root, r.Root) {
			return port.CheckoutOverlap
		}
	}
	// Old public registrations are authority even without new metadata. Read
	// their original intent/snapshot without opening mutable checkout paths,
	// acquiring operation locks or contacting any other daemon.
	records, f := d.Sessions()
	if f != "" {
		return storageFailure(f, false)
	}
	for _, raw := range records {
		claim, knownClaim := known[raw.Index+"/"+raw.ID]
		// Registration and this scan hold the same lock. Only a pending claim
		// proves an absent public record never granted launch authority. Retain
		// active/legacy claims and refuse unsafe or unreadable records above.
		if raw.Data == nil && knownClaim && claim.Pending {
			continue
		}
		r, failure := s.Codec.DecodePublic(raw.Data)
		if failure != "" {
			return failure
		}
		if r.ID != raw.ID {
			return port.Invalid
		}
		if knownClaim {
			if claim.Intent != r.Intent || (claim.Complete && (r.State != "stopped" || r.LastProblem != nil)) {
				return port.Invalid
			}
			if !claim.Complete && domain.NestedCheckouts(c.Root, claim.Root) {
				return port.CheckoutOverlap
			}
			continue
		}
		i, failure := s.Store.transaction(ctx, r.Intent, false)
		if failure != "" {
			return failure
		}
		root := i.Binding().Checkout.Root
		if registryKey(root, c.UID) != raw.Index {
			return port.Invalid
		}
		// No certificate means no completion evidence established by admission.
		// Legacy stop can durably re-confirm cleanup and publish that certificate.
		if domain.NestedCheckouts(c.Root, root) {
			return port.CheckoutOverlap
		}
	}
	if !d.Unchanged() {
		return port.Unsafe
	}
	return ""
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
