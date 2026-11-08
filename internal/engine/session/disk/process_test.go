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
	f.ProcessAbsenceContract(t, &Workspace{}, child.Process.Pid)
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
		p := &workerProcess{file: file, poll: func(fd int32) (int16, error) {
			if int(fd) != int(file.Fd()) {
				t.Errorf("poll used different reference: %d", fd)
			}
			return 0, fault
		}}
		if exited, failure := p.Exited(); exited || failure != port.MountRefused {
			t.Fatal("failed poll claimed exit", exited, failure)
		}
		if p.MountError() != mountSystemError(fault) {
			t.Fatal("lost poll diagnostic", p.MountError())
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

func TestHoldProcessRetainsPIDFDOpenError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native pidfd is Linux only")
	}
	widePID := int64(1) << 31
	pid := int(widePID)
	if int64(pid) != widePID {
		t.Skip("requires a host integer wider than the kernel pid_t")
	}
	// pidfd_open consumes a signed 32-bit pid_t. This positive Go int reaches
	// the syscall as a negative PID and fails with EINVAL, not process absence.
	w := &Workspace{}
	process, failure := w.HoldProcess(pid)
	if process != nil {
		process.Close()
		t.Fatal("invalid kernel PID produced a process reference")
	}
	if failure != port.MountRefused || w.MountError() != mountSystemError(syscall.EINVAL) {
		t.Fatal("pidfd error lost or misreported as process absence", failure, w.MountError())
	}
}

func TestWorkerProcessReadiness(t *testing.T) {
	for _, tc := range []struct {
		name   string
		events int16
		exited bool
		failed bool
	}{
		{"running", 0, false, false},
		{"unreaped", 0x0001, true, false},
		{"reaped", 0x0011, true, false},
		{"hangup without exit", 0x0010, false, true},
		{"invalid descriptor", 0x0020, false, true},
		{"poll error", 0x0008, false, true},
		{"readable with error", 0x0009, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := os.CreateTemp(t.TempDir(), "reference")
			if err != nil {
				t.Fatal(err)
			}
			p := &workerProcess{file: file, poll: func(int32) (int16, error) { return tc.events, nil }}
			defer p.Close()
			exited, failure := p.Exited()
			if exited != tc.exited || (failure == port.MountRefused) != tc.failed {
				t.Fatal("incorrect readiness observation", exited, failure)
			}
			if tc.failed && p.MountError().Message == "" {
				t.Fatal("missing readiness diagnostic")
			}
			p.Close()
			if exited, failure := p.Exited(); exited || failure != port.MountRefused || p.MountError().Errno != int(syscall.EBADF) {
				t.Fatal("closed reference supplied authority", exited, failure, p.MountError())
			}
		})
	}
}
