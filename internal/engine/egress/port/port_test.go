package port_test

import (
	"errors"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/egress/port"
)

// TestFailureText holds each failure's text. The handler may print it, so it is
// written here as the person would read it and holds no Go error text.
func TestFailureText(t *testing.T) {
	for failure, want := range map[port.Failure]string{
		port.ListInvalid:  "egress allowlist is invalid",
		port.ListenFailed: "egress proxy could not listen",
		port.AcceptFailed: "egress proxy stopped accepting connections",
		port.LogFailed:    "egress proxy could not write its refusal log",
		port.LogExhausted: "egress proxy refusal log is full",
	} {
		if got := failure.Error(); got != want {
			t.Errorf("Failure(%q).Error() = %q; want %q", string(failure), got, want)
		}
		var err error = failure
		if !errors.Is(err, failure) || err.Error() != want {
			t.Errorf("%q does not behave as an error", want)
		}
	}
}
