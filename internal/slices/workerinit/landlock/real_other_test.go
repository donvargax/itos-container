//go:build !linux

package landlock

import (
	"testing"

	"github.com/donvargax/itos-container/internal/slices/workerinit/port"
	"github.com/donvargax/itos-container/internal/slices/workerinit/port/porttest"
)

// Off Linux the real adapter does nothing to the process, so the contract
// runs in the test process itself.
func testRealAdapter(t *testing.T) {
	porttest.LandlockContract(t, func() (port.Landlock, porttest.LandlockWant) {
		return New(), porttest.LandlockWant{Unavailable: true}
	})
}
