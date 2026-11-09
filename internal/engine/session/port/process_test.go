package port_test

import (
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestValidDetectorAcceptsOnlyRecordedOutcomes(t *testing.T) {
	for _, value := range []string{"", "verified", "not observed: worker exited", "not observed: worker root changed", "refused"} {
		if !port.ValidDetector(value) {
			t.Errorf("rejected recorded detector outcome %q", value)
		}
	}
	for _, value := range []string{"unknown", "not observed", "verified ", "engine response"} {
		if port.ValidDetector(value) {
			t.Errorf("accepted unknown detector outcome %q", value)
		}
	}
}
