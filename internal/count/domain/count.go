// Package domain owns the private example's file-counting behavior.
package domain

import (
	"bytes"
	"strings"

	"github.com/donvargax/itos-container/internal/count/port"
)

// Result is the number of logical lines and whitespace-delimited words.
type Result struct {
	Lines int
	Words int
}

// Count counts logical lines and words in data. A terminal LF ends the last
// line rather than starting another one; CRLF has one LF separator. Fields
// uses Go's Unicode whitespace rules and preserves punctuation within words.
func Count(data []byte) Result {
	lines := bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		lines++
	}
	return Result{Lines: lines, Words: len(strings.Fields(string(data)))}
}

// File counts the contents of path through the Files port.
func File(files port.Files, path string) (Result, error) {
	data, failure := files.Read(path)
	if failure != nil {
		return Result{}, failure
	}
	return Count(data), nil
}
