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
func credentialStatx(fd int) (uint64, uint32, error) {
	var stat unix.Statx_t
	err := unix.Statx(fd, "", unix.AT_EMPTY_PATH|unix.AT_SYMLINK_NOFOLLOW, unix.STATX_MNT_ID, &stat)
	return stat.Mnt_id, stat.Mask, err
}
func credentialParseMounts(reader io.Reader) ([]*mountinfo.Info, error) {
	return mountinfo.GetMountsFromReader(reader, nil)
}
