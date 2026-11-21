//go:build !linux

package process

import (
	"errors"
	"testing"

	"github.com/donvargax/itos-container/internal/slices/workerinit/port"
)

func testRealProcess(*testing.T) {}

// Off Linux nothing can be replaced: both calls refuse, and no child runs.
func testExec(t *testing.T) {
	a := New()
	if got, err := a.Lookup("tool"); got != "" || !errors.Is(err, port.ErrUnavailable) {
		t.Errorf("Lookup = %q, %v; want empty, ErrUnavailable", got, err)
	}
	if err := a.Exec("/bin/tool", []string{"tool"}); !errors.Is(err, port.ErrUnavailable) {
		t.Errorf("Exec = %v; want ErrUnavailable", err)
	}
}
