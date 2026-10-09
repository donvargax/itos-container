// Package port defines the files the counting domain can read.
package port

// ReadFailure describes a file-read failure without exposing operating-system
// error types to the domain.
type ReadFailure interface {
	error
	readFailure()
}

// Missing is returned when the requested input does not exist.
type Missing struct{ Path string }

func (*Missing) readFailure()    {}
func (e *Missing) Error() string { return "missing file: " + e.Path }

// Unreadable is returned when the requested input exists but cannot be read.
type Unreadable struct{ Path string }

func (*Unreadable) readFailure()    {}
func (e *Unreadable) Error() string { return "unreadable file: " + e.Path }

// Files reads the bytes at a path. Implementations return a typed ReadFailure
// for missing and unreadable inputs.
type Files interface {
	Read(path string) ([]byte, ReadFailure)
}
