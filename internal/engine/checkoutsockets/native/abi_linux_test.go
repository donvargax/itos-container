package native

import (
	"context"
	"math/bits"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port"
	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port/porttest"
)

type query struct{ trap, a1, a2, a3 uintptr }

// controlled is a probe whose system call returns what the test sets, recording
// every query it receives.
type controlled struct {
	r       uintptr
	errno   syscall.Errno
	queries []query
}

func (c *controlled) probe() Probe {
	return Probe{call: func(trap, a1, a2, a3 uintptr) (uintptr, uintptr, syscall.Errno) {
		c.queries = append(c.queries, query{trap, a1, a2, a3})
		return c.r, 0, c.errno
	}}
}

func (c *controlled) subject() porttest.KernelSubject {
	return porttest.KernelSubject{Kernel: c.probe(), Reads: func() int { return len(c.queries) }, Reply: func(reply porttest.KernelReply) {
		c.r, c.errno = ^uintptr(0), 0
		switch reply.Answer {
		case porttest.ReplyLevel, porttest.ReplyNonPositive:
			c.r = uintptr(reply.Level)
		case porttest.ReplyNotImplemented:
			c.errno = syscall.ENOSYS
		case porttest.ReplyDisabled:
			c.errno = syscall.EOPNOTSUPP
		case porttest.ReplyDenied:
			c.errno = syscall.EPERM
		}
	}}
}

func TestLandlockABIProbe(t *testing.T) {
	t.Run("shared contract", func(t *testing.T) {
		porttest.KernelContract(t, func() porttest.KernelSubject { return (&controlled{}).subject() })
	})
	t.Run("version query arguments", func(t *testing.T) {
		c := &controlled{r: 9}
		abi, failure := c.probe().LandlockABI(context.Background())
		if abi != (port.ABI{Level: 9}) || failure != port.None {
			t.Fatalf("observed %+v, %v", abi, failure)
		}
		want := query{trap: unix.SYS_LANDLOCK_CREATE_RULESET, a3: unix.LANDLOCK_CREATE_RULESET_VERSION}
		if len(c.queries) != 1 || c.queries[0] != want {
			t.Fatalf("queries = %+v, want exactly %+v (NULL attributes, zero size, version flag)", c.queries, want)
		}
		if want.trap != 444 || want.a3 != 1 {
			t.Fatalf("pinned constants drifted: %+v", want)
		}
	})
	t.Run("errors", func(t *testing.T) {
		for _, e := range []struct {
			name    string
			errno   syscall.Errno
			want    port.ABI
			failure port.Failure
		}{
			{"ENOSYS", syscall.ENOSYS, port.ABI{Absent: true}, port.None},
			{"EOPNOTSUPP", syscall.EOPNOTSUPP, port.ABI{Absent: true}, port.None},
			{"EPERM", syscall.EPERM, port.ABI{}, port.ABIQuery},
			{"EINVAL", syscall.EINVAL, port.ABI{}, port.ABIQuery},
			{"EACCES", syscall.EACCES, port.ABI{}, port.ABIQuery},
			{"EFAULT", syscall.EFAULT, port.ABI{}, port.ABIQuery},
		} {
			for _, r := range []uintptr{0, 9, ^uintptr(0)} {
				c := &controlled{r: r, errno: e.errno}
				abi, failure := c.probe().LandlockABI(context.Background())
				if abi != e.want || failure != e.failure {
					t.Errorf("%s with result %d: observed %+v, %v; want %+v, %v", e.name, r, abi, failure, e.want, e.failure)
				}
			}
		}
	})
	t.Run("invalid ABI", func(t *testing.T) {
		for _, r := range []uintptr{0, ^uintptr(0), ^uintptr(0) - 100, uintptr(1) << (bits.UintSize - 1)} {
			c := &controlled{r: r}
			abi, failure := c.probe().LandlockABI(context.Background())
			if abi != (port.ABI{}) || failure != port.ABIInvalid {
				t.Errorf("result %#x: observed %+v, %v; want ABIInvalid", r, abi, failure)
			}
		}
	})
	t.Run("positive ABI", func(t *testing.T) {
		for _, r := range []uintptr{1, 8, 9, 10, 1 << 20} {
			c := &controlled{r: r}
			abi, failure := c.probe().LandlockABI(context.Background())
			if abi != (port.ABI{Level: int(r)}) || failure != port.None {
				t.Errorf("result %d: observed %+v, %v", r, abi, failure)
			}
		}
	})
}

// kernelAnswer makes the version query itself, straight through the pinned
// golang.org/x/sys and not through the probe's seam, and maps the reply by the
// specification's rules: ENOSYS and EOPNOTSUPP mean absent, another errno is a
// query failure, a reply of zero or less is invalid, and anything else is the
// level. The running kernel does not change its answer between two calls.
func kernelAnswer() (port.ABI, port.Failure) {
	reply, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	switch {
	case errno == unix.ENOSYS || errno == unix.EOPNOTSUPP:
		return port.ABI{Absent: true}, port.None
	case errno != 0:
		return port.ABI{}, port.ABIQuery
	case int(reply) <= 0:
		return port.ABI{}, port.ABIInvalid
	}
	return port.ABI{Level: int(reply)}, port.None
}

// The real wrapper reaches the running kernel, so the answer depends on the
// machine, but never differs from what the kernel itself says: a probe bound to
// the wrong trap or to a stub fails here on a host whose answer is not the one
// the stub would give.
func TestProbeReachesTheRunningKernel(t *testing.T) {
	wantABI, wantFailure := kernelAnswer()
	t.Logf("the running kernel answers %+v, %v", wantABI, wantFailure)
	for name, probe := range map[string]Probe{"New": New(), "zero value": {}} {
		abi, failure := probe.LandlockABI(context.Background())
		if abi != wantABI || failure != wantFailure {
			t.Errorf("%s observed %+v, %v; the kernel answers %+v, %v", name, abi, failure, wantABI, wantFailure)
		}
	}
}
