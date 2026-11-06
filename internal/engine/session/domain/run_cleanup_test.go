package domain_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestForegroundLostCreateReplyReconcilesWithoutStarting(t *testing.T) {
	s, frozen, store, engine := runnerFixture()
	engine.Fault = "lost create response"
	result := s.Run(context.Background(), frozen, []string{"probe"}, port.Streams{})
	if result.Outcome != port.Uncertain || result.Cleanup != port.Complete || result.Exited || result.ID != "" {
		t.Fatal(result)
	}
	if slices.Contains(engine.Events, "start") || len(engine.Resources) != 0 {
		t.Fatal("uncertain create started work or left its owned worker", engine.Events)
	}
	creates, removals := 0, 0
	for _, event := range engine.Engine.Events {
		switch event.Method {
		case "CREATE":
			creates++
		case "REMOVE":
			removals++
			record, failure := store.Codec.Decode([]byte(event.Intent))
			if failure != "" || record.Stage != "owned" || record.ContainerID != event.Target {
				t.Fatal("removal without durable reconciled identity", event)
			}
		}
	}
	if creates != 1 || removals != 1 {
		t.Fatal("create was retried or cleanup was skipped", creates, removals)
	}
}

func TestForegroundPersistentOwnershipFailureRetainsRecoverableIntent(t *testing.T) {
	s, frozen, store, engine := runnerFixture()
	store.Fault = "owned-record file fsync"
	result := s.Run(context.Background(), frozen, []string{"probe", "private-argument"}, port.Streams{})
	if result.Outcome != port.OwnershipWrite || result.Cleanup != port.OwnershipWrite || result.Exited || result.ID != "" {
		t.Fatal(result)
	}
	if slices.Contains(engine.Events, "start") || slices.Contains(engine.Events, "remove") || len(engine.Resources) != 1 {
		t.Fatal("undurable ownership permitted a mutation", engine.Events)
	}
	record, failure := store.Codec.Decode(store.Records[result.Reference])
	if failure != "" || record.Stage != "intent" || record.ContainerID != "" || strings.Contains(string(store.Records[result.Reference]), "private-argument") {
		t.Fatal("failed publication lost the protected intent", record, failure)
	}
	store.Fault = ""
	if recovery := s.Cleanup(context.Background(), result.Reference); recovery.Outcome != port.Complete {
		t.Fatal(recovery)
	}
	if len(engine.Resources) != 0 || slices.Contains(engine.Events, "start") || result.Outcome != port.OwnershipWrite {
		t.Fatal("recovery changed the original outcome or launched work", result, engine.Events)
	}
}

func TestForegroundCleanupRefusesChangedOwnership(t *testing.T) {
	for _, defect := range []string{
		"different full container ID", "different name", "different image config ID",
		"different intent label", "different project label", "different fingerprint label",
		"missing State", "unavailable", "daemon", "snapshot", "record", "host identity",
	} {
		t.Run(defect, func(t *testing.T) {
			s, frozen, store, engine := runnerFixture()
			engine.After = func(event string) {
				if event != "drain" {
					return
				}
				switch defect {
				case "daemon":
					engine.DaemonID = "different-daemon"
				case "snapshot":
					store.UnsafeSnapshot = true
				case "record":
					store.UnsafeRecord = true
				case "host identity":
					store.UID = "0"
				default:
					engine.Inspection = defect
				}
			}
			result := s.Run(context.Background(), frozen, []string{"probe"}, port.Streams{})
			if result.Outcome != "" || !result.Exited || result.Exit != 0 || result.Cleanup == port.Complete || result.Reference == "" {
				t.Fatal(result)
			}
			if slices.Contains(engine.Events, "remove") || len(engine.Resources) != 1 {
				t.Fatal("uncertain ownership allowed removal", engine.Events)
			}
		})
	}
}

func TestForegroundCompletionRequiresDurableAbsence(t *testing.T) {
	s, frozen, store, engine := runnerFixture()
	store.Fault = "absent-record file fsync"
	result := s.Run(context.Background(), frozen, []string{"probe"}, port.Streams{})
	if result.Outcome != "" || !result.Exited || result.Cleanup != port.CompletionWrite || len(engine.Resources) != 0 {
		t.Fatal(result)
	}
	record, failure := store.Codec.Decode(store.Records[result.Reference])
	if failure != "" || record.Stage != "owned" || record.ContainerID != engine.CreateID {
		t.Fatal("lost recovery identity after removal", record, failure)
	}
	store.Fault = ""
	engine.Events = nil
	if recovery := s.Cleanup(context.Background(), result.Reference); recovery.Outcome != port.Complete {
		t.Fatal(recovery)
	}
	if slices.Contains(engine.Events, "remove") {
		t.Fatal("removed an already absent worker")
	}
	record, failure = store.Codec.Decode(store.Records[result.Reference])
	if failure != "" || record.Stage != "absent" || record.ContainerID != engine.CreateID {
		t.Fatal("absence was not recorded", record, failure)
	}
}

func TestForegroundRevalidatesAfterAttachAndBeforeCleanupMutation(t *testing.T) {
	for _, boundary := range []string{"attach", "cleanup publication"} {
		t.Run(boundary, func(t *testing.T) {
			s, frozen, store, engine := runnerFixture()
			engine.After = func(event string) {
				if event == "attach" && boundary == "attach" {
					store.UnsafeSnapshot = true
				}
				if event == "drain" && boundary == "cleanup publication" {
					store.AfterSave = func() { engine.DaemonID = "replacement-daemon" }
				}
			}
			result := s.Run(context.Background(), frozen, []string{"probe"}, port.Streams{})
			if result.Cleanup == port.Complete || slices.Contains(engine.Events, "remove") || len(engine.Resources) != 1 {
				t.Fatal("changed authority permitted cleanup", result, engine.Events)
			}
			if boundary == "attach" && (result.Outcome != port.Unsafe || slices.Contains(engine.Events, "start")) {
				t.Fatal("changed snapshot permitted start", result, engine.Events)
			}
			if boundary == "cleanup publication" && (!result.Exited || result.Cleanup != port.Refused) {
				t.Fatal("cleanup lost the workload outcome or daemon refusal", result)
			}
		})
	}
}
