package disk

import "golang.org/x/sys/unix"

const credentialNoFollow = unix.O_NOFOLLOW
const credentialDirectory = unix.O_DIRECTORY

func credentialFilesystem(path string) (int64, error) {
	var stat unix.Statfs_t
	err := unix.Statfs(path, &stat)
	return int64(stat.Type), err
}
