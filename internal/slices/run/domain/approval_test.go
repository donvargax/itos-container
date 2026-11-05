package domain

import (
	"errors"
	approval "github.com/donvargax/itos-container/internal/engine/approval/port"
	enginefake "github.com/donvargax/itos-container/internal/engine/approval/port/porttest"
	"github.com/donvargax/itos-container/internal/slices/run/port/porttest"
	"testing"
)

func TestApprovalBeforeStartup(t *testing.T) {
	for _, kind := range []string{"matching", "missing", "changed", "invalid", "prerequisite", "superseded"} {
		fake := &porttest.Approval{View: approval.View{Status: kind, Inspection: enginefake.Ready()}}
		want := kind
		switch kind {
		case "matching":
			want = ""
		case "invalid":
			fake.View.Failure = "state.invalid"
		case "prerequisite":
			fake.View.Inspection.Ready = false
			want = "invalid"
		case "superseded":
			fake.View.Status = "matching"
			fake.Failure = "superseded"
		}
		err := CheckApproval(fake)
		var refusal ApprovalRefusal
		if want == "" {
			if err != nil {
				t.Fatal(err)
			}
		} else if !errors.As(err, &refusal) || refusal.Reason != want || refusal.Error() == "" {
			t.Fatal(err)
		}
		selected := 0
		if kind == "matching" || kind == "superseded" {
			selected = 1
		}
		if fake.Inspections != 1 || fake.Selections != selected {
			t.Fatal(fake)
		}
	}
}
