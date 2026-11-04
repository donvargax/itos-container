// Package input is the Files the count command reads through: the file the
// argument names, or standard input where the argument is -.
//
// The domain knows no paths of the command line and no standard input: it is
// handed bytes through this port, and this adapter is the one place the - of
// a command line becomes the reader behind it.
package input

import (
	"io"

	"github.com/donvargax/itos-container/internal/slices/count/port"
)

// Stdin is the argument that names standard input.
const Stdin = "-"

// Files reads a named file through Disk, and all of In where the path is -.
type Files struct {
	Disk port.Files
	In   io.Reader
}

var _ port.Files = Files{}

// Read reads path, and refuses any path but Stdin through In, so a caller
// that names no file reads the disk and one that names - reads a reader. A
// reader that fails is the environment's, as a file that cannot be read is.
func (f Files) Read(path string) ([]byte, port.ReadFailure) {
	if path != Stdin {
		return f.Disk.Read(path)
	}
	data, err := io.ReadAll(f.In)
	if err != nil {
		return nil, &port.Unreadable{Path: path}
	}
	return data, nil
}
