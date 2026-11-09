package tcp

import (
	"io"
	"net"
	"sync"
)

// splice copies both ways between the raw sockets, which Go turns into splice(2)
// on Linux, and returns when both directions have ended. EOF in one direction
// closes the write side of the other socket, so a half-close carries; an error in
// either closes both at once. There is no idle timeout, because agents stream for
// long periods.
func splice(a, b *net.TCPConn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go half(&wg, b, a)
	go half(&wg, a, b)
	wg.Wait()
}

func half(wg *sync.WaitGroup, dst, src *net.TCPConn) {
	defer wg.Done()
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = src.Close()
		return
	}
	_ = dst.CloseWrite()
}
