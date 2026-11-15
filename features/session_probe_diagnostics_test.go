package features

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSessionProbeDiagnostics(t *testing.T) {
	env := readFeatureEnvironment(os.Getenv, runtime.GOOS, runtime.GOARCH)
	if filteredFeatureRun(*scenarios, env) || env.os != "linux" || env.engine == "1" {
		t.Skip("whole-suite Linux fixture regression; preserve filtered and engine selections")
	}
	isolateProductCoverage(t)
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(removeCallerPath)
	bin := filepath.Join(t.TempDir(), "itos-container")
	for _, offline := range []bool{false, true} {
		name, action := "create_refusal", "create"
		if offline {
			name, action = "offline_refusal", "run"
		}
		t.Run(name, func(t *testing.T) {
			w := &world{root: root, bin: bin, dir: t.TempDir()}
			t.Cleanup(func() {
				if err := w.closeSession(); err != nil {
					t.Error(err)
				}
				w.closeDoctor()
			})
			if err := w.sessionFixture(false, "0022"); err != nil {
				t.Fatal(err)
			}
			w.session.backend, w.session.offline = "memory", offline
			if err := w.sessionChange("current approval is corrupt"); err != nil {
				t.Fatal(err)
			}
			checkpoint := false
			result, err := w.sessionProbe(action, "", func() error {
				checkpoint = true
				return nil
			})
			if result.Memory != nil || result.PreflightFailure == "" {
				t.Fatalf("expected recorded preflight refusal without memory observations: %+v", result)
			}
			if checkpoint {
				t.Error("preflight refusal reached the session checkpoint")
			}
			w.session.api.mu.Lock()
			created := len(w.session.api.created)
			w.session.api.mu.Unlock()
			if created != 0 {
				t.Error("preflight refusal created a worker")
			}
			if err == nil {
				t.Fatal("preflight refusal accepted missing observations")
			}
			for _, context := range []string{
				"memory backend produced no independent observations",
				result.PreflightFailure,
				fmt.Sprintf("exit %d;", result.Code),
				result.Diagnostic,
			} {
				if !strings.Contains(err.Error(), context) {
					t.Errorf("probe error %q lost refusal context %q", err, context)
				}
			}
		})
	}
}
