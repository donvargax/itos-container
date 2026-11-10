package disk

import (
	"io"

	"github.com/moby/sys/mountinfo"
	"golang.org/x/sys/unix"
)

func credentialOpen(parent int, name string) (int, error) {
	return unix.Openat(parent, name, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
}
func credentialReadlink(path string, buf []byte) (int, error) { return unix.Readlink(path, buf) }
func credentialParseMounts(reader io.Reader) ([]*mountinfo.Info, error) {
	return mountinfo.GetMountsFromReader(reader, nil)
}
