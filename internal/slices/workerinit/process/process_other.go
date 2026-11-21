//go:build !linux

package process

import "github.com/donvargax/itos-container/internal/slices/workerinit/port"

// Lookup is unavailable off Linux.
func (*Adapter) Lookup(string) (string, error) { return "", port.ErrUnavailable }

// Exec is unavailable off Linux.
func (*Adapter) Exec(string, []string) error { return port.ErrUnavailable }
