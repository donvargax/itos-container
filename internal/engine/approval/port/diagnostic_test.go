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
				Fix: "resolve the cause and inspect again with itos container doctor; approve cannot clear it. The effective user needs a local /etc/passwd entry with an existing absolute home directory, so LDAP or SSSD-only accounts are refused. " +
					"A state location the catalog retains may be missing or unreadable, which refuses every selection on the account until the host user restores the original directory or removes its record under the account home's .local/state/itos-container/coordination after confirming no worker uses it. " +
					"Session records with no catalog refuse that selection only, so choose another XDG_STATE_HOME",
			},
		},
		{
			failure: "state.path",
			want: port.Problem{
				Rule: "approval.state.path", Exit: 1, Field: "approval",
				Message: "Approval state path is unsafe or was substituted",
				Fix:     "use a stable state tree outside the checkout without symbolic links; the account home's own path must have none either, and XDG_STATE_HOME cannot move that anchor",
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
