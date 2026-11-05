package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSessionContractIOFaultIsPersistentAndLeavesIntent(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux disk durability fixture")
	}
	dir := t.TempDir()
	journal := filepath.Join(dir, "observations.jsonl")
	if err := os.WriteFile(journal, nil, 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	directory, err := root.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = directory.Close() }()
	ops := &sessionContractIO{journal: journal, fault: "owned-record file fsync"}
	intent := []byte(`{"stage":"intent","container_id":""}`)
	file, err := ops.create(root, ".intent-private")
	if err != nil {
		t.Fatal(err)
	}
	if n, err := ops.write(file, intent); err != nil || n != len(intent) {
		t.Fatalf("intent write = %d, %v", n, err)
	}
	if err := ops.fileSync(file); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ops.rename(root, ".intent-private", "record.json"); err != nil {
		t.Fatal(err)
	}
	if err := ops.parentSync(directory); err != nil {
		t.Fatal(err)
	}
	if err := ops.directorySync(directory); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".owned-first", ".owned-retry"} {
		file, err := ops.create(root, name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ops.write(file, []byte(`{"stage":"owned","container_id":"observed"}`)); err != nil {
			t.Fatal(err)
		}
		if err := ops.fileSync(file); err == nil {
			t.Fatal("owned publication fault did not remain active")
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	data, err := root.ReadFile("record.json")
	if err != nil || string(data) != string(intent) {
		t.Fatal("fixture failure changed the published intent")
	}
	failed := 0
	for _, event := range ops.events {
		if event.Failed && event.Stage == "owned" && event.Operation == "file fsync" {
			failed++
		}
	}
	if failed != 2 {
		t.Fatal("fixture did not record both fsync refusals")
	}
}
