package porttest

import (
	"github.com/donvargax/itos-container/internal/engine/session/port"
	"testing"
)

func TestMountErrorContract(t *testing.T) {
	m := &MountErrors{}
	MountErrorContract(t, m, func(fail bool) {
		m.Error = port.MountSystemError{}
		if fail {
			m.Error = port.MountSystemError{Errno: 13, Message: "permission denied"}
		}
	})
}
