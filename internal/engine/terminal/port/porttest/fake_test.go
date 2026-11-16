package porttest

import (
	"testing"

	"github.com/donvargax/itos-container/internal/engine/terminal/port"
)

func TestFakeContract(t *testing.T) {
	Contract(t, func(_ *testing.T, invalid port.FailureKind) Fixture {
		f := &Fake{Size: port.Dimensions{Columns: 93, Rows: 31}, Invalid: invalid, State: 17}
		return Fixture{Provider: f, Raw: f.Raw, Snapshot: f.Snapshot, ChangeState: f.ChangeState,
			Resize: f.Resize, Hangup: f.Hangup, Break: f.Break}
	})
}
