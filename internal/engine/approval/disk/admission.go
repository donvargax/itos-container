package disk

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

func (s Registry) Admission(ctx context.Context, c port.Candidate) (port.AdmissionDirectory, port.Failure) {
	transaction, failure := s.Store.control(ctx, c, true)
	if failure != "" {
		return nil, failure
	}
	success := false
	defer func() {
		if !success {
			transaction.Close()
		}
	}()
	store := s.store(ctx)
	location, failure := store.locationAt(c, "sessions")
	if failure != "" {
		return nil, failure
	}
	base := filepath.Dir(filepath.Dir(location))
	p, failure := store.pinPath(c, true, filepath.Join(base, "admission"), 2)
	if failure != "" {
		return nil, failure
	}
	success = true
	return &admissionDirectory{
		directoryStore: directoryStore{pin: p, ctx: ctx, exclusive: (*os.Root).Link, control: transaction},
		store:          store, candidate: c, sessions: filepath.Join(base, "sessions"),
	}, ""
}

type admissionDirectory struct {
	directoryStore
	store     Store
	candidate port.Candidate
	sessions  string
}

func (d *admissionDirectory) Names() ([]string, port.Failure) {
	if d.pin == nil {
		return nil, ""
	}
	names, failure := pinnedNames(d.pin)
	if failure != "" {
		return nil, failure
	}
	var records []string
	for _, name := range names {
		// A killed private-file publisher can leave a temporary file. It never
		// acquired a public reservation or authority to create a worker.
		if strings.HasPrefix(name, ".intent-") && strings.HasSuffix(name, ".tmp") {
			continue
		}
		if !recordName(name) || len(name) != 69 {
			return nil, "state.invalid"
		}
		records = append(records, name)
	}
	return records, ""
}

func pinnedNames(p *pin) ([]string, port.Failure) {
	entries, err := p.dir.ReadDir(-1)
	if err != nil {
		return nil, "state.unavailable"
	}
	if !p.unchanged() {
		return nil, "state.path"
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names, ""
}

func (d *admissionDirectory) Sessions() ([]port.RegisteredSession, port.Failure) {
	// Registration holds the common admission lock. Progress is atomically
	// replaced under each operation lock, which may span engine calls; scanning
	// must not acquire that lock or obstruct independent recovery.
	store := d.store
	ops := *store.ops
	ops.lock = func(*os.File, bool) error { return nil }
	store.ops = &ops
	p, failure := store.pinPath(d.candidate, false, d.sessions, 2)
	if failure != "" || p == nil {
		return nil, failure
	}
	defer p.close()
	indexes, failure := pinnedNames(p)
	if failure != "" {
		return nil, failure
	}
	var records []port.RegisteredSession
	for _, index := range indexes {
		if !publicID(index) {
			return nil, "state.invalid"
		}
		entries, failure := d.projectSessions(store, index)
		if failure != "" {
			return nil, failure
		}
		records = append(records, entries...)
	}
	if !d.Unchanged() || !p.unchanged() {
		return nil, "state.path"
	}
	return records, ""
}

func (d *admissionDirectory) projectSessions(store Store, index string) ([]port.RegisteredSession, port.Failure) {
	path := filepath.Join(d.sessions, index)
	p, failure := store.pinPath(d.candidate, false, path, 3)
	if failure != "" {
		return nil, failure
	}
	if p == nil {
		return nil, "state.path"
	}
	defer p.close()
	ids, failure := pinnedNames(p)
	if failure != "" {
		return nil, failure
	}
	var records []port.RegisteredSession
	for _, id := range ids {
		if !publicID(id) {
			return nil, "state.invalid"
		}
		data, failure := d.sessionRecord(store, filepath.Join(path, id))
		if failure != "" {
			return nil, failure
		}
		records = append(records, port.RegisteredSession{Index: index, ID: id, Data: data})
	}
	if !p.unchanged() {
		return nil, "state.path"
	}
	return records, ""
}

func (d *admissionDirectory) sessionRecord(store Store, path string) ([]byte, port.Failure) {
	p, failure := store.pinPath(d.candidate, false, path, 4)
	if failure != "" {
		return nil, failure
	}
	if p == nil {
		return nil, "state.path"
	}
	defer p.close()
	reader := directoryStore{pin: p, ctx: d.ctx, public: true}
	// Progress may replace the inode between lookup, open and final validation.
	// A descriptor unlinked before fstat also fails the link-count safety check.
	// Retry the entire checked read, without taking the operation lock. A stable
	// unsafe record still refuses, and churn cannot hold admission indefinitely.
	for range 3 {
		data, readFailure := reader.Read("record.json")
		if readFailure != "state.path" && readFailure != "state.unsafe" {
			return data, readFailure
		}
		if !reader.Unchanged() {
			return nil, readFailure
		}
		// Preserve the refusal if all attempts fail validation.
		failure = readFailure
	}
	return nil, failure
}

func (d *admissionDirectory) Close() {
	if d.pin != nil {
		d.directoryStore.Close()
	}
}
func (d *admissionDirectory) Unchanged() bool {
	return d.ctx.Err() == nil && (d.pin == nil || d.directoryStore.Unchanged())
}
