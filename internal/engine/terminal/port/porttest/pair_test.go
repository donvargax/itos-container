package porttest

import (
	"testing"

	"github.com/donvargax/itos-container/internal/engine/terminal/port"
)

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
