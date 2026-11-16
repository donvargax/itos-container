package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/approval/canonical"
	"github.com/donvargax/itos-container/internal/engine/approval/disk"
	"github.com/donvargax/itos-container/internal/engine/approval/domain"
	"github.com/donvargax/itos-container/internal/engine/approval/port"
	"github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
)

// Encode only fixture inputs, independently of the product codec.
func credentialIndependentCandidate(raw string) port.Candidate {
	var s map[string]any
	_ = json.Unmarshal([]byte(raw), &s)
	host, _ := s["host"].(map[string]any)
	checkout, _ := s["checkout"].(map[string]any)
	encode := func(value any) string { data, _ := json.Marshal(value); return string(data) }
	hash := func(text string) string { sum := sha256.Sum256([]byte(text)); return hex.EncodeToString(sum[:]) }
	bytes := encode(s)
	return port.Candidate{UID: host["uid"].(string), GID: host["gid"].(string), Root: checkout["root"].(string), Project: hash(encode(map[string]any{"schema": 1, "uid": host["uid"], "checkout": checkout})), Canonical: bytes, Fingerprint: "sha256:" + hash(bytes)}
}

func TestCredentialFreezeProcess(t *testing.T) {
	path := os.Getenv("SLICE_30_ADMISSION_INPUT")
	if path == "" {
		t.Skip("feature freeze assembly only")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var request struct{ Approved, Replacement string }
	if err := json.Unmarshal(data, &request); err != nil {
		t.Fatal(err)
	}
	a, b := credentialIndependentCandidate(request.Approved), credentialIndependentCandidate(request.Replacement)
	result := map[string]any{}
	for _, backend := range []string{"memory", "disk"} {
		codec := canonical.JSON{}
		state := t.TempDir()
		memory := &porttest.Store{Records: map[string]port.Record{a.Project: {Candidate: a, Generation: "4"}}}
		store := disk.Store{StateHome: state, Records: codec}
		var active port.Store = memory
		snapshots := &porttest.Snapshots{Values: map[string]port.Frozen{}}
		var publisher port.Snapshots = snapshots
		if backend == "disk" {
			active = store
			contractSeed(t, state, a, "4")
			publisher = disk.Snapshots{Store: store, Records: codec}
		}
		service := domain.Service{Store: active}
		view := port.View{Candidate: a, Record: port.Record{Candidate: a, Generation: "4"}}
		selected, failure := service.Freeze(view)
		var durable port.Durable
		var publication port.Failure
		if failure == "" {
			durable, publication = publisher.Publish(selected)
		}
		retained := selected
		bytes := selected.Bytes()
		if len(bytes) > 0 {
			bytes[0] = 'X'
		}
		selected.Settings = "caller changed"
		independent, secondFailure := service.Freeze(view)
		_, replaceFailure := active.Commit(b, "4")
		_, stale := service.Freeze(view)
		var persisted any
		count := len(snapshots.Values)
		if backend == "memory" {
			persisted = snapshots.Values[a.Project+"/4"]
		} else {
			path := filepath.Join(state, "itos-container", "snapshots", a.Project, "4.json")
			if data, err := os.ReadFile(path); err == nil {
				var record map[string]any
				_ = json.Unmarshal(data, &record)
				persisted = record
			}
			entries, _ := os.ReadDir(filepath.Dir(path))
			count = len(entries)
		}
		result[backend] = map[string]any{"failure": failure, "publication": publication, "retained": retained, "durable": durable, "independent": independent, "second_failure": secondFailure, "replacement_failure": replaceFailure, "stale": stale, "persisted": persisted, "count": count}
	}
	data, err = json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("SLICE_30_ADMISSION_OUTPUT"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
