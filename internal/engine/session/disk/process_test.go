package disk

import (
	"io"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestWorkerProcessChild(t *testing.T) {
	if os.Getenv("SLICE_17_PROCESS_CHILD") != "1" {
		t.Skip("process reference child only")
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
}

func TestWorkerProcessContract(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native pidfd is Linux only")
	}
	child := exec.Command(os.Args[0], "-test.run=^TestWorkerProcessChild$")
	child.Env = append(os.Environ(), "SLICE_17_PROCESS_CHILD=1")
	in, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = child.Start(); err != nil {
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
		if err := child.Wait(); err != nil {
			t.Fatal(err)
		}
	})
	// A reaped PID cannot produce a new live reference.
	if p, failure := (&Workspace{}).HoldProcess(child.Process.Pid); p != nil || failure != port.MountRefused {
		t.Fatal(p, failure)
	}
}

func TestWorkerProcessRefusesInvalidReferences(t *testing.T) {
	for _, pid := range []int{0, -1} {
		if p, failure := (&Workspace{}).HoldProcess(pid); p != nil || failure != port.MountRefused {
			t.Fatal(pid, p, failure)
		}
	}
	for _, fault := range []error{syscall.EPERM, syscall.EBADF} {
		file, err := os.CreateTemp(t.TempDir(), "reference")
		if err != nil {
			t.Fatal(err)
		}
		p := &workerProcess{file: file, signal: func(fd int) error {
			if fd != int(file.Fd()) {
				t.Errorf("signal used different reference: %d", fd)
			}
			return fault
		}}
		if exited, failure := p.Exited(); exited || failure != port.MountRefused {
			t.Fatal("failed signal claimed exit", exited, failure)
		}
		p.Close()
	}
	w := &Workspace{ProcessOpen: func(pid int) (port.Process, port.Outcome) {
		if pid != 123 {
			t.Fatal(pid)
		}
		return nil, port.MountRefused
	}}
	if p, failure := w.HoldProcess(123); p != nil || failure != port.MountRefused {
		t.Fatal(p, failure)
	}
}
