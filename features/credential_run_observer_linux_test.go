package features

import "golang.org/x/sys/unix"

var credentialWatchSystem = credentialWatchIO{
	open:     credentialWatchOpen,
	addWatch: unix.InotifyAddWatch,
	read:     unix.Read,
	poll:     credentialRunWatchPoll,
	close:    unix.Close,
}

func credentialRunWatchPoll(fd, timeoutMillis int) (int, error) {
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	return unix.Poll(fds, timeoutMillis)
}

func credentialRunDevice(path string) (uint64, error) {
	var stat unix.Stat_t
	err := unix.Stat(path, &stat)
	return uint64(stat.Dev), err
}
