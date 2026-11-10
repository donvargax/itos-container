// Package disk binds session records to protected storage and original snapshots.
package disk

import (
	"bytes"
	"context"
	budget "github.com/donvargax/itos-container/internal/engine/budget/domain"
	"strconv"

	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

type Store struct {
	Storage  approval.Storage
	Codec    port.Codec
	Host     approval.Host
	Checkout readiness.Checkout
	Input    readiness.Input
}
type intent struct {
	store     Store
	ctx       context.Context
	directory approval.Directory
	candidate approval.Candidate
	record    port.Record
	binding   port.Binding
	bytes     []byte
	name      string
	fresh     bool
}

func (s Store) identity() (string, string, port.Outcome) {
	uid, gid := s.Host.Identity()
	u, ue := strconv.ParseUint(uid, 10, 32)
	g, ge := strconv.ParseUint(gid, 10, 32)
	if ue != nil || ge != nil || u == 0 || strconv.FormatUint(u, 10) != uid || strconv.FormatUint(g, 10) != gid || s.Input.OS != "linux" || s.Input.Architecture != "amd64" || s.Input.Context != "" || s.Input.TLSVerify != "" || s.Input.CertPath != "" || s.Input.APIVersion != "" {
		return "", "", port.Refused
	}
	return uid, gid, ""
}
func (s Store) candidate(project, token string) (approval.Candidate, port.Outcome) {
	if !port.HexID(project) || !port.HexID(token) {
		return approval.Candidate{}, port.Invalid
	}
	uid, gid, failure := s.identity()
	if failure != "" {
		return approval.Candidate{}, failure
	}
	return approval.Candidate{UID: uid, GID: gid, Root: s.Input.Root, Project: project}, ""
}

// openIntents opens the intent collection; missing is the outcome when a
// read-only open finds no collection.
func (s Store) openIntents(ctx context.Context, candidate approval.Candidate, create bool, missing port.Outcome) (approval.Directory, port.Outcome) {
	directory, f := s.Storage.Open(ctx, candidate, "intents", create)
	if f != "" {
		return nil, storageFailure(f, create)
	}
	if directory == nil {
		return nil, missing
	}
	return directory, ""
}

func (s Store) acquireMissing(ctx context.Context, project, token string, fresh bool, missing port.Outcome) (*intent, port.Outcome) {
	candidate, failure := s.candidate(project, token)
	if failure != "" {
		return nil, failure
	}
	directory, failure := s.openIntents(ctx, candidate, false, missing)
	if failure != "" {
		return nil, failure
	}
	if fresh && s.Checkout.Pin(s.Input.Root) != readiness.None {
		directory.Close()
		return nil, port.Refused
	}
	return &intent{store: s, ctx: budget.Lifetime(ctx), directory: directory, candidate: candidate, name: token + ".json", fresh: fresh}, ""
}
func (s Store) New(ctx context.Context, durable approval.Durable, token string) (port.Intent, port.Outcome) {
	return s.newIntent(ctx, durable, token, "intent")
}
func (s Store) Prepare(ctx context.Context, durable approval.Durable, token string) (port.Intent, port.Outcome) {
	return s.newIntent(ctx, durable, token, "reserved")
}
func (s Store) newIntent(ctx context.Context, durable approval.Durable, token, stage string) (port.Intent, port.Outcome) {
	binding, failure := s.Codec.Frozen(durable.Frozen)
	if failure != "" {
		return nil, failure
	}
	candidate, failure := s.candidate(durable.Frozen.Project, token)
	if failure != "" {
		return nil, failure
	}
	t := &intent{store: s, ctx: budget.Lifetime(ctx), candidate: candidate, binding: binding, name: token + ".json", fresh: true}
	t.record = binding.Intent(token)
	t.record.Stage = stage
	success := false
	defer func() {
		if !success {
			t.Close()
		}
	}()
	// The checkout scans may use their whole call budget, and a hostile checkout
	// can make them slow. Creating the intent collection takes the account-wide
	// control lock, so scan first and hold that lock only for the record checks
	// and the publication: one slow checkout must not delay other projects.
	if s.Checkout.Pin(s.Input.Root) != readiness.None {
		return nil, port.Refused
	}
	if failure = t.validateCheckout(); failure != "" {
		return nil, failure
	}
	t.directory, failure = s.openIntents(ctx, candidate, true, port.Refused)
	if failure != "" {
		return nil, failure
	}
	old, f := t.directory.Read(t.name)
	if f != "" {
		return nil, storageFailure(f, true)
	}
	if old != nil {
		return nil, port.Refused
	}
	if failure = t.validateRecord(); failure != "" {
		return nil, failure
	}
	data := s.Codec.Encode(t.record)
	if f = t.directory.Publish(t.name, nil, data); f != "" {
		return nil, storageFailure(f, true)
	}
	t.bytes = data
	success = true
	return t, ""
}
func (s Store) Open(ctx context.Context, reference string) (port.Intent, port.Outcome) {
	return s.open(ctx, reference, true)
}
func (s Store) OpenCleanup(ctx context.Context, reference string) (port.Intent, port.Outcome) {
	return s.open(ctx, reference, false)
}
func (s Store) open(ctx context.Context, reference string, fresh bool) (port.Intent, port.Outcome) {
	return s.openMissing(ctx, reference, fresh, port.Refused)
}

func (s Store) openMissing(ctx context.Context, reference string, fresh bool, missing port.Outcome) (port.Intent, port.Outcome) {
	project, token, ok := port.ParseReference(reference)
	if !ok {
		return nil, port.Invalid
	}
	t, failure := s.acquireMissing(ctx, project, token, fresh, missing)
	if failure != "" {
		return nil, failure
	}
	success := false
	defer func() {
		if !success {
			t.Close()
		}
	}()
	data, f := t.directory.Read(t.name)
	if f != "" {
		return nil, storageFailure(f, false)
	}
	if data == nil {
		return nil, missing
	}
	record, failure := s.Codec.Decode(data)
	if failure != "" {
		return nil, failure
	}
	if record.Project != project || record.NameToken() != token {
		return nil, port.Invalid
	}
	t.record, t.bytes = record, data
	t.binding, failure = t.snapshot()
	if failure != "" {
		return nil, failure
	}
	if failure = t.validateBinding(); failure != "" {
		return nil, failure
	}
	success = true
	return t, ""
}
func storageFailure(f approval.Failure, writing bool) port.Outcome {
	switch f {
	case "state.path", "state.unsafe", "state.location":
		return port.Unsafe
	case "state.invalid":
		return port.Invalid
	case "state.conflict":
		return port.Refused
	default:
		if writing {
			return port.IntentWrite
		}
		return port.Incomplete
	}
}
func (t *intent) snapshot() (port.Binding, port.Outcome) {
	directory, f := t.store.Storage.Open(t.ctx, t.candidate, "snapshots", false)
	if f != "" {
		return port.Binding{}, storageFailure(f, false)
	}
	if directory == nil {
		return port.Binding{}, port.Invalid
	}
	defer directory.Close()
	data, f := directory.Read(t.record.Generation + ".json")
	if f != "" {
		return port.Binding{}, storageFailure(f, false)
	}
	if data == nil {
		return port.Binding{}, port.Invalid
	}
	return t.store.Codec.Snapshot(data)
}
func (t *intent) validateBinding() port.Outcome {
	if failure := t.validateRecord(); failure != "" {
		return failure
	}
	if t.fresh {
		return t.validateCheckout()
	}
	return ""
}

// validateRecord checks the snapshot, the record and the host identity against
// the binding; it never scans the checkout.
func (t *intent) validateRecord() port.Outcome {
	b, failure := t.snapshot()
	if failure != "" {
		return failure
	}
	if b != t.binding {
		return port.Invalid
	}
	expected := b.Intent(t.record.NameToken())
	expected.Stage, expected.ContainerID = t.record.Stage, t.record.ContainerID
	expected.Schema, expected.Anchor = t.record.Schema, t.record.Anchor
	if t.record != expected || !t.record.ValidAnchor(b) {
		return port.Invalid
	}
	uid, gid, failure := t.store.identity()
	if failure != "" || b.UID != uid || b.GID != gid {
		return port.Refused
	}
	return ""
}

// validateCheckout runs the two full checkout scans against the pinned root.
func (t *intent) validateCheckout() port.Outcome {
	checkout, f := t.store.Checkout.Inspect()
	if f != readiness.None || checkout != t.binding.Checkout || !t.store.Checkout.Unchanged() {
		return port.Refused
	}
	return ""
}
func (t *intent) Record() port.Record   { return t.record }
func (t *intent) Binding() port.Binding { return t.binding }
func (t *intent) Close() {
	if t.fresh {
		t.store.Checkout.Close()
	}
	if t.directory != nil {
		t.directory.Close()
	}
}
func (t *intent) Validate() port.Outcome {
	data, f := t.directory.Read(t.name)
	if f != "" {
		return storageFailure(f, false)
	}
	if data == nil || !bytes.Equal(data, t.bytes) {
		return port.Refused
	}
	return t.validateBinding()
}
func (t *intent) Save(next port.Record) port.Outcome {
	if failure := t.Validate(); failure != "" {
		return failure
	}
	if !t.record.ValidTransition(next) {
		return port.Invalid
	}
	data := t.store.Codec.Encode(next)
	var f approval.Failure
	if next == t.record {
		// Sync preserves the existing representation, which need not match a
		// fresh encoding of an independently written, otherwise valid record.
		data = t.bytes
		f = t.directory.Sync(t.name, t.bytes)
	} else {
		f = t.directory.Publish(t.name, t.bytes, data)
	}
	if f != "" {
		// Publication may already be visible although parent fsync failed. Retain
		// exactly that complete version for fresh inspection and bounded recovery.
		visible, readFailure := t.directory.Read(t.name)
		if readFailure == "" && bytes.Equal(visible, data) {
			t.record, t.bytes = next, data
		}
		return storageFailure(f, true)
	}
	t.record, t.bytes = next, data
	return ""
}
