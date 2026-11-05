//go:build !linux

package features

import "net"

func runPeerPID(net.Conn) int { return 0 }
