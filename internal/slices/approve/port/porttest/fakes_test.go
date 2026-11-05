package porttest

import (
	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	shared "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"strings"
	"testing"
)

func TestReviewContract(t *testing.T) {
	shared.ReviewContract(t, func(line string) approval.Reviewer {
		word := strings.ToLower(strings.TrimSpace(line))
		return &UI{IsTerminal: true, Yes: strings.HasSuffix(line, "\n") && (word == "y" || word == "yes")}
	})
}
