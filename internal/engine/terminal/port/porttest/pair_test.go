package porttest

import (
	"context"
	"errors"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/terminal/port"
)

func TestPairAcquisitionRetainsRollbackFailure(t *testing.T) {
	for _, seam := range []bool{false, true} {
		t.Run(map[bool]string{false: "provider", true: "output state"}[seam], func(t *testing.T) {
			restore := errors.New("restore failed")
			p := &Pair{
				Input:  &Fake{Size: port.Dimensions{Columns: 113, Rows: 37}, RestoreError: restore},
				Output: &Fake{Invalid: port.AcquisitionFailed},
			}
			if seam {
				p.OutputStateError = errors.New("state unavailable")
			}
			l, err := p.Acquire(context.Background())
			var failure *port.Failure
			if l != nil || !errors.As(err, &failure) || failure.Kind != port.AcquisitionFailed || !errors.Is(failure.RestoreErr, restore) {
				t.Fatalf("rollback failure lost: %v %v", l, err)
			}
		})
	}
}

func TestPairContract(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		distinct, outputEvents bool
	}{
		{"shared_terminal", false, false},
		{"distinct_input_events", true, false},
		{"distinct_output_events", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			Contract(t, func(_ *testing.T, invalid port.FailureKind) Fixture {
				input := &Fake{Size: port.Dimensions{Columns: 93, Rows: 31}, Invalid: invalid, State: 17}
				output := input
				if tc.distinct {
					output = &Fake{OutputOnly: true, State: 29}
				}
				events := input
				if tc.outputEvents {
					events = output
				}
				return Fixture{
					Provider: &Pair{Input: input, Output: output},
					Raw:      func() bool { return input.Raw() && output.Raw() },
					Snapshot: func() any { return [2]any{input.Snapshot(), output.Snapshot()} },
					ChangeState: func() {
						input.ChangeState()
						if tc.distinct {
							output.ChangeState()
						}
					},
					Resize: input.Resize,
					Hangup: events.Hangup,
					Break:  events.Break,
				}
			})
		})
	}
}
