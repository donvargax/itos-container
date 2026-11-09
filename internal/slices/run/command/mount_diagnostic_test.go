package command

import (
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/cli"
	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestMountDiagnosticPreservesClassificationAndExit(t *testing.T) {
	for _, exit := range []int{0, 7} {
		var stdout, stderr strings.Builder
		r := port.RunResult{Result: port.Result{Outcome: port.MountRefused, Cleanup: port.Complete}, Exited: true, Exit: exit,
			MountDiagnostic: port.MountDiagnostic{Check: "mounted-identity", Reason: "identity differs", ExpectedDevice: "2049", ExpectedInode: "101", ObservedDevice: "2049", ObservedInode: "102", PID: 123}}
		code := reportRun(&cli.UI{Stdout: &stdout, Stderr: &stderr, Classify: Classify}, r)
		if code != max(1, exit) || stdout.Len() != 0 {
			t.Fatal(code, stdout.String(), stderr.String())
		}
		for _, want := range []string{"run.mount-refused", "check=mounted-identity", "expected=2049:101", "observed=2049:102", "pid=123"} {
			if !strings.Contains(stderr.String(), want) {
				t.Fatal(want, stderr.String())
			}
		}
	}
}

func TestMountDiagnosticRejectsNonnumericIdentityPayload(t *testing.T) {
	d := port.MountDiagnostic{Check: "mounted-stat", ExpectedDevice: "private response", ObservedInode: "private path", System: port.MountSystemError{Errno: 13, Message: "permission denied"}}
	got := mountDiagnosticText(d)
	if strings.Contains(got, "private") || !strings.Contains(got, "expected=unavailable:unavailable") || !strings.Contains(got, `errno=13 error="permission denied"`) {
		t.Fatal(got)
	}
}
