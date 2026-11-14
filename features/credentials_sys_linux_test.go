package features

import "golang.org/x/sys/unix"

func credentialWatchOpen() (int, error) { return unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC) }
func credentialWatchAdd(fd int, path string) (int, error) {
	return unix.InotifyAddWatch(fd, path, unix.IN_OPEN|unix.IN_ACCESS)
}
func credentialWatchClose(fd int) { _ = unix.Close(fd) }
func credentialWatchRead(fd int) ([]byte, error) {
	data := make([]byte, 65536)
	n, err := unix.Read(fd, data)
	if err != nil {
		return nil, err
	}
	return data[:n], nil
}
