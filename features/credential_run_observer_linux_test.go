package features

import "golang.org/x/sys/unix"

var credentialWatchSystem = credentialWatchIO{
	open:     credentialWatchOpen,
	addWatch: credentialRunWatchAdd,
	read:     unix.Read,
	poll:     credentialRunWatchPoll,
	close:    unix.Close,
}

func credentialRunWatchAdd(fd int, path string) (int, error) {
	const mask = unix.IN_CREATE | unix.IN_MODIFY | unix.IN_ATTRIB | unix.IN_CLOSE_WRITE |
		unix.IN_MOVED_FROM | unix.IN_MOVED_TO | unix.IN_DELETE | unix.IN_DELETE_SELF |
		unix.IN_MOVE_SELF | unix.IN_ONLYDIR | unix.IN_DONT_FOLLOW
	return unix.InotifyAddWatch(fd, path, mask)
}

func credentialRunWatchPoll(fd int) (int, error) {
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	return unix.Poll(fds, 10)
}

func credentialRunDevice(path string) (uint64, error) {
	var stat unix.Stat_t
	err := unix.Stat(path, &stat)
	return uint64(stat.Dev), err
}
