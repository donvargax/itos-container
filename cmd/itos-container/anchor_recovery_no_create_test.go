package main

import (
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/domain"
	sp "github.com/donvargax/itos-container/internal/engine/session/port"
)

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
	for _, event := range m.e.Events {
		if strings.Contains(event, worker) || strings.Contains(event, r.Name) {
			t.Error("no-create recovery looked up a worker", event)
		}
	}
	want := r
	want.Stage, want.Anchor.Stage = "cancelled", "absent"
	got, failure := m.registry.Store.Codec.Decode(dir.Files[r.NameToken()+".json"])
	if failure != "" || got != want {
		t.Error("no-create completion lost original intent", got, failure)
	}
}
