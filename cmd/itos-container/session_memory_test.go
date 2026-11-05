package main

import (
	"encoding/json"
	"os"
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
	engine := &fake.Engine{Store: store, Resources: seed.Resources, Endpoint: seed.Endpoint, DaemonID: seed.DaemonID, CreateID: seed.CreateID, Inspection: seed.Inspection, Following: seed.Following, Removal: seed.Removal, Fault: request.Fault, DisagreeName: seed.DisagreeName, Removed: seed.Removed, Unavailable: seed.Unavailable}
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
