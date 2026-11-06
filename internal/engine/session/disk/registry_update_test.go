package disk

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	ap "github.com/donvargax/itos-container/internal/engine/approval/port"
	af "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	sf "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

type registryFixture struct {
	registry Registry
	binding  sp.Binding
	files    *af.Registry
	storage  *af.Storage
	codec    *sf.PublicRecords
	record   sp.PublicRecord
}

func newRegistryFixture(t *testing.T) registryFixture {
	t.Helper()
	store, binding, storage, _ := intentReadFixture(t)
	record := sp.PublicRecord{Schema: 1, ID: strings.Repeat("b", 64), Project: binding.Frozen.Project,
		Intent: sp.Reference(binding.Frozen.Project, strings.Repeat("c", 64)), State: "preparing"}
	codec := &sf.PublicRecords{Records: map[string]sp.PublicRecord{"original": record}}
	files := &af.Registry{Sessions: map[string]*af.SessionFiles{}}
	registry := Registry{Store: store, Storage: files, Codec: codec, Identity: store.Codec.(*sf.Codec)}
	return registryFixture{registry, binding, files, storage, codec, record}
}

func (m registryFixture) seed() *af.SessionFiles {
	intent := m.binding.Intent(strings.Repeat("c", 64))
	m.storage.Directories["intents/"+m.binding.Frozen.Project].Files[intent.NameToken()+".json"] = m.registry.Store.Codec.Encode(intent)
	files := &af.SessionFiles{Data: m.codec.EncodePublic(m.record)}
	m.files.Sessions[registryKey(m.binding.Checkout.Root, m.binding.UID)+"/"+m.record.ID] = files
	return files
}

func TestRegistryContractWithStorageFakes(t *testing.T) {
	sf.RegistryContract(t, func() (sp.Registry, sp.Store, ap.Durable) {
		m := newRegistryFixture(t)
		initial := sp.PublicRecord{Schema: 1, ID: strings.Repeat("a", 64), Project: m.binding.Frozen.Project,
			Intent: sp.Reference(m.binding.Frozen.Project, strings.Repeat("1", 64)), State: "preparing"}
		progress := initial
		exit := 7
		progress.State, progress.WorkloadExit = "failed", &exit
		m.codec.Records = map[string]sp.PublicRecord{"initial": initial, "progress": progress}
		return m.registry, m.registry.Store, ap.Durable{Frozen: m.binding.Frozen}
	})
}

func TestRegistryRejectsInvalidRegistrationBeforePublication(t *testing.T) {
	for _, fault := range []string{"id", "token", "project", "frozen", "gid"} {
		t.Run(fault, func(t *testing.T) {
			m := newRegistryFixture(t)
			id, token := m.record.ID, strings.Repeat("c", 64)
			durable := ap.Durable{Frozen: m.binding.Frozen}
			want := sp.Refused
			switch fault {
			case "id":
				id = "../foreign"
			case "token":
				token = "invalid"
			case "project":
				durable.Frozen.Project = strings.Repeat("f", 64)
			case "frozen":
				durable.Frozen.Settings, want = "different settings", sp.Invalid
			case "gid":
				m.registry.Store.Host.(*af.Host).GID = "1001"
			}
			lease, failure := m.registry.Register(t.Context(), durable, id, token)
			if lease != nil {
				lease.Close()
			}
			if lease != nil || failure != want || len(m.files.Sessions) != 0 || len(m.files.Events) != 0 {
				t.Fatal("invalid registration acquired a lease or published state", failure)
			}
			m.registry.Store.Host.(*af.Host).GID = m.binding.GID
			lease, failure = m.registry.Register(t.Context(), ap.Durable{Frozen: m.binding.Frozen}, m.record.ID, strings.Repeat("c", 64))
			if failure != "" || lease == nil {
				t.Fatal("matching registration could not retry", failure)
			}
			defer lease.Close()
			files := m.files.Sessions[registryKey(m.binding.Checkout.Root, m.binding.UID)+"/"+m.record.ID]
			if files == nil || !files.Controller || string(files.Data) != "original" {
				t.Fatal("successful registration lost the original binding or controller lease")
			}
		})
	}
}

func TestRegistryRevalidationFailureReleasesLockAndPreservesEvidence(t *testing.T) {
	for _, tc := range []struct {
		failure ap.Failure
		want    sp.Outcome
	}{
		{"state.unavailable", sp.Incomplete}, {"state.path", sp.Unsafe},
		{"state.invalid", sp.Invalid}, {"state.conflict", sp.Refused},
	} {
		t.Run(string(tc.failure), func(t *testing.T) {
			m := newRegistryFixture(t)
			files := m.seed()
			before := bytes.Clone(files.Data)
			m.files.AfterRead = func() { m.files.ReadFailure = tc.failure }
			entry, failure := m.registry.Open(t.Context(), m.record.ID, false)
			if entry != nil {
				entry.Close()
			}
			if entry != nil || failure != tc.want {
				t.Fatal("failed revalidation granted authority", entry, failure)
			}
			m.files.AfterRead, m.files.ReadFailure = nil, ""
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			entry, failure = m.registry.Open(ctx, m.record.ID, false)
			if failure != "" || entry == nil {
				t.Fatal("failed open retained its operation lock", failure)
			}
			defer entry.Close()
			if entry.Record() != m.record || entry.Validate() != "" || !bytes.Equal(files.Data, before) || len(m.files.Events) != 0 {
				t.Fatal("failed revalidation changed ownership evidence")
			}
		})
	}
}

func TestRegistryReadOnlySessionCannotPublishOrRepairDurability(t *testing.T) {
	m := newRegistryFixture(t)
	files := m.seed()
	before := bytes.Clone(files.Data)
	entry, failure := m.registry.Open(t.Context(), m.record.ID, true)
	if failure != "" {
		t.Fatal(failure)
	}
	defer entry.Close()
	next := entry.Record()
	next.State = "running"
	m.codec.Records["running"] = next
	if failure := entry.Save(next); failure != sp.Refused {
		t.Fatal("read-only session published progress", failure)
	}
	if failure := entry.Sync(); failure != sp.Refused {
		t.Fatal("read-only session repaired durability", failure)
	}
	if !reflect.DeepEqual(entry.Record(), m.record) || !bytes.Equal(files.Data, before) || len(m.files.Events) != 0 {
		t.Fatal("read-only operations changed the record or storage")
	}
}

func TestRegistryUpdateRefusesChangedAuthorityAndInvalidProgress(t *testing.T) {
	for _, fault := range []string{"schema", "id", "project", "intent", "stopped state", "stopped problem", "lost exit", "changed exit", "invalid progress"} {
		t.Run(fault, func(t *testing.T) {
			m := newRegistryFixture(t)
			if strings.HasPrefix(fault, "stopped") {
				m.record.State = "stopped"
			}
			if strings.HasSuffix(fault, "exit") {
				exit := 7
				m.record.WorkloadExit = &exit
			}
			m.codec.Records["original"] = m.record
			files := m.seed()
			before := bytes.Clone(files.Data)
			entry, failure := m.registry.Open(t.Context(), m.record.ID, false)
			if failure != "" {
				t.Fatal(failure)
			}
			defer entry.Close()
			next, want := entry.Record(), sp.Refused
			switch fault {
			case "schema":
				next.Schema++
			case "id":
				next.ID = strings.Repeat("d", 64)
			case "project":
				next.Project = strings.Repeat("d", 64)
			case "intent":
				next.Intent = sp.Reference(next.Project, strings.Repeat("d", 64))
			case "stopped state":
				next.State = "running"
			case "stopped problem":
				problem := sp.Incomplete
				next.LastProblem = &problem
			case "lost exit":
				next.WorkloadExit = nil
			case "changed exit":
				*next.WorkloadExit = 0
			case "invalid progress":
				next.State, want = "future", sp.Invalid
			}
			if fault != "invalid progress" {
				// Even a codec-accepted replacement cannot change held authority.
				m.codec.Records["replacement"] = next
			}
			if failure := entry.Save(next); failure != want {
				t.Fatal("invalid update acquired authority", failure)
			}
			if !reflect.DeepEqual(entry.Record(), m.record) || !bytes.Equal(files.Data, before) || len(m.files.Events) != 0 {
				t.Fatal("refused update changed original progress or evidence")
			}
			if failure := entry.Save(entry.Record()); failure != "" {
				t.Fatal("refused update prevented retry with the original record", failure)
			}
		})
	}
}

func TestRegistrySaveAndSyncRevalidateBeforeWriting(t *testing.T) {
	for _, fault := range []string{"directory", "record", "read", "intent"} {
		t.Run(fault, func(t *testing.T) {
			m := newRegistryFixture(t)
			files := m.seed()
			entry, failure := m.registry.Open(t.Context(), m.record.ID, false)
			if failure != "" {
				t.Fatal(failure)
			}
			defer entry.Close()
			next := entry.Record()
			next.State = "running"
			m.codec.Records["running"] = next
			want := sp.Refused
			switch fault {
			case "directory":
				files.Changed = true
			case "record":
				files.Data = []byte("substituted")
			case "read":
				m.files.ReadFailure, want = "state.unavailable", sp.Incomplete
			case "intent":
				m.storage.Failure, want = "state.unavailable", sp.Incomplete
			}
			before := bytes.Clone(files.Data)
			if failure := entry.Save(next); failure != want {
				t.Fatal("save did not preserve the revalidation failure", failure)
			}
			if failure := entry.Sync(); failure != want {
				t.Fatal("sync did not preserve the revalidation failure", failure)
			}
			if entry.Record() != m.record || !bytes.Equal(files.Data, before) || len(m.files.Events) != 0 {
				t.Fatal("failed revalidation changed storage or held progress")
			}
		})
	}
}

func TestRegistryUnchangedSaveSyncsWithoutPublicationAndCanRetry(t *testing.T) {
	for _, fault := range []string{"", "file fsync", "parent fsync"} {
		t.Run(fault, func(t *testing.T) {
			m := newRegistryFixture(t)
			files := m.seed()
			before := bytes.Clone(files.Data)
			entry, failure := m.registry.Open(t.Context(), m.record.ID, false)
			if failure != "" {
				t.Fatal(failure)
			}
			defer entry.Close()
			m.files.Fault = fault
			want := sp.Outcome("")
			if fault != "" {
				want = sp.IntentWrite
			}
			if failure := entry.Save(entry.Record()); failure != want {
				t.Fatal("unchanged save lost durability failure", failure)
			}
			var expected []af.RegistryEvent
			for _, operation := range []string{"file fsync", "parent fsync"} {
				expected = append(expected, af.RegistryEvent{ID: m.record.ID, Operation: operation, Data: before, Failed: operation == fault})
				if operation == fault {
					break
				}
			}
			if !reflect.DeepEqual(m.files.Events, expected) || !bytes.Equal(files.Data, before) || entry.Record() != m.record {
				t.Fatal("unchanged save rewrote evidence or skipped durability", m.files.Events)
			}
			m.files.Fault, m.files.Events = "", nil
			if failure := entry.Save(entry.Record()); failure != "" {
				t.Fatal("unchanged retry did not restore durability", failure)
			}
			expected = []af.RegistryEvent{
				{ID: m.record.ID, Operation: "file fsync", Data: before},
				{ID: m.record.ID, Operation: "parent fsync", Data: before},
			}
			if !reflect.DeepEqual(m.files.Events, expected) || !bytes.Equal(files.Data, before) || entry.Record() != m.record {
				t.Fatal("durability retry did not preserve the original evidence", m.files.Events)
			}
		})
	}
}

func TestRegistryIntentSaveRejectsFailedReopenAndPreservesRetry(t *testing.T) {
	for _, fault := range []string{"unavailable", "unsafe", "missing", "replaced"} {
		t.Run(fault, func(t *testing.T) {
			m := newRegistryFixture(t)
			files := m.seed()
			publicData := bytes.Clone(files.Data)
			entry, failure := m.registry.Open(t.Context(), m.record.ID, false)
			if failure != "" {
				t.Fatal(failure)
			}
			defer entry.Close()
			intent := entry.Intent()
			original := intent.Record()
			directory := m.storage.Directories["intents/"+original.Project]
			name := original.NameToken() + ".json"
			data := bytes.Clone(directory.Files[name])
			next := original
			next.Stage, next.ContainerID = "owned", strings.Repeat("d", 64)
			want := sp.Refused
			switch fault {
			case "unavailable":
				m.storage.Failure, want = "state.unavailable", sp.Incomplete
			case "unsafe":
				directory.ReadFailure, want = "state.path", sp.Unsafe
			case "missing":
				delete(directory.Files, name)
			case "replaced":
				replacement := next
				replacement.ContainerID = strings.Repeat("e", 64)
				directory.Files[name] = m.registry.Store.Codec.Encode(replacement)
			}
			before := bytes.Clone(directory.Files[name])
			if failure := intent.Save(next); failure != want {
				t.Fatal("failed reopen authorized an intent update", failure)
			}
			if intent.Record() != original || intent.Binding() != m.binding || !bytes.Equal(directory.Files[name], before) || directory.Locked || !bytes.Equal(files.Data, publicData) || len(m.files.Events) != 0 {
				t.Fatal("failed intent reopen changed held authority, storage or locks")
			}
			m.storage.Failure, directory.ReadFailure = "", ""
			directory.Files[name] = bytes.Clone(data)
			if failure := intent.Save(next); failure != "" {
				t.Fatal("matching intent could not retry publication", failure)
			}
			if intent.Record() != next || intent.Validate() != "" || directory.Locked || !bytes.Equal(directory.Files[name], m.registry.Store.Codec.Encode(next)) || !bytes.Equal(files.Data, publicData) {
				t.Fatal("successful intent retry lost ownership or changed public progress")
			}
		})
	}
}
