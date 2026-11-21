//go:build !linux

package landlock

import "github.com/donvargax/itos-container/internal/slices/workerinit/port"

// realCalls answers port.ErrUnavailable to every call: Landlock is Linux's.
func realCalls() calls {
	return calls{
		version:  func() (int, error) { return 0, port.ErrUnavailable },
		create:   func(rulesetAttr, uint) (int, error) { return -1, port.ErrUnavailable },
		prctl:    func(int, uintptr, uintptr, uintptr, uintptr) error { return port.ErrUnavailable },
		restrict: func(int, uint) error { return port.ErrUnavailable },
		close:    func(int) error { return port.ErrUnavailable },
	}
}
