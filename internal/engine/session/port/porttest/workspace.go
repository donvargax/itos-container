package porttest

import "github.com/donvargax/itos-container/internal/engine/session/port"

// WorkspaceContract keeps successful validation, substitution refusal and pin
// lifetime identical for the memory fake and handle-relative disk adapter.
func WorkspaceContract(t Reporter, w port.Workspace, binding port.Binding, substitute func()) {
	t.Helper()
	defer w.Close()
	if f := w.Pin(binding); f != "" {
		t.Errorf("pin: %s", f)
		return
	}
	for range 2 {
		if f := w.Validate(); f != "" {
			t.Errorf("unchanged workspace: %s", f)
		}
	}
	substitute()
	if f := w.Validate(); f != port.Refused {
		t.Errorf("substituted workspace: %s", f)
	}
	w.Close()
	if f := w.Validate(); f != port.Refused {
		t.Errorf("closed workspace: %s", f)
	}
}
