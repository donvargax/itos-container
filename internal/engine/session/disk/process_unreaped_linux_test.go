package disk

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
	"golang.org/x/sys/unix"
)

func TestWorkerProcessUnreapedContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorkerProcessChild$")
	child.Env = append(os.Environ(), "SLICE_17_PROCESS_CHILD=1")
	in, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close(); _ = child.Process.Kill(); _ = child.Wait() })
	p, failure := (&Workspace{}).HoldProcess(child.Process.Pid)
	if failure != "" || p == nil {
		t.Fatal("pidfd open", failure)
	}
	defer p.Close()
	f.ProcessContract(t, p, func() {
		if err := in.Close(); err != nil {
			t.Fatal(err)
		}
		var info unix.Siginfo
		if err := unix.Waitid(unix.P_PID, child.Process.Pid, &info, unix.WEXITED|unix.WNOWAIT, nil); err != nil {
			t.Fatal(err)
		}
	})
}
