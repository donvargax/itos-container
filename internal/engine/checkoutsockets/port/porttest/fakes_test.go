package porttest

import "testing"

func TestKernelFakeMeetsContract(t *testing.T) {
	KernelContract(t, func() KernelSubject { return (&Kernel{}).Subject() })
}

func TestRuntimesFakeMeetsContract(t *testing.T) {
	RuntimesContract(t, func() RuntimesSubject { return (&Runtimes{}).Subject() })
}

func TestFakesCountRefusedCalls(t *testing.T) {
	k, r := &Kernel{}, &Runtimes{}
	cancelled, cancel := cancelledContext()
	defer cancel()
	k.LandlockABI(cancelled)
	r.Runsc(cancelled)
	if k.Calls != 1 || k.Reads != 0 || r.Calls != 1 || r.Reads != 0 {
		t.Fatalf("kernel %d/%d, runtimes %d/%d calls/reads", k.Calls, k.Reads, r.Calls, r.Reads)
	}
}
