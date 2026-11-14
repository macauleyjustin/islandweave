//go:build darwin || linux || freebsd

package beacon

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func reuseControl(network, address string, c syscall.RawConn) error {
	return c.Control(func(fd uintptr) {
		_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
		_ = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	})
}
