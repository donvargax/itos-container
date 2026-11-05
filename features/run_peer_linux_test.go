package features

import (
	"golang.org/x/sys/unix"
	"net"
)

func runPeerPID(conn net.Conn) int {
	c, ok := conn.(*net.UnixConn)
	if !ok {
		return 0
	}
	raw, err := c.SyscallConn()
	if err != nil {
		return 0
	}
	pid := 0
	_ = raw.Control(func(fd uintptr) {
		cred, e := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if e == nil {
			pid = int(cred.Pid)
		}
	})
	return pid
}
