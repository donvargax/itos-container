package porttest

import (
	"context"
	"errors"
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

func TestControlledRestorationFailure(t *testing.T) {
	restoreErr := errors.New("restoration refused")
	f := &Fake{Size: port.Dimensions{Columns: 93, Rows: 31}, RestoreError: restoreErr}
	l, err := f.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r := l.Close()
	if r.Cause != port.Closed || r.Err != nil || r.RestoreErr != restoreErr || !f.Raw() {
		t.Fatalf("restoration failure was hidden: %+v", r)
	}
	if l.Close() != r {
		t.Fatal("restoration result changed")
	}
}

func TestFakeInvalidSizeAndVanishedTerminal(t *testing.T) {
	f := &Fake{Size: port.Dimensions{Columns: 1, Rows: 0}}
	refusal(t, f, context.Background(), port.InvalidDimensions)
	f.Resize(port.Dimensions{Columns: 1, Rows: 1})
	f.Break()
	refusal(t, f, context.Background(), port.InvalidTerminal)
}
