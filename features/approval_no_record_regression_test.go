package features

import (
	"os"
	"path/filepath"
	"testing"
)

// TestApprovalNoRecordRejectsFrozenInput holds the step "no approval record or
// frozen input is produced" to its whole claim: with no current approval
// record, a frozen snapshot left for the project must still fail the step.
func TestApprovalNoRecordRejectsFrozenInput(t *testing.T) {
	state := t.TempDir()
	root := filepath.Join(state, "itos-container")
	record := filepath.Join(root, "approvals", "project", "current.json")
	w := &world{approval: &approvalWorld{project: "project", record: record}}

	snapshots := filepath.Join(root, "snapshots", "project")
	if err := os.MkdirAll(snapshots, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := w.approvalNoRecord(); err != nil {
		t.Fatalf("no record and an empty snapshot directory must pass: %v", err)
	}

	frozen := approvalJSON(map[string]any{"schema": 1, "project": "project", "generation": "1"})
	if err := os.WriteFile(filepath.Join(snapshots, "1.json"), frozen, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := w.approvalNoRecord(); err == nil {
		t.Fatal("a frozen snapshot with no approval record must fail the step")
	}
}
