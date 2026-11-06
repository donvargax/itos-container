package features

import "golang.org/x/sys/unix"

func sessionInotifyInit() (int, error) { return unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC) }
func sessionInotifyAdd(fd int, path string) (int, error) {
	return unix.InotifyAddWatch(fd, path, unix.IN_OPEN)
}
func sessionInotifyRead(fd int, data []byte) (int, error) { return unix.Read(fd, data) }
