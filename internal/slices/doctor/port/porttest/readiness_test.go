package porttest

import (
	"testing"

	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/port"
)

func TestReadinessContract(t *testing.T) {
	ReadinessContract(t, func(kind readiness.Failure) port.Readiness {
		return &Readiness{Result: readiness.Result{Issues: []readiness.Issue{{Kind: kind, Subject: "/checkout"}}}}
	})
}
