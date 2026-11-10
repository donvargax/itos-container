package main

import (
	"strings"
	"testing"
	"testing/synctest"

	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestAnchorRecoveryNoCreate(t *testing.T) {
	for _, stage := range []string{"reserved", "cancelled"} {
		t.Run(stage, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				anchorRecoveryMemoryCase(t, map[string]string{
					"mode": "no-create", "fault": stage, "phase": "preparing",
					"worker": "absent", "volume": "present", "completion": "pending", "exit": "unknown",
				})
			})
		})
	}
}

func anchorRecoveryMemoryNoCreate(t *testing.T, q map[string]string, m publicMemory, e *anchorRecoveryMemoryEngine, s domain.Recovery, id, worker string, r sp.Record, p sp.PublicRecord) {
	t.Helper()
	r.Stage, r.ContainerID = q["fault"], ""
	dir := m.storage.Directories["intents/"+m.c.Project]
	dir.Files[r.NameToken()+".json"] = m.registry.Store.Codec.Encode(r)
	anchorRecoveryMemoryStop(t, m, e, s, id, worker, r, p)
	inspections := e.inspections
	code, report := anchorMemoryInvoke(s, "status", id)
	if code != 0 {
		t.Fatalf("status after no-create cleanup returned %d: %+v", code, report)
	}
	found := false
	for _, entry := range report.Sessions {
		if entry["id"] != id {
			continue
		}
		found = true
		for key, want := range map[string]string{"state": "stopped", "resource": "absent", "anchor": "absent", "cleanup": "complete"} {
			if entry[key] != want {
				t.Errorf("%s = %v, want %s", key, entry[key], want)
			}
		}
		if entry["workload_exit"] != nil || entry["retry"] != nil || entry["stop_candidate"] != false {
			t.Error("no-create cleanup invented workload exit or retained retry", entry)
		}
	}
	if !found || e.inspections != inspections+1 || e.deletions != 1 {
		t.Error("missing session or independent anchor inspection/release", report, e.inspections, e.deletions)
	}
	for _, event := range m.e.Engine.Events {
		if event.Method != "INFO" && (event.Method != "INSPECT" || event.Target != strings.Repeat("c", 63)+"2") {
			t.Error("no-create recovery looked up or mutated a worker", event)
		}
	}
	want := r
	want.Stage, want.Anchor.Stage = "cancelled", "absent"
	got, failure := m.registry.Store.Codec.Decode(dir.Files[r.NameToken()+".json"])
	if failure != "" || got != want {
		t.Error("no-create completion lost original intent", got, failure)
	}
}
