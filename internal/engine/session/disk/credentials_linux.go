package disk

import "golang.org/x/sys/unix"

func credentialFilesystem(path string) (int64, error) {
	var stat unix.Statfs_t
	err := unix.Statfs(path, &stat)
	return int64(stat.Type), err
}
