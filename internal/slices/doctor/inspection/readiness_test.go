package inspection

import (
	"testing"

	"github.com/donvargax/itos-container/internal/engine/readiness/domain"
	readiness "github.com/donvargax/itos-container/internal/engine/readiness/port"
	readinessfake "github.com/donvargax/itos-container/internal/engine/readiness/port/porttest"
	"github.com/donvargax/itos-container/internal/slices/doctor/port"
	"github.com/donvargax/itos-container/internal/slices/doctor/port/porttest"
)

func TestReadinessContract(t *testing.T) {
	porttest.ReadinessContract(t, func(kind readiness.Failure) port.Readiness {
		return Readiness{Inspector: domain.Inspector{Checkout: &readinessfake.Checkout{Failure: kind}}, Input: readiness.Input{Root: "/checkout"}}
	})
}
