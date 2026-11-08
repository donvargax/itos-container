package porttest

import "github.com/donvargax/itos-container/internal/engine/session/port"

// MountErrors supplies syscall observations as data without host I/O.
type MountErrors struct{ Error port.MountSystemError }

func (m *MountErrors) MountError() port.MountSystemError { return m.Error }

func MountErrorContract(t Reporter, source port.MountErrorSource, observe func(bool)) {
	t.Helper()
	observe(true)
	want := port.MountSystemError{Errno: 13, Message: "permission denied"}
	for range 2 {
		if got := source.MountError(); got != want {
			t.Errorf("syscall diagnostic: %#v", got)
		}
	}
	observe(false)
	if got := source.MountError(); got != (port.MountSystemError{}) {
		t.Errorf("stale syscall diagnostic: %#v", got)
	}
}
