// Package disk binds session records to protected storage and original snapshots.
package disk

import (
	"bytes"
	"context"
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
func (s Store) acquire(ctx context.Context, project, token string, create bool) (*intent, port.Outcome) {
	if !port.HexID(project) || !port.HexID(token) {
		return nil, port.Invalid
	}
	uid, gid, failure := s.identity()
	if failure != "" {
		return nil, failure
	}
	candidate := approval.Candidate{UID: uid, GID: gid, Root: s.Input.Root, Project: project}
	directory, f := s.Storage.Open(ctx, candidate, "intents", create)
	if f != "" {
		return nil, storageFailure(f, create)
	}
	if directory == nil {
		return nil, port.Refused
	}
	if s.Checkout.Pin(s.Input.Root) != readiness.None {
		directory.Close()
		return nil, port.Refused
	}
	return &intent{store: s, ctx: ctx, directory: directory, candidate: candidate, name: token + ".json"}, ""
}
func (s Store) New(ctx context.Context, durable approval.Durable, token string) (port.Intent, port.Outcome) {
	binding, failure := s.Codec.Frozen(durable.Frozen)
	if failure != "" {
		return nil, failure
	}
	t, failure := s.acquire(ctx, durable.Frozen.Project, token, true)
	if failure != "" {
		return nil, failure
	}
	success := false
	defer func() {
		if !success {
			t.Close()
		}
	}()
	old, f := t.directory.Read(t.name)
	if f != "" {
		return nil, storageFailure(f, true)
	}
	if old != nil {
		return nil, port.Refused
	}
	t.binding = binding
	t.record = binding.Intent(token)
	if failure = t.validateBinding(); failure != "" {
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
	project, token, ok := port.ParseReference(reference)
	if !ok {
		return nil, port.Invalid
	}
	t, failure := s.acquire(ctx, project, token, false)
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
		return nil, port.Refused
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
	b, failure := t.snapshot()
	if failure != "" {
		return failure
	}
	if b != t.binding {
		return port.Invalid
	}
	expected := b.Intent(t.record.NameToken())
	expected.Stage, expected.ContainerID = t.record.Stage, t.record.ContainerID
	if t.record != expected {
		return port.Invalid
	}
	uid, gid, failure := t.store.identity()
	if failure != "" || b.UID != uid || b.GID != gid {
		return port.Refused
	}
	checkout, f := t.store.Checkout.Inspect()
	if f != readiness.None || checkout != b.Checkout || !t.store.Checkout.Unchanged() {
		return port.Refused
	}
	return ""
}
func (t *intent) Record() port.Record   { return t.record }
func (t *intent) Binding() port.Binding { return t.binding }
func (t *intent) Close()                { t.store.Checkout.Close(); t.directory.Close() }
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
	expected := t.record
	expected.Stage, expected.ContainerID = next.Stage, next.ContainerID
	if next != expected || !port.HexID(next.ContainerID) || (next.Stage != "owned" && next.Stage != "absent") || (t.record.Stage == "absent" && next != t.record) || (t.record.ContainerID != "" && next.ContainerID != t.record.ContainerID) {
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
