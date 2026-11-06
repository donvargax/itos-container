package features

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPolicySentinelsKeepIndependentFilesAndRecorders(t *testing.T) {
	fixture := func() *world {
		w := &world{dir: t.TempDir(), policy: &policyWorld{host: t.TempDir(), recorder: t.TempDir(), env: map[string]string{}}}
		if err := w.installPolicySentinels(); err != nil {
			t.Fatal(err)
		}
		return w
	}
	first, second := fixture(), fixture()
	for _, w := range []*world{first, second} {
		entries, err := os.ReadDir(w.policy.recorder)
		if err != nil || len(entries) != 0 {
			t.Fatal("fixture setup executed a sentinel", entries, err)
		}
	}
	names := []string{"git", "docker", "devcontainer", "sh", "bash", "npm"}
	// The fixture writes .exe aliases on every host; native Go binaries remain
	// executable under that name on Unix too.
	const suffix = ".exe"
	for _, name := range names {
		// Changing one scenario's copy must not damage another scenario's oracle
		// or the cached executable used to set up later scenarios.
		if err := os.WriteFile(filepath.Join(first.policy.env["PATH"], name+suffix), []byte("changed fixture"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, w := range []*world{second, fixture()} {
		for _, name := range names {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			cmd := exec.CommandContext(ctx, filepath.Join(w.policy.env["PATH"], name+suffix))
			cmd.Env = []string{"SLICE_1_RECORDER=" + w.policy.recorder}
			output, err := cmd.CombinedOutput()
			cancel()
			if err != nil || len(output) != 0 {
				t.Fatalf("sentinel %s failed: %v, %s", name, err, output)
			}
			data, err := os.ReadFile(filepath.Join(w.policy.recorder, name+suffix))
			if err != nil || string(data) != "executed" {
				t.Fatalf("sentinel %s lost its independent marker: %q, %v", name, data, err)
			}
		}
		entries, err := os.ReadDir(w.policy.recorder)
		if err != nil || len(entries) != len(names) {
			t.Fatal("sentinels did not produce exactly their own markers", entries, err)
		}
	}
	entries, err := os.ReadDir(first.policy.recorder)
	if err != nil || len(entries) != 0 {
		t.Fatal("another scenario wrote to the first recorder", entries, err)
	}
}
