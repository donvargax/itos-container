package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCredentialInvalidStateLocation(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("credential observation requires Linux amd64")
	}
	root := t.TempDir()
	t.Chdir(root)
	policy := "version: 3\nmanifest: a.json\ncredentials: [{kind: claude-token, source: /synthetic/token}]\n"
	if err := os.WriteFile(filepath.Join(root, "itos-container.yaml"), []byte(policy), 0600); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"relative", ""} {
		t.Run("state="+state, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", state)
			t.Setenv("HOME", "")
			// Every absolute spelling must get past checkout validation and
			// report the invalid state, without requiring native metadata traversal.
			for _, checkout := range []string{root, root + "/", strings.ReplaceAll(root, "/", "//"), root + "/.", root + "/unused/.."} {
				t.Setenv("ITOS_ROOT", checkout)
				var out, errout bytes.Buffer
				code := run([]string{"doctor", "--credentials-only", "--json"}, strings.NewReader(""), &out, &errout)
				if code != 2 || !strings.Contains(out.String(), `"rule":"approval.state.location"`) {
					t.Fatalf("invalid state location with checkout %q: exit %d, stdout %s, stderr %s", checkout, code, out.String(), errout.String())
				}
			}
		})
	}
}
