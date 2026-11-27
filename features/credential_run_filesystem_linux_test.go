package features

import (
	"io"
	"os"

	"golang.org/x/sys/unix"
)

var credentialRunFilesystemSystem = credentialRunFilesystemIO{
	openMounts: credentialRunOpenMounts,
	statfs:     credentialRunStatfs,
	lstat:      credentialRunLstat,
	statx:      credentialRunStatx,
}

func credentialRunOpenMounts() (io.ReadCloser, error) {
	return os.Open("/proc/self/mountinfo")
}

func credentialRunStatfs(path string) (uint32, error) {
	var stat unix.Statfs_t
	err := unix.Statfs(path, &stat)
	return uint32(stat.Type), err
}

func credentialRunLstat(path string) (credentialRunFileStat, error) {
	var stat unix.Stat_t
	err := unix.Lstat(path, &stat)
	return credentialRunFileStat{uid: stat.Uid, mode: uint32(stat.Mode)}, err
}

func credentialRunStatx(path string) (credentialRunFileStat, error) {
	var stat unix.Statx_t
	err := unix.Statx(unix.AT_FDCWD, path, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_BASIC_STATS|unix.STATX_MNT_ID, &stat)
	return credentialRunFileStat{uid: stat.Uid, mode: uint32(stat.Mode), mask: stat.Mask, mountID: stat.Mnt_id}, err
}
