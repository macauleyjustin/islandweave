package lan

import (
	"net"
	"net/netip"
	"strings"
)

// IsLocalAddress classifies whether an endpoint host:port belongs to loopback or a local/private subnet (RFC 1918 / ULA / link-local).
func IsLocalAddress(addr string) bool {
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".local")
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
