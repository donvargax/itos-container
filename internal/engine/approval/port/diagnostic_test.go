package port_test

import (
	"reflect"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/approval/port"
)

func TestProblemsDistinguishesUnavailableStateFromUncertainPublication(t *testing.T) {
	for _, tc := range []struct {
		failure port.Failure
		want    port.Problem
	}{
		{
			failure: "state.unavailable",
			want: port.Problem{
				Rule: "approval.state.unavailable", Exit: 3, Field: "approval",
				Message: "Approval state is unavailable",
				Fix:     "inspect with itos container doctor and run itos container approve after resolving the problem; the effective user needs a local /etc/passwd entry with an absolute home directory, so LDAP or SSSD-only accounts are refused",
			},
		},
		{
			failure: "state.location",
			want: port.Problem{
				Rule: "approval.state.location", Exit: 2, Field: "approval",
				Message: "Host state location must be absolute and valid UTF-8",
				Fix:     "set an absolute XDG_STATE_HOME, or an absolute HOME for its fallback, whose path is valid UTF-8",
			},
		},
		{
			failure: "state.write",
			want: port.Problem{
				Rule: "approval.state.write", Exit: 75, Field: "approval",
				Message: "Approval publication could not be confirmed durable",
				Fix:     "inspect actual state with itos container doctor before retrying approval",
			},
		},
	} {
		t.Run(string(tc.failure), func(t *testing.T) {
			view := port.View{Failure: tc.failure}
			view.Inspection.Ready = true
			if got := port.Problems(view); !reflect.DeepEqual(got, []port.Problem{tc.want}) {
				t.Fatalf("Problems = %+v, want %+v", got, tc.want)
			}
		})
	}
}
