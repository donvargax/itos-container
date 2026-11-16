package features

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHomeNativeAdditionalFixtureGuards(t *testing.T) {
	if os.Getenv("ITOS_CONTAINER_ENGINE_TESTS") == "1" {
		t.Skip("guard refusal belongs to the non-engine tier")
	}
	for _, fixture := range []struct {
		name  string
		setup func(*world) error
	}{
		{"pair", (*world).homeNativePairFixture},
		{"consent", func(w *world) error { return w.homeNativeConsent(strings.Repeat("a", 64), "--reset").err }},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			w := &world{dir: t.TempDir()}
			if fixture.setup(w) == nil || w.home != nil || w.doctor != nil {
				t.Fatal("native home fixture escaped its CI-only guard")
			}
			entries, err := os.ReadDir(w.dir)
			if err != nil || len(entries) != 0 {
				t.Fatal("refused native fixture allocated state", err)
			}
		})
	}
}

func TestHomeNativeRetentionOracle(t *testing.T) {
	for _, defect := range []string{"valid", "refused", "wrong marker", "initial copy", "lost state", "different volume", "missing run"} {
		t.Run(defect, func(t *testing.T) {
			home := map[string]any{"Name": "itos-container-home-" + strings.Repeat("a", 64), "CreatedAt": "original"}
			r := homeNativeReport{UID: 1001, GID: 1002, RootUID: 1001, RootGID: 1002, Home: "/home/itos", Tmp: "/tmp", Mode: "drwx------", Writable: true}
			second := r
			second.HomeBefore, second.Marker = true, "alpha"
			runs := []homeNativeRun{{report: r, home: home}, {report: second, home: home}}
			switch defect {
			case "refused":
				runs[0].exit = 2
			case "wrong marker":
				runs[1].report.Marker = "beta"
			case "initial copy":
				runs[0].report.Initial = []string{"image-copy"}
			case "lost state":
				runs[1].report.HomeBefore = false
			case "different volume":
				runs[1].home = map[string]any{"Name": home["Name"], "CreatedAt": "replacement"}
			case "missing run":
				runs = runs[:1]
			}
			err := homeNativeRetained(runs, "alpha", "1001", "1002")
			if (err == nil) != (defect == "valid") {
				t.Fatalf("retention oracle %s: %v", defect, err)
			}
		})
	}
}

func TestHomeNativeRecoveryOracle(t *testing.T) {
	id := strings.Repeat("a", 64)
	for _, defect := range []string{"valid", "lost home", "wrong home", "live worker", "anchor leak", "pending cleanup", "wrong session", "retry retained"} {
		t.Run(defect, func(t *testing.T) {
			entry := homeNativeSessionEntry{recoveryEntry: recoveryEntry{ID: id, Project: "project", Fingerprint: "fingerprint", State: "stopped", Resource: "absent", Anchor: "absent", Cleanup: "complete"}}
			entry.Home.Storage, entry.Home.State, entry.Home.Name = "volume", "present", "home"
			switch defect {
			case "lost home":
				entry.Home.State = "absent"
			case "wrong home":
				entry.Home.Name = "another-home"
			case "live worker":
				entry.Resource = "running"
			case "anchor leak":
				entry.Anchor = "present"
			case "pending cleanup":
				entry.Cleanup = "pending"
			case "wrong session":
				entry.ID = strings.Repeat("b", 64)
			case "retry retained":
				entry.Retry = []string{"itos", "container", "stop", id}
			}
			err := homeNativeCompletedSession(entry, id, "project", "fingerprint", "home")
			if (err == nil) != (defect == "valid") {
				t.Fatalf("recovery oracle %s: %v", defect, err)
			}
		})
	}
}

func TestHomeNativeSessionResult(t *testing.T) {
	id := strings.Repeat("a", 64)
	entry := map[string]any{"id": id, "project": "original", "workload_exit": 7, "home": map[string]any{"storage": "volume", "state": "present", "name": "original-home"}}
	result := homeNativeResult{out: approvalJSON(map[string]any{"schema": 1, "ok": true, "problems": []any{}, "sessions": []any{entry}})}
	got, err := homeNativeSession(result, id)
	if err != nil || got.ID != id || got.Project != "original" || got.WorkloadExit == nil || *got.WorkloadExit != 7 || got.Home.Name != "original-home" {
		t.Fatalf("home recovery result lost its original session or observed exit: %+v %v", got, err)
	}
	if _, err := homeNativeSession(result, strings.Repeat("b", 64)); err == nil {
		t.Fatal("home recovery accepted a different session")
	}
	result.out = append(result.out, result.out...)
	if _, err := homeNativeSession(result, id); err == nil {
		t.Fatal("home recovery accepted more than one result object")
	}
}

func TestHomeNativeRegistrationIgnoresEarlierFailedAttempt(t *testing.T) {
	root := t.TempDir()
	w := &world{approval: &approvalWorld{uid: "1001", project: "project"}, doctor: &doctorWorld{checkout: filepath.Join(root, "checkout"), env: map[string]string{"XDG_STATE_HOME": root}}}
	id, token := strings.Repeat("a", 64), strings.Repeat("c", 64)
	for _, row := range []struct{ id, token, state string }{
		{strings.Repeat("b", 64), strings.Repeat("d", 64), "failed"},
		{id, token, "running"},
	} {
		path := filepath.Join(root, "itos-container", "sessions", w.recoveryIndexKey(), row.id, "record.json")
		data := approvalJSON(map[string]any{"id": row.id, "project": "project", "intent": "intents/project/" + row.token + ".json", "state": row.state})
		if err := writeManifestFixture(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	worker := map[string]any{"Config": map[string]any{"Labels": map[string]any{"io.itos-container.intent": token}}}
	got, record, err := w.homeNativeRegistration(worker)
	if err != nil || got != id || record["state"] != "running" {
		t.Fatalf("registration confused a previous failure with this worker: %s %v", got, err)
	}
	object(object(worker["Config"])["Labels"])["io.itos-container.intent"] = strings.Repeat("f", 64)
	if _, _, err := w.homeNativeRegistration(worker); err == nil {
		t.Fatal("registration adopted a different worker's intent")
	}
}

func TestHomeNativeRetirementOracle(t *testing.T) {
	for _, defect := range []string{"valid", "present", "wrong scope", "wrong project", "missing successor", "still claimed", "pending", "retry"} {
		t.Run(defect, func(t *testing.T) {
			entry := map[string]any{"id": "original", "scope": "scope", "project": "project", "name": "volume", "state": "absent", "disposition": "reset", "references": []any{}, "successor": strings.Repeat("b", 64), "retry": nil}
			switch defect {
			case "present":
				entry["state"] = "present"
			case "wrong scope":
				entry["scope"] = "other"
			case "wrong project":
				entry["project"] = "other"
			case "missing successor":
				entry["successor"] = nil
			case "still claimed":
				entry["references"] = []any{"live-session"}
			case "pending":
				entry["disposition"] = "reset-pending"
			case "retry":
				entry["retry"] = []any{"stop"}
			}
			err := homeNativeRetirementEntry(entry, "original", "scope", "project", "volume", "reset")
			if (err == nil) != (defect == "valid") {
				t.Fatalf("retirement oracle %s: %v", defect, err)
			}
		})
	}
}

func TestHomeNativeDurabilityOracle(t *testing.T) {
	path := filepath.Join("account", "home.json")
	final := sessionRecord{Mode: 0600, Bytes: `{"complete":true}`, Inode: "device:inode"}
	file := recoverySync{Inode: final.Inode, Data: final.Bytes}
	directory := recoverySync{Directory: true, Path: filepath.Dir(path), Files: map[string]string{filepath.Base(path): final.Inode}}
	for _, tc := range []struct {
		name   string
		events []recoverySync
		valid  bool
	}{
		{"synced publication", []recoverySync{file, directory}, true},
		{"file only", []recoverySync{file}, false},
		{"directory only", []recoverySync{directory}, false},
		{"reversed order", []recoverySync{directory, file}, false},
		{"different inode", []recoverySync{{Inode: "old", Data: final.Bytes}, directory}, false},
		{"different bytes", []recoverySync{{Inode: final.Inode, Data: "old"}, directory}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := homeNativeSynced(final, path, tc.events); (err == nil) != tc.valid {
				t.Fatalf("durability oracle: %v", err)
			}
		})
	}
}
