package features

import (
	"debug/buildinfo"
	"os"
	"path/filepath"
	"testing"
)

func TestEgressBuildWithoutVCSMetadata(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":  "module fixture.example/egress\n",
		"main.go": "package main\nfunc main() {}\n",
		// Proof snapshots can carry a worktree pointer whose target is absent.
		".git": "gitdir: " + filepath.Join(root, "unavailable-git-directory") + "\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	output := filepath.Join(t.TempDir(), "probe")
	if err := egressBuild(root, ".", output); err != nil {
		t.Fatal("trusted fixture build requires unavailable VCS metadata:", err)
	}
	info, err := buildinfo.ReadFile(output)
	if err != nil {
		t.Fatal("trusted fixture build did not produce a Go executable:", err)
	}
	if info.Path != "fixture.example/egress" {
		t.Fatal("trusted fixture build did not compile the requested source", info.Path)
	}
}
