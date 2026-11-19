package command

import (
	"fmt"
	"strings"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/session/port"
)

func TestCheckoutOverlapReportsRefusalAndOriginalRecoveryGuidance(t *testing.T) {
	code, problems := Classify(fmt.Errorf("launch: %w", runFailure{outcome: port.CheckoutOverlap}))
	if code != 1 || len(problems) != 1 || problems[0].Exit != 1 || problems[0].Rule != "run.checkout-overlap" || !strings.Contains(problems[0].Fix, "original checkout") || !strings.Contains(problems[0].Message, "unfinished session") {
		t.Fatal("overlapping checkout lost its refusal or recovery guidance", code, problems)
	}
}
