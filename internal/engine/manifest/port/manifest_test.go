package port_test

import (
	"github.com/donvargax/itos-container/internal/engine/manifest/port"
	"testing"
)

func TestPointer(t *testing.T) {
	for _, tc := range []struct {
		path []string
		want string
	}{{nil, ""}, {[]string{"a/b~c", ""}, "/a~1b~0c/"}, {[]string{"~1", "/"}, "/~01/~1"}} {
		if got := port.Pointer(tc.path); got != tc.want {
			t.Fatalf("%q, want %q", got, tc.want)
		}
	}
}
