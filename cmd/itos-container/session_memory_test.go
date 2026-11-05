package main

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	sessiondisk "github.com/donvargax/itos-container/internal/engine/session/disk"
	session "github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
	fake "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

type sessionMemoryInput struct {
	Records, Snapshots                                                  map[string][]byte
	Resources                                                           map[string]sp.Observation
	UnsafeRecord, UnsafeSnapshot                                        bool
	Endpoint, DaemonID, CreateID, Inspection, Following, Removal, Fault string
	Unavailable, DisagreeName, Removed                                  bool
	ImageInspection                                                     string
}
type sessionMemoryOutput struct {
	Store  *fake.Store
	Engine *fake.Engine
}

func assembleMemorySession(t *testing.T, request sessionContractRequest, controller session.Service) (session.Service, *sessionMemoryOutput) {
	t.Helper()
	data, err := os.ReadFile(request.MemoryFile)
	if err != nil {
		t.Fatal(err)
	}
	var seed sessionMemoryInput
	if err = json.Unmarshal(data, &seed); err != nil {
		t.Fatal(err)
	}
	original := controller.Store.(sessiondisk.Store)
	uid, gid := original.Host.Identity()
	store := &fake.Store{Codec: original.Codec, Records: seed.Records, Snapshots: seed.Snapshots, UID: uid, GID: gid, Root: original.Input.Root, UnsafeRecord: seed.UnsafeRecord, UnsafeSnapshot: seed.UnsafeSnapshot, Fault: request.Fault}
	engine := &fake.Engine{ImageInspection: seed.ImageInspection, Store: store, Resources: seed.Resources, Endpoint: seed.Endpoint, DaemonID: seed.DaemonID, CreateID: seed.CreateID, Inspection: seed.Inspection, Following: seed.Following, Removal: seed.Removal, Fault: request.Fault, DisagreeName: seed.DisagreeName, Removed: seed.Removed, Unavailable: seed.Unavailable}
	controller.Store, controller.Engine = store, engine
	return controller, &sessionMemoryOutput{Store: store, Engine: engine}
}
func (m *sessionMemoryOutput) events() []sessionContractEvent {
	var result []sessionContractEvent
	for _, e := range m.Store.Events {
		result = append(result, sessionContractEvent{Operation: e.Operation, Stage: e.Stage, Bytes: e.Bytes, Failed: e.Failed})
	}
	return result
}

func TestSessionMemoryOutputSerializesObservations(t *testing.T) {
	for _, armed := range []bool{false, true} {
		t.Run(map[bool]string{false: "nil barriers", true: "armed barriers"}[armed], func(t *testing.T) {
			store := &fake.Store{
				Records: map[string][]byte{"intent.json": []byte(`{"stage":"owned"}`)},
				Events:  []fake.WriteEvent{{Operation: "parent fsync", Stage: "owned"}},
			}
			engine := &fake.Engine{
				Store:     store,
				Resources: map[string]sp.Observation{"worker": {ID: "worker", Status: "created"}},
				Events:    []fake.EngineEvent{{Method: "INSPECT", Target: "worker"}},
			}
			if armed {
				barrier := func() { t.Error("serializing observations executed a fixture barrier") }
				store.AfterSave = barrier
				engine.AfterOpen, engine.BeforeInspect, engine.AfterInspect = barrier, barrier, barrier
			}
			data, err := json.Marshal(sessionMemoryOutput{Store: store, Engine: engine})
			if err != nil {
				t.Fatalf("serialize session observations: %v", err)
			}
			var observed sessionMemoryOutput
			if err := json.Unmarshal(data, &observed); err != nil {
				t.Fatalf("read session observations: %v", err)
			}
			if observed.Store == nil || observed.Engine == nil ||
				!reflect.DeepEqual(observed.Store.Records, store.Records) ||
				!reflect.DeepEqual(observed.Store.Events, store.Events) ||
				!reflect.DeepEqual(observed.Engine.Resources, engine.Resources) ||
				!reflect.DeepEqual(observed.Engine.Events, engine.Events) {
				t.Fatal("session observation export lost records, resources or ordered events")
			}
		})
	}
}
