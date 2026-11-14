//go:build !linux

package disk

import (
	"errors"
	"io"

	"github.com/moby/sys/mountinfo"
)

func credentialOpen(int, string) (int, error)                    { return -1, errors.ErrUnsupported }
func credentialReadlink(string, []byte) (int, error)             { return 0, errors.ErrUnsupported }
func credentialStatx(int) (uint64, uint32, error)                { return 0, 0, errors.ErrUnsupported }
func credentialParseMounts(io.Reader) ([]*mountinfo.Info, error) { return nil, errors.ErrUnsupported }
