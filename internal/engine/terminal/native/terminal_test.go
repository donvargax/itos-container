package native

import (
	"context"
	"errors"
	"testing"

	"github.com/donvargax/itos-container/internal/engine/terminal/port"
)

func TestUnsupportedBeforeFileAccess(t *testing.T) {
	l, err := New(nil, "unsupported").Acquire(context.Background())
	var failure *port.Failure
	if l != nil || !errors.As(err, &failure) || failure.Kind != port.Unsupported {
		t.Fatalf("unsupported OS = %v, %v", l, err)
	}
}
