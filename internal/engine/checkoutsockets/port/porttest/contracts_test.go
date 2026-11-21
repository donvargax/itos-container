package porttest

import (
	"context"
	"fmt"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/checkoutsockets/port"
)

// recorder collects what a contract reports, so a test can require that a
// broken implementation is caught instead of passing silently.
type recorder struct{ reports []string }

func (r *recorder) Helper() {}
func (r *recorder) Errorf(format string, args ...any) {
	r.reports = append(r.reports, fmt.Sprintf(format, args...))
}

type ignoresCancellationKernel struct{ port.Kernel }

func (k ignoresCancellationKernel) LandlockABI(context.Context) (port.ABI, port.Failure) {
	return k.Kernel.LandlockABI(context.Background())
}

type ignoresCancellationRuntimes struct{ port.Runtimes }

func (r ignoresCancellationRuntimes) Runsc(context.Context) (port.Registration, port.Failure) {
	return r.Runtimes.Runsc(context.Background())
}

func TestKernelContractCatchesBrokenKernels(t *testing.T) {
	for name, create := range map[string]func() KernelSubject{
		"answers do not follow the observation": func() KernelSubject {
			s := (&Kernel{ABI: port.ABI{Level: 1}}).Subject()
			s.Reply = func(KernelReply) {}
			return s
		},
		"cancelled call still observes": func() KernelSubject {
			s := (&Kernel{}).Subject()
			s.Kernel = ignoresCancellationKernel{s.Kernel}
			return s
		},
		"observations are not counted": func() KernelSubject {
			s := (&Kernel{}).Subject()
			s.Reads = func() int { return 0 }
			return s
		},
	} {
		var r recorder
		KernelContract(&r, create)
		if len(r.reports) == 0 {
			t.Errorf("%s: contract passed", name)
		}
	}
	var r recorder
	KernelContract(&r, func() KernelSubject { return (&Kernel{}).Subject() })
	if len(r.reports) != 0 {
		t.Errorf("correct fake reported: %v", r.reports)
	}
}

func TestRuntimesContractCatchesBrokenRuntimes(t *testing.T) {
	for name, create := range map[string]func() RuntimesSubject{
		"answers do not follow the observation": func() RuntimesSubject {
			s := (&Runtimes{Registration: port.RunscRegistered}).Subject()
			s.Reply = func(RuntimesReply) {}
			return s
		},
		"cancelled call still observes": func() RuntimesSubject {
			s := (&Runtimes{}).Subject()
			s.Runtimes = ignoresCancellationRuntimes{s.Runtimes}
			return s
		},
		"observations are not counted": func() RuntimesSubject {
			s := (&Runtimes{}).Subject()
			s.Reads = func() int { return 0 }
			return s
		},
	} {
		var r recorder
		RuntimesContract(&r, create)
		if len(r.reports) == 0 {
			t.Errorf("%s: contract passed", name)
		}
	}
	var r recorder
	RuntimesContract(&r, func() RuntimesSubject { return (&Runtimes{}).Subject() })
	if len(r.reports) != 0 {
		t.Errorf("correct fake reported: %v", r.reports)
	}
}
