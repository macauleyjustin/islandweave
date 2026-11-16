package internet

import (
	"net"
	"strings"
	"time"

	"github.com/islandweave/islandweave/transports/lan"
)

// IsPublicTCPAddr reports whether addr appears to be a globally routable IP/hostname (non-LAN, non-onion).
func IsPublicTCPAddr(addr string) bool {
	lower := strings.ToLower(addr)
	if strings.Contains(lower, ".onion") {
		return false
	}
	return !lan.IsLocalAddress(addr)
}

// DialDirect establishes a direct TCP connection with timeout.
func DialDirect(addr string, timeout time.Duration) (net.Conn, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return net.DialTimeout("tcp", addr, timeout)
}
