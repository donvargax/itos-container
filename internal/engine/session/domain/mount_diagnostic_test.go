package domain

import (
	"testing"
	"testing/synctest"

	"github.com/donvargax/itos-container/internal/engine/session/port"
	f "github.com/donvargax/itos-container/internal/engine/session/port/porttest"
)

func TestFiniteMountDiagnosticSharesOperationGate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := make(chan struct{}, 1)
		source := &f.MountErrors{Error: port.MountSystemError{Errno: 13, Message: "permission denied"}}
		got := finiteMountError(gate, source)
		if got.Errno != 0 || got.Message != "diagnostic unavailable while observation is pending" {
			t.Fatal(got)
		}
		gate <- struct{}{}
		synctest.Wait()
		if got := finiteMountError(gate, source); got != source.Error {
			t.Fatal(got)
		}
	})
}

func TestMountDiagnosticWithoutStartupState(t *testing.T) {
	if got := anchorRefusal(t.Context(), port.Record{}, "anchor-record", "missing"); got != port.MountRefused {
		t.Fatal(got)
	}
	if got := mountSystemDetail(struct{}{}); got != (port.MountSystemError{}) {
		t.Fatal(got)
	}
}
