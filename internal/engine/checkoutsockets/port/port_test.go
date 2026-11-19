package port

import "testing"

func TestSelectionConfirmed(t *testing.T) {
	for _, c := range []struct {
		name string
		s    Selection
		want bool
	}{
		{"zero value", Selection{}, false},
		{"landlock", Selection{Backend: Landlock}, true},
		{"gvisor", Selection{Backend: GVisor}, true},
		{"neither", Selection{Backend: Neither}, true},
		{"failure", Selection{Failure: RuntimeInspect}, false},
		{"failure with a backend", Selection{Backend: Neither, Failure: RuntimeInspect}, false},
	} {
		if got := c.s.Confirmed(); got != c.want {
			t.Errorf("%s: Confirmed = %v, want %v", c.name, got, c.want)
		}
	}
}
