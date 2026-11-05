package porttest

import (
	"reflect"

	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/port"
)

// Readiness returns supplied observations and records calls, without I/O.
type Readiness struct {
	Result readiness.Result
	Reads  int
}

// Inspect supplies the configured observation rather than simulating a daemon.
func (r *Readiness) Inspect() readiness.Result { r.Reads++; return r.Result }

// ReadinessContract checks prerequisite failures across the forwarding adapter
// and fake. Full successful inspection is the shared engine's own contract.
func ReadinessContract(t Reporter, create func(readiness.Failure) port.Readiness) {
	t.Helper()
	for _, kind := range []readiness.Failure{readiness.CheckoutRoot, readiness.CheckoutUnavailable} {
		got := create(kind).Inspect()
		want := readiness.Result{Issues: []readiness.Issue{{Kind: kind, Subject: "/checkout"}}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("readiness = %#v, want %#v", got, want)
		}
	}
}
